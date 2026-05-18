package logbuf

import (
	"context"
	"io"
	"log/slog"
	"strconv"
	"testing"
)

func newTestLogger(size int) (*slog.Logger, *Buffer) {
	buf := NewBuffer(size)
	h := NewHandler(slog.NewTextHandler(io.Discard, nil), buf)
	return slog.New(h), buf
}

func msgs(recs []Record) []string {
	out := make([]string, len(recs))
	for i, r := range recs {
		out[i] = r.Msg
	}
	return out
}

func TestCaptureAndOrder(t *testing.T) {
	log, buf := newTestLogger(10)
	for i := range 3 {
		log.Info("m" + strconv.Itoa(i))
	}

	recs := buf.Records()
	if got := msgs(recs); len(got) != 3 || got[0] != "m0" || got[2] != "m2" {
		t.Fatalf("records: got %v, want [m0 m1 m2] (newest last)", got)
	}
	if recs[0].Level != "INFO" {
		t.Errorf("level: got %q, want INFO", recs[0].Level)
	}
	if recs[0].Time == 0 {
		t.Errorf("time not captured")
	}
	if buf.Dropped() != 0 {
		t.Errorf("dropped: got %d, want 0", buf.Dropped())
	}
}

func TestBoundedEviction(t *testing.T) {
	log, buf := newTestLogger(3)
	for i := range 5 {
		log.Info("m" + strconv.Itoa(i))
	}

	got := msgs(buf.Records())
	want := []string{"m2", "m3", "m4"}
	if len(got) != 3 || got[0] != want[0] || got[2] != want[2] {
		t.Fatalf("records: got %v, want %v", got, want)
	}
	if buf.Dropped() != 2 {
		t.Errorf("dropped: got %d, want 2", buf.Dropped())
	}
}

func TestCustomSizeOne(t *testing.T) {
	log, buf := newTestLogger(1)
	for i := range 4 {
		log.Info("m" + strconv.Itoa(i))
	}

	got := msgs(buf.Records())
	if len(got) != 1 || got[0] != "m3" {
		t.Fatalf("records: got %v, want [m3]", got)
	}
	if buf.Dropped() != 3 {
		t.Errorf("dropped: got %d, want 3", buf.Dropped())
	}
}

func TestAttrsCaptured(t *testing.T) {
	log, buf := newTestLogger(10)
	log.With("service", "remindb").Info("hello", "node_id", "n1")

	recs := buf.Records()
	if len(recs) != 1 {
		t.Fatalf("records: got %d, want 1", len(recs))
	}
	a := recs[0].Attrs
	if a["service"] != "remindb" {
		t.Errorf("WithAttrs key: got %v, want remindb", a["service"])
	}
	if a["node_id"] != "n1" {
		t.Errorf("record attr: got %v, want n1", a["node_id"])
	}
}

func TestAttrsAlwaysNonNil(t *testing.T) {
	log, buf := newTestLogger(10)
	log.Info("no attrs")

	if a := buf.Records()[0].Attrs; a == nil {
		t.Fatal("attrs is nil; want empty map")
	}
}

func TestEnabledDelegates(t *testing.T) {
	buf := NewBuffer(4)
	h := NewHandler(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelWarn}), buf)

	if h.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("Info should be disabled when base level is Warn")
	}
	if !h.Enabled(context.Background(), slog.LevelError) {
		t.Error("Error should be enabled when base level is Warn")
	}
}
