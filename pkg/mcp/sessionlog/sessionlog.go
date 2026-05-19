// Package sessionlog tees a session's records into an append-only per-session file under .remindb/logs/.
package sessionlog

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/radimsem/remindb/pkg/config"
)

const (
	subDir = "logs"

	// Tool-call trace messages emitted by tools.Deps.logCall.
	MsgToolCall       = "mcp call"
	MsgToolCallFailed = "mcp call failed"

	slugMaxLen = 64
)

type Record struct {
	Time   time.Time      `json:"time"`
	Level  string         `json:"level"`
	Msg    string         `json:"msg"`
	Fields map[string]any `json:"fields,omitempty"`
}

type ctxKey struct{}

// NewContext returns ctx carrying the canonical session id the registry resolved.
func NewContext(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

func FromContext(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

type Sink struct {
	dir         string
	maxFileSize int64

	mu sync.Mutex
}

// New creates <workspace>/.remindb/logs and returns a sink bounded by maxFileSize bytes.
func New(workspace string, maxFileSize int64) (*Sink, error) {
	dir := Dir(workspace)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create: session logs dir: %w", err)
	}

	return &Sink{dir: dir, maxFileSize: maxFileSize}, nil
}

// Dir returns the per-session log directory under workspace.
func Dir(workspace string) string {
	return filepath.Join(workspace, config.DirName, subDir)
}

// Write appends line to the file keyed by sessionID, rotating once when the cap is reached.
func (s *Sink) Write(sessionID string, line []byte) error {
	path := filepath.Join(s.dir, Slug(sessionID)+".log")

	s.mu.Lock()
	defer s.mu.Unlock()

	// Single-generation rotation: the prior .1 is intentionally discarded.
	fi, statErr := os.Stat(path)
	overCap := statErr == nil && fi.Size() > 0 &&
		int64(len(line)) <= s.maxFileSize &&
		fi.Size()+int64(len(line)) > s.maxFileSize

	if overCap {
		if err := os.Rename(path, path+".1"); err != nil {
			return fmt.Errorf("failed to rotate: session log %s: %w", path, err)
		}
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open: session log %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Write(line); err != nil {
		return fmt.Errorf("failed to write: session log %s: %w", path, err)
	}
	return nil
}

// Slug reduces a session id to its filesystem-safe logfile stem; ids are UUID/hex in practice.
func Slug(id string) string {
	var b strings.Builder

	for _, r := range id {
		if isSafeSlugRune(r) {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}

	s := b.String()
	if len(s) > slugMaxLen {
		s = s[:slugMaxLen]
	}

	if s == "" {
		return "session"
	}
	return s
}

// isSafeSlugRune reports whether r is filesystem-safe in a session-log filename.
func isSafeSlugRune(r rune) bool {
	return r >= 'a' && r <= 'z' ||
		r >= 'A' && r <= 'Z' ||
		r >= '0' && r <= '9' ||
		r == '.' || r == '_' || r == '-'
}

type Handler struct {
	next   slog.Handler
	sink   *Sink
	prefix string
	attrs  map[string]any
}

// NewHandler wraps next as the outermost handler, teeing captured records to sink.
func NewHandler(next slog.Handler, sink *Sink) *Handler {
	return &Handler{next: next, sink: sink}
}

// Enabled stays permissive: the session file captures Debug tool traces even when the shared stream is at Info.
func (h *Handler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	if sid := FromContext(ctx); sid != "" && h.captures(r) {
		_ = h.sink.Write(sid, h.render(r))
	}

	if h.next.Enabled(ctx, r.Level) {
		return h.next.Handle(ctx, r)
	}
	return nil
}

func (h *Handler) captures(r slog.Record) bool {
	return r.Level >= slog.LevelWarn || r.Message == MsgToolCall || r.Message == MsgToolCallFailed
}

// render serializes one record as a single append-only JSON line.
func (h *Handler) render(r slog.Record) []byte {
	rec := Record{Time: r.Time, Level: r.Level.String(), Msg: r.Message}

	fields := make(map[string]any, len(h.attrs)+r.NumAttrs())
	for k, v := range h.attrs {
		if !isEmptyAttrValue(v) {
			fields[k] = jsonable(v)
		}
	}

	r.Attrs(func(a slog.Attr) bool {
		v := a.Value.Resolve().Any()
		if !isEmptyAttrValue(v) {
			fields[h.prefix+a.Key] = jsonable(v)
		}
		return true
	})
	if len(fields) > 0 {
		rec.Fields = fields
	}

	line, err := json.Marshal(rec)
	if err != nil {
		// An attr value json can't encode is rare here.
		rec.Fields = nil
		line, _ = json.Marshal(rec)
	}

	return append(line, '\n')
}

// ParseLog reads JSONL session-log records in append order.
func ParseLog(r io.Reader) ([]Record, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)

	var recs []Record
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}

		var rec Record
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}

		recs = append(recs, rec)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("failed to read: session log: %w", err)
	}

	return recs, nil
}

// jsonable replaces values whose JSON encoding would lose their meaning.
func jsonable(v any) any {
	if e, ok := v.(error); ok {
		return e.Error()
	}
	return v
}

func isEmptyAttrValue(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	default:
		return false
	}
}

func (h *Handler) WithAttrs(as []slog.Attr) slog.Handler {
	nh := h.clone()
	nh.next = h.next.WithAttrs(as)

	for _, a := range as {
		nh.attrs[h.prefix+a.Key] = a.Value.Resolve().Any()
	}

	return nh
}

func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}

	nh := h.clone()
	nh.next = h.next.WithGroup(name)
	nh.prefix = h.prefix + name + "."

	return nh
}

func (h *Handler) clone() *Handler {
	attrs := make(map[string]any, len(h.attrs))
	maps.Copy(attrs, h.attrs)

	return &Handler{next: h.next, sink: h.sink, prefix: h.prefix, attrs: attrs}
}
