package mcp

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/radimsem/remindb/internal/testutil"
)

func TestRescanLoop_SeedMtimes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.md", "# Hello\n")
	writeFile(t, dir, "b.json", `{"key": "value"}`)
	writeFile(t, dir, "skip.txt", "not supported")

	st := testutil.OpenTestDB(t)
	r := NewRescanLoop(st, dir, time.Minute, nil)
	r.seedMtimes()

	if len(r.modTimes) != 2 {
		t.Errorf("mtimes = %d, want 2 (md + json)", len(r.modTimes))
	}
}

func TestRescanLoop_LogsWalkErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "ok.md", "# OK\n")

	unreadable := filepath.Join(dir, "nope")
	if err := os.MkdirAll(unreadable, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o755) })

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	st := testutil.OpenTestDB(t)
	r := NewRescanLoop(st, dir, time.Minute, logger)
	r.seedMtimes()

	if !strings.Contains(buf.String(), "level=WARN") {
		t.Errorf("expected WARN log for walk error, got: %q", buf.String())
	}
	if !strings.Contains(buf.String(), "nope") {
		t.Errorf("expected path %q in log, got: %q", "nope", buf.String())
	}
}

func TestRescanLoop_SkipsHiddenDirs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "notes.md", "# Notes\n")

	hidden := filepath.Join(dir, ".obsidian")
	if err := os.MkdirAll(hidden, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, hidden, "prefs.json", `{"hidden": true}`)

	st := testutil.OpenTestDB(t)
	r := NewRescanLoop(st, dir, time.Minute, nil)
	r.seedMtimes()

	for path := range r.modTimes {
		if filepath.Base(filepath.Dir(path)) == ".obsidian" {
			t.Errorf("seeded hidden path: %s", path)
		}
	}
	if len(r.modTimes) != 1 {
		t.Errorf("mtimes = %d, want 1 (notes.md only)", len(r.modTimes))
	}
}

func TestRescanLoop_DetectsChanges(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "doc.md", "# Original\n\nContent.\n")

	st := testutil.OpenTestDB(t)
	r := NewRescanLoop(st, dir, time.Minute, nil)

	// Pin the clock past the settle window so writes count as settled.
	r.now = func() time.Time { return time.Now().Add(time.Hour) }
	r.seedMtimes()

	ctx := context.Background()

	// First scan — no changes since seed.
	r.scan(ctx)
	roots, _ := st.GetRootNodes(ctx)
	if len(roots) != 0 {
		t.Errorf("expected no nodes after seed+scan, got %d", len(roots))
	}

	// Modify the file (bump mtime).
	time.Sleep(10 * time.Millisecond)
	writeFile(t, dir, "doc.md", "# Updated\n\nNew content.\n")

	r.scan(ctx)
	roots, _ = st.GetRootNodes(ctx)
	if len(roots) == 0 {
		t.Error("expected nodes after recompile")
	}
}

func TestRescanLoop_DebouncesMidSave(t *testing.T) {
	dir := t.TempDir()

	st := testutil.OpenTestDB(t)
	r := NewRescanLoop(st, dir, time.Minute, nil)
	r.seedMtimes()

	// Freeze "now" so the file's mtime is always inside the settle window.
	frozen := time.Now()
	r.now = func() time.Time { return frozen }
	writeFile(t, dir, "doc.md", "# Fresh\n\nContent.\n")

	ctx := context.Background()
	r.scan(ctx)
	roots, _ := st.GetRootNodes(ctx)
	if len(roots) != 0 {
		t.Errorf("expected no nodes — file is still settling, got %d", len(roots))
	}

	// Advance past the settle window; now the file should be compiled.
	r.now = func() time.Time { return frozen.Add(r.settle + time.Second) }
	r.scan(ctx)
	roots, _ = st.GetRootNodes(ctx)
	if len(roots) == 0 {
		t.Error("expected nodes after file has settled")
	}
}

func TestRescanLoop_CommitsMtimesOnlyAfterSuccess(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "ok.md", "# OK\n")
	writeFile(t, dir, "bad.json", `{"unterminated`)

	st := testutil.OpenTestDB(t)
	r := NewRescanLoop(st, dir, time.Minute, nil)
	r.now = func() time.Time { return time.Now().Add(time.Hour) }

	ctx := context.Background()
	r.scan(ctx)

	if len(r.modTimes) != 0 {
		t.Errorf("mtimes = %d, want 0 (compile failed, nothing committed)", len(r.modTimes))
	}

	writeFile(t, dir, "bad.json", `{"valid": "now"}`)

	r.scan(ctx)

	if len(r.modTimes) != 2 {
		t.Errorf("mtimes = %d, want 2 (both files compiled on retry)", len(r.modTimes))
	}
}

func TestRescanLoop_ReconcilesDeletedFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "keep.md", "# Keep\n")
	writeFile(t, dir, "gone.md", "# Gone\n\nBody.\n")

	st := testutil.OpenTestDB(t)
	r := NewRescanLoop(st, dir, time.Minute, nil)
	r.now = func() time.Time { return time.Now().Add(time.Hour) }

	ctx := context.Background()

	// First scan populates both.
	r.scan(ctx)

	before, err := st.GetNodesByFile(ctx, "gone.md")
	if err != nil {
		t.Fatalf("GetNodesByFile: %v", err)
	}
	if len(before) == 0 {
		t.Fatal("expected gone.md nodes after initial scan")
	}

	// Delete the file from disk.
	if err := os.Remove(filepath.Join(dir, "gone.md")); err != nil {
		t.Fatal(err)
	}

	// Rescan should purge the orphaned nodes.
	r.scan(ctx)

	after, err := st.GetNodesByFile(ctx, "gone.md")
	if err != nil {
		t.Fatalf("GetNodesByFile: %v", err)
	}
	if len(after) != 0 {
		t.Errorf("orphan nodes after delete = %d, want 0", len(after))
	}

	kept, err := st.GetNodesByFile(ctx, "keep.md")
	if err != nil {
		t.Fatalf("GetNodesByFile: %v", err)
	}
	if len(kept) == 0 {
		t.Error("keep.md nodes should remain")
	}
}

func TestRescanLoop_NewFile(t *testing.T) {
	dir := t.TempDir()

	st := testutil.OpenTestDB(t)
	r := NewRescanLoop(st, dir, time.Minute, nil)
	r.now = func() time.Time { return time.Now().Add(time.Hour) }
	r.seedMtimes()

	// Add a new file after seed.
	writeFile(t, dir, "new.md", "# New\n\nContent.\n")

	ctx := context.Background()
	r.scan(ctx)

	roots, _ := st.GetRootNodes(ctx)
	if len(roots) == 0 {
		t.Error("expected nodes from new file")
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
