// Package ledger persists per-MCP-client session metrics to .remindb/sessions/ as append-only JSONL.
package ledger

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/radimsem/remindb/internal/contentid"
	"github.com/radimsem/remindb/pkg/config"
	"github.com/radimsem/remindb/pkg/mcp/jsonlsink"
)

const (
	subDir = "sessions"

	// Line-scan tuning for reading client files.
	scanInitialBuf = 64 << 10 // 64 KiB initial buffer
	scanMaxLine    = 1 << 20  // 1 MiB cap on a single ledger line
)

type ClientMeta struct {
	Name     string `json:"name"`
	Title    string `json:"title,omitempty"`
	Version  string `json:"version"`
	Protocol string `json:"protocol"`
}

type Record struct {
	SessionID      string     `json:"session_id"`
	Client         ClientMeta `json:"client"`
	Transport      string     `json:"transport"`
	ConnectedAt    int64      `json:"connected_at"`
	LastSeen       int64      `json:"last_seen"`
	DisconnectedAt int64      `json:"disconnected_at,omitempty"`
	ToolCalls      int64      `json:"tool_calls"`
}

type ClientLedger struct {
	Hash            string     `json:"hash"`
	Client          ClientMeta `json:"client"`
	Transport       string     `json:"transport"`
	Sessions        int        `json:"sessions"`
	LifetimeSeconds int64      `json:"lifetime_seconds"`
	LastDisconnect  int64      `json:"last_disconnect"`
	ToolCalls       int64      `json:"tool_calls"`
}

type Ledger struct {
	dir    string
	inner  *jsonlsink.Sink
	logger *slog.Logger

	mu sync.Mutex
}

// New opens (and compacts) the ledger under <workspace>/.remindb/sessions.
func New(workspace string, logger *slog.Logger) (*Ledger, error) {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	dir := filepath.Join(workspace, config.DirName, subDir)

	// growth bounded by compact(), not rotation
	inner, err := jsonlsink.New(dir, 0)
	if err != nil {
		return nil, err
	}

	l := &Ledger{dir: dir, inner: inner, logger: logger}
	l.compact()

	return l, nil
}

func Hash(c ClientMeta, transport string) string {
	payload := strings.Join([]string{c.Name, c.Title, c.Version, c.Protocol, transport}, "\x00")
	return contentid.IdentifyPayload("client", payload)
}

func fileName(c ClientMeta, transport string) string {
	return slug(c.Name) + "-" + Hash(c, transport) + ".jsonl"
}

const slugMaxLen = 40

// slug reduces a self-reported client name to a filesystem-safe prefix.
func slug(name string) string {
	var b strings.Builder
	dash := false

	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}

	s := strings.Trim(b.String(), "-")
	if len(s) > slugMaxLen {
		s = strings.Trim(s[:slugMaxLen], "-")
	}
	if s == "" {
		return "client"
	}

	return s
}

// Append writes one session checkpoint line.
func (l *Ledger) Append(r Record) error {
	line, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("failed to marshal: session record: %w", err)
	}

	line = append(line, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()

	return l.inner.Append(fileName(r.Client, r.Transport), line)
}

// Clients aggregates every client file, collapsing each by session_id.
func (l *Ledger) Clients() ([]ClientLedger, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	paths, err := filepath.Glob(filepath.Join(l.dir, "*.jsonl"))
	if err != nil {
		return nil, fmt.Errorf("failed to list: session ledger: %w", err)
	}
	sort.Strings(paths)

	out := make([]ClientLedger, 0, len(paths))
	for _, p := range paths {
		cl, ok := l.summarizeClient(p)
		if ok {
			out = append(out, cl)
		}
	}

	return out, nil
}

// Client aggregates the single client file whose name carries hash.
func (l *Ledger) Client(hash string) (*ClientLedger, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	matches, err := filepath.Glob(filepath.Join(l.dir, "*-"+hash+".jsonl"))
	if err != nil {
		return nil, fmt.Errorf("failed to list: session ledger: %w", err)
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("unknown client %q", hash)
	}

	cl, ok := l.summarizeClient(matches[0])
	if !ok {
		return nil, fmt.Errorf("unknown client %q", hash)
	}
	return &cl, nil
}

// latestPerSession returns the last checkpoint per session_id, in first-seen order.
func (l *Ledger) latestPerSession(path string) []Record {
	f, err := os.Open(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			l.logger.Warn("failed to open: session ledger", "path", path, "err", err)
		}
		return nil
	}
	defer func() { _ = f.Close() }()

	bySession := map[string]Record{}
	var order []string

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, scanInitialBuf), scanMaxLine)

	for sc.Scan() {
		raw := sc.Bytes()
		if len(raw) == 0 {
			continue
		}

		var r Record
		if err := json.Unmarshal(raw, &r); err != nil {
			l.logger.Warn("skipping corrupt session line", "path", path, "err", err)
			continue
		}

		if _, seen := bySession[r.SessionID]; !seen {
			order = append(order, r.SessionID)
		}

		bySession[r.SessionID] = r
	}

	records := make([]Record, 0, len(order))
	for _, id := range order {
		records = append(records, bySession[id])
	}

	return records
}

// summarizeClient folds one client file's sessions into a single rolled-up ledger.
func (l *Ledger) summarizeClient(path string) (ClientLedger, bool) {
	records := l.latestPerSession(path)
	if len(records) == 0 {
		return ClientLedger{}, false
	}

	cl := ClientLedger{Sessions: len(records)}
	newest := int64(-1)

	for _, r := range records {
		end := r.DisconnectedAt
		if end == 0 {
			end = r.LastSeen
		}

		if d := end - r.ConnectedAt; d > 0 {
			cl.LifetimeSeconds += d
		}

		cl.ToolCalls += r.ToolCalls
		if r.DisconnectedAt > cl.LastDisconnect {
			cl.LastDisconnect = r.DisconnectedAt
		}

		if r.LastSeen >= newest {
			newest = r.LastSeen
			cl.Client = r.Client
			cl.Transport = r.Transport
		}
	}

	cl.Hash = Hash(cl.Client, cl.Transport)
	return cl, true
}

// compact rewrites every client file down to one line per session_id, bounding growth.
func (l *Ledger) compact() {
	paths, err := filepath.Glob(filepath.Join(l.dir, "*.jsonl"))
	if err != nil {
		l.logger.Warn("failed to list: session ledger for compaction", "err", err)
		return
	}

	for _, p := range paths {
		records := l.latestPerSession(p)
		if len(records) == 0 {
			continue
		}

		sort.SliceStable(records, func(i, j int) bool {
			if records[i].ConnectedAt != records[j].ConnectedAt {
				return records[i].ConnectedAt < records[j].ConnectedAt
			}
			return records[i].SessionID < records[j].SessionID
		})

		if err := l.atomicWrite(p, records); err != nil {
			l.logger.Warn("failed to compact session ledger", "path", p, "err", err)
		}
	}
}

// atomicWrite replaces path with records via a temp file + rename.
func (l *Ledger) atomicWrite(path string, records []Record) error {
	var buf bytes.Buffer
	for _, r := range records {
		line, err := json.Marshal(r)
		if err != nil {
			return fmt.Errorf("failed to marshal: session record: %w", err)
		}

		buf.Write(line)
		buf.WriteByte('\n')
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("failed to write: %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("failed to rename: %s: %w", tmp, err)
	}

	return nil
}
