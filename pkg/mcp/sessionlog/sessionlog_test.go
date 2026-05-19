package sessionlog

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/radimsem/remindb/pkg/config"
)

func newTestLogger(t *testing.T, ws string, maxFileSize int64) (*slog.Logger, *strings.Builder) {
	t.Helper()

	sink, err := New(ws, maxFileSize)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var shared strings.Builder
	base := slog.NewTextHandler(&shared, &slog.HandlerOptions{Level: slog.LevelInfo})

	return slog.New(NewHandler(base, sink)), &shared
}

func readSessionLog(t *testing.T, ws, id string) string {
	t.Helper()

	path := filepath.Join(ws, config.DirName, subDir, Slug(id)+".log")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	return string(data)
}

func TestHandler_CapturesToolTraceAndErrorsExcludesPayload(t *testing.T) {
	ws := t.TempDir()
	logger, shared := newTestLogger(t, ws, 1<<20)

	const secret = "TOP-SECRET-NODE-BODY"
	ctx := NewContext(context.Background(), "sess-abc")

	// Successful tool call: logged at Debug, must still reach the session file
	// even though the shared stream sits at Info. Only payload_bytes, never body.
	logger.DebugContext(ctx, MsgToolCall, "tool", "MemoryWrite", "elapsed_ms", 4, "payload_bytes", len(secret))

	// A real error value, exactly as logCall passes it (*errp). Its message
	// must survive JSON encoding — json.Marshal of an error alone yields "{}".
	logger.ErrorContext(ctx, MsgToolCallFailed, "tool", "MemorySearch", "err", errors.New("boom"))
	logger.WarnContext(ctx, "failed to boost: access", "count", 3)

	got := readSessionLog(t, ws, "sess-abc")

	recs, err := ParseLog(strings.NewReader(got))
	if err != nil {
		t.Fatalf("ParseLog: %v", err)
	}
	if len(recs) != 3 {
		t.Fatalf("parsed %d records, want 3\n--- log ---\n%s", len(recs), got)
	}

	if recs[0].Msg != MsgToolCall || recs[0].Fields["tool"] != "MemoryWrite" {
		t.Errorf("record 0 = %+v, want the MemoryWrite tool trace", recs[0])
	}
	if pb := recs[0].Fields["payload_bytes"]; pb != float64(len(secret)) {
		t.Errorf("payload_bytes = %v, want %d (count, not body)", pb, len(secret))
	}

	if recs[1].Msg != MsgToolCallFailed || recs[1].Fields["err"] != "boom" {
		t.Errorf("record 1 = %+v, want the failed-call error record", recs[1])
	}

	if recs[2].Msg != "failed to boost: access" || recs[2].Level != "WARN" {
		t.Errorf("record 2 = %+v, want the Warn record", recs[2])
	}
	if strings.Contains(got, secret) {
		t.Errorf("session log leaked payload body %q\n--- log ---\n%s", secret, got)
	}

	// Shared stream keeps its Info gate: the Debug trace (the only line
	// carrying MemoryWrite/payload_bytes) must not be there.
	if strings.Contains(shared.String(), "MemoryWrite") || strings.Contains(shared.String(), "payload_bytes") {
		t.Errorf("shared stream should not contain the Debug tool trace, got:\n%s", shared.String())
	}
	if !strings.Contains(shared.String(), MsgToolCallFailed) {
		t.Errorf("shared stream should still contain the Error record, got:\n%s", shared.String())
	}
}

