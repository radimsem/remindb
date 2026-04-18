package mcp

import (
	"context"
	"os"
	"path/filepath"
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
	r := NewRescanLoop(st, dir, time.Minute)
	r.seedMtimes()

	if len(r.modTimes) != 2 {
		t.Errorf("mtimes = %d, want 2 (md + json)", len(r.modTimes))
	}
}

func TestRescanLoop_DetectsChanges(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "doc.md", "# Original\n\nContent.\n")

	st := testutil.OpenTestDB(t)
	r := NewRescanLoop(st, dir, time.Minute)

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
	r := NewRescanLoop(st, dir, time.Minute)
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

func TestRescanLoop_NewFile(t *testing.T) {
	dir := t.TempDir()

	st := testutil.OpenTestDB(t)
	r := NewRescanLoop(st, dir, time.Minute)
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
