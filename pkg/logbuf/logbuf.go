// Package logbuf provides a bounded in-memory slog ring buffer.
package logbuf

import (
	"context"
	"log/slog"
	"sync"
)

type Record struct {
	Time  int64          `json:"time"`
	Level string         `json:"level"`
	Msg   string         `json:"msg"`
	Attrs map[string]any `json:"attrs"`
}

type Buffer struct {
	mu      sync.Mutex
	records []Record
	size    int
	next    int
	count   int
	dropped int64
}

func NewBuffer(size int) *Buffer {
	return &Buffer{records: make([]Record, size), size: size}
}

func (b *Buffer) append(r Record) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.count == b.size {
		b.dropped++
	}

	b.records[b.next] = r
	b.next = (b.next + 1) % b.size

	if b.count < b.size {
		b.count++
	}
}

// Records returns the buffered records oldest-first (newest last).
func (b *Buffer) Records() []Record {
	b.mu.Lock()
	defer b.mu.Unlock()

	out := make([]Record, 0, b.count)
	if b.count < b.size {
		return append(out, b.records[:b.count]...)
	}
	out = append(out, b.records[b.next:]...)
	return append(out, b.records[:b.next]...)
}

// Dropped returns the number of records evicted by capacity overflow.
func (b *Buffer) Dropped() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.dropped
}

type Handler struct {
	next   slog.Handler
	buf    *Buffer
	prefix string
	attrs  map[string]any
}

func NewHandler(next slog.Handler, buf *Buffer) *Handler {
	return &Handler{next: next, buf: buf}
}

func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	attrs := make(map[string]any, len(h.attrs)+r.NumAttrs())
	for k, v := range h.attrs {
		attrs[k] = v
	}
	r.Attrs(func(a slog.Attr) bool {
		attrs[h.prefix+a.Key] = a.Value.Resolve().Any()
		return true
	})

	h.buf.append(Record{
		Time:  r.Time.UnixMilli(),
		Level: r.Level.String(),
		Msg:   r.Message,
		Attrs: attrs,
	})
	return h.next.Handle(ctx, r)
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
	for k, v := range h.attrs {
		attrs[k] = v
	}
	return &Handler{next: h.next, buf: h.buf, prefix: h.prefix, attrs: attrs}
}