func TestHandler_NoSessionInContextWritesNoFile(t *testing.T) {
	ws := t.TempDir()
	logger, _ := newTestLogger(t, ws, 1<<20)

	logger.ErrorContext(context.Background(), MsgToolCallFailed, "tool", "MemoryStats")

	entries, err := os.ReadDir(filepath.Join(ws, config.DirName, subDir))
	if err != nil {
		t.Fatalf("read logs dir: %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("expected no session files without a session id, got %d", len(entries))
	}
}

func TestHandler_DistinctSessionsGetDistinctFiles(t *testing.T) {
	ws := t.TempDir()
	logger, _ := newTestLogger(t, ws, 1<<20)

	logger.ErrorContext(NewContext(context.Background(), "sess-1"), MsgToolCallFailed, "tool", "MemoryFetch")
	logger.ErrorContext(NewContext(context.Background(), "sess-2"), MsgToolCallFailed, "tool", "MemoryTree")

	if s := readSessionLog(t, ws, "sess-1"); !strings.Contains(s, "MemoryFetch") || strings.Contains(s, "MemoryTree") {
		t.Errorf("sess-1 file wrong:\n%s", s)
	}
	if s := readSessionLog(t, ws, "sess-2"); !strings.Contains(s, "MemoryTree") || strings.Contains(s, "MemoryFetch") {
		t.Errorf("sess-2 file wrong:\n%s", s)
	}
}

func TestSink_RotatesAtCap(t *testing.T) {
	ws := t.TempDir()
	sink, err := New(ws, 64)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	for range 20 {
		if err := sink.Write("sess-rot", []byte("0123456789abcdef\n")); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	base := filepath.Join(ws, config.DirName, subDir, "sess-rot.log")
	if _, err := os.Stat(base); err != nil {
		t.Errorf("active log missing: %v", err)
	}
	if _, err := os.Stat(base + ".1"); err != nil {
		t.Errorf("rotated log missing: %v", err)
	}
}

func TestSink_OversizedLineRotatesInsteadOfGrowing(t *testing.T) {
	ws := t.TempDir()
	sink, err := New(ws, 8) // cap smaller than a single record — the old footgun
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	line := []byte("this single line is way over the cap\n")
	for range 5 {
		if err := sink.Write("sess-big", line); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	base := filepath.Join(ws, config.DirName, subDir, "sess-big.log")
	fi, err := os.Stat(base)
	if err != nil {
		t.Fatalf("active log missing: %v", err)
	}

	// Fixed centrally in jsonlsink: a lone line larger than the cap no longer silently disables rotation.
	if fi.Size() != int64(len(line)) {
		t.Errorf("active log = %d bytes, want %d (one line; growth bounded)", fi.Size(), len(line))
	}
	if _, err := os.Stat(base + ".1"); err != nil {
		t.Errorf("expected rotated .1 to exist: %v", err)
	}
}

func TestRoundTrip(t *testing.T) {
	h := &Handler{}
	when := time.Date(2026, 5, 19, 12, 34, 56, 789, time.UTC)

	levels := map[string]slog.Level{
		"DEBUG": slog.LevelDebug, "INFO": slog.LevelInfo,
		"WARN": slog.LevelWarn, "ERROR": slog.LevelError,
	}
	cases := []Record{
		{Time: when, Level: "INFO", Msg: "mcp call", Fields: map[string]any{
			"tool": "MemorySearch", "query": "a=b c d", "elapsed_ms": float64(7),
		}},
		{Time: when, Level: "ERROR", Msg: "", Fields: map[string]any{
			"err": "boom: spaces, equals = and 日本語\nsecond line",
		}},
		{Time: when, Level: "DEBUG", Msg: "no fields"},
	}

	var buf bytes.Buffer
	for _, c := range cases {
		r := slog.NewRecord(c.Time, levels[c.Level], c.Msg, 0)
		for k, v := range c.Fields {
			r.AddAttrs(slog.Any(k, v))
		}

		buf.Write(h.render(r))
	}

	got, err := ParseLog(&buf)
	if err != nil {
		t.Fatalf("ParseLog: %v", err)
	}
	if len(got) != len(cases) {
		t.Fatalf("round-tripped %d records, want %d", len(got), len(cases))
	}

	for i, want := range cases {
		g := got[i]
		if !g.Time.Equal(want.Time) || g.Level != want.Level || g.Msg != want.Msg {
			t.Errorf("record %d header = {%v %q %q}, want {%v %q %q}",
				i, g.Time, g.Level, g.Msg, want.Time, want.Level, want.Msg)
		}

		if len(g.Fields) != len(want.Fields) {
			t.Errorf("record %d fields = %v, want %v", i, g.Fields, want.Fields)
			continue
		}

		for k, wv := range want.Fields {
			if g.Fields[k] != wv {
				t.Errorf("record %d field %q = %v (%T), want %v (%T)",
					i, k, g.Fields[k], g.Fields[k], wv, wv)
			}
		}
	}
}

type stringerAttr struct{ s string }

func (a stringerAttr) String() string { return a.s }

// A fmt.Stringer-only attr (no error, no json.Marshaler) must reach the
// session file as its String() form, matching the shared text handler —
// not as marshalled exported fields or "{}".
func TestRoundTrip_StringerAttr(t *testing.T) {
	h := &Handler{}
	when := time.Date(2026, 5, 19, 12, 34, 56, 789, time.UTC)

	r := slog.NewRecord(when, slog.LevelInfo, MsgToolCall, 0)
	r.AddAttrs(slog.Any("dur", stringerAttr{"1.5s"}))

	var buf bytes.Buffer
	buf.Write(h.render(r))

	got, err := ParseLog(&buf)
	if err != nil {
		t.Fatalf("ParseLog: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("round-tripped %d records, want 1", len(got))
	}

	if g := got[0].Fields["dur"]; g != "1.5s" {
		t.Errorf("dur = %v (%T), want \"1.5s\" (string) — Stringer must coerce via String()", g, g)
	}
}

func TestSlug_FilesystemSafe(t *testing.T) {
	for in, want := range map[string]string{
		"abc-123":               "abc-123",
		"a/b:c":                 "a-b-c",
		"":                      "session",
		strings.Repeat("x", 80): strings.Repeat("x", slugMaxLen),
	} {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}
