// Package sessionlog tees a session's records into an append-only per-session file under .remindb/logs/.
package sessionlog

import (
	"bytes"
	"context"
	"fmt"
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
	dir := filepath.Join(workspace, config.DirName, subDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create: session logs dir: %w", err)
	}

	return &Sink{dir: dir, maxFileSize: maxFileSize}, nil
}

// Write appends line to the file keyed by sessionID, rotating once when the cap is reached.
func (s *Sink) Write(sessionID string, line []byte) error {
	path := filepath.Join(s.dir, slug(sessionID)+".log")

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

// slug reduces a session id to a filesystem-safe name; ids are UUID/hex in practice.
func slug(id string) string {
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

// render formats one record as a single append-only line: time, level, msg, then key=value attrs.
func (h *Handler) render(r slog.Record) []byte {
	var b bytes.Buffer
	b.WriteString(r.Time.Format(time.RFC3339))
	b.WriteByte(' ')
	b.WriteString(r.Level.String())
	b.WriteByte(' ')
	b.WriteString(r.Message)

	for k, v := range h.attrs {
		if isEmptyAttrValue(v) {
			continue
		}

		fmt.Fprintf(&b, " %s=%v", k, v)
	}

	r.Attrs(func(a slog.Attr) bool {
		v := a.Value.Resolve().Any()
		if isEmptyAttrValue(v) {
			return true
		}

		fmt.Fprintf(&b, " %s%s=%v", h.prefix, a.Key, v)
		return true
	})
	b.WriteByte('\n')

	return b.Bytes()
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
