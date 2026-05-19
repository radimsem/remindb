package sessionlog

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

	path := filepath.Join(ws, config.DirName, subDir, slug(id)+".log")
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
	logger.ErrorContext(ctx, MsgToolCallFailed, "tool", "MemorySearch", "err", "boom")
	logger.WarnContext(ctx, "failed to boost: access", "count", 3)

	got := readSessionLog(t, ws, "sess-abc")

	for _, want := range []string{MsgToolCall, "MemoryWrite", "payload_bytes=20", MsgToolCallFailed, "err=boom", "failed to boost: access"} {
		if !strings.Contains(got, want) {
			t.Errorf("session log missing %q\n--- log ---\n%s", want, got)
		}
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

func TestSink_OversizedLineDoesNotInfiniteRotate(t *testing.T) {
	ws := t.TempDir()
	sink, err := New(ws, 8)
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

	// Without the guard the active file would hold exactly one line; with it,
	// successive oversized lines accumulate (rotation only fires once the
	// file is non-empty and the next line would cross the cap).
	if fi.Size() <= int64(len(line)) {
		t.Errorf("active log = %d bytes, want > one line (infinite-rotation regression)", fi.Size())
	}
}

func TestSlug_FilesystemSafe(t *testing.T) {
	for in, want := range map[string]string{
		"abc-123":               "abc-123",
		"a/b:c":                 "a-b-c",
		"":                      "session",
		strings.Repeat("x", 80): strings.Repeat("x", slugMaxLen),
	} {
		if got := slug(in); got != want {
			t.Errorf("slug(%q) = %q, want %q", in, got, want)
		}
	}
}
