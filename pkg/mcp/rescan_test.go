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
	"github.com/radimsem/remindb/pkg/compiler"
)

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
	r.now = func() time.Time { return time.Now().Add(time.Hour) }
	r.scan(context.Background())

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
	r.now = func() time.Time { return time.Now().Add(time.Hour) }
	r.scan(context.Background())

	for path := range r.modTimes {
		if filepath.Base(filepath.Dir(path)) == ".obsidian" {
			t.Errorf("indexed hidden path: %s", path)
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
	r.now = func() time.Time { return time.Now().Add(time.Hour) }

	ctx := context.Background()

	// First scan compiles doc.md into the DB.
	r.scan(ctx)
	nodes, _ := st.GetAllNodes(ctx)
	for _, n := range nodes {
		if strings.Contains(n.Content, "Updated") {
			t.Fatalf("unexpected updated content before edit: %q", n.Content)
		}
	}

	// Modify the file (bump mtime).
	time.Sleep(10 * time.Millisecond)
	writeFile(t, dir, "doc.md", "# Updated\n\nNew content.\n")

	r.scan(ctx)
	nodes, _ = st.GetAllNodes(ctx)

	foundUpdated := false
	for _, n := range nodes {
		if strings.Contains(n.Content, "New content") {
			foundUpdated = true
		}
	}
	if !foundUpdated {
		t.Error("expected updated content after recompile")
	}
}

func TestRescanLoop_DebouncesMidSave(t *testing.T) {
	dir := t.TempDir()

	st := testutil.OpenTestDB(t)
	r := NewRescanLoop(st, dir, time.Minute, nil)

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

	ctx := context.Background()
	r.scan(ctx)

	// Add a new file after the first scan.
	writeFile(t, dir, "new.md", "# New\n\nContent.\n")

	r.scan(ctx)

	roots, _ := st.GetRootNodes(ctx)
	if len(roots) == 0 {
		t.Error("expected nodes from new file")
	}
}

func TestRescanLoop_ScanBlocksOnOpMu(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "doc.md", "# Doc\n\nBody.\n")

	st := testutil.OpenTestDB(t)
	r := NewRescanLoop(st, dir, time.Minute, nil)
	r.now = func() time.Time { return time.Now().Add(time.Hour) }

	st.OpMu.Lock()

	done := make(chan struct{})
	go func() {
		r.scan(context.Background())
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("scan completed while OpMu was held")
	case <-time.After(50 * time.Millisecond):
	}

	st.OpMu.Unlock()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("scan did not complete after OpMu.Unlock")
	}
}

func TestRescanLoop_RunCatchesStaleEditsAtStartup(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "doc.md", "# Before\n\nOriginal body.\n")

	st := testutil.OpenTestDB(t)
	ctx := context.Background()

	if _, err := compiler.CompileDir(ctx, st, dir, "initial"); err != nil {
		t.Fatalf("CompileDir: %v", err)
	}

	// User edits the file while serve was offline.
	writeFile(t, dir, "doc.md", "# Before\n\nUpdated body.\n")

	// Long interval — ticker must not fire during the test, so only the
	// startup reconcile can catch the edit.
	r := NewRescanLoop(st, dir, time.Hour, nil)
	r.now = func() time.Time { return time.Now().Add(time.Hour) }

	runCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	r.Run(runCtx)

	nodes, err := st.GetAllNodes(ctx)
	if err != nil {
		t.Fatalf("GetAllNodes: %v", err)
	}

	foundUpdated := false
	for _, n := range nodes {
		if strings.Contains(n.Content, "Original body") {
			t.Errorf("stale content persisted after startup reconcile: %q", n.Content)
		}
		if strings.Contains(n.Content, "Updated body") {
			foundUpdated = true
		}
	}
	if !foundUpdated {
		t.Error("startup reconcile did not apply edit made while serve was down")
	}
}

func TestMaybeInitialCompile_EmptyDB(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.md", "# A\n\nBody.\n")
	writeFile(t, dir, "b.md", "# B\n\nBody.\n")

	st := testutil.OpenTestDB(t)
	ctx := context.Background()

	if err := MaybeInitialCompile(ctx, st, dir, nil); err != nil {
		t.Fatalf("MaybeInitialCompile: %v", err)
	}

	roots, err := st.GetRootNodes(ctx)
	if err != nil {
		t.Fatalf("GetRootNodes: %v", err)
	}
	if len(roots) == 0 {
		t.Error("expected nodes to be compiled on empty DB")
	}
}

func TestMaybeInitialCompile_NonEmptyDB(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.md", "# A\n\nBody.\n")

	st := testutil.OpenTestDB(t)
	ctx := context.Background()

	// Pre-populate so DB is not empty.
	if err := MaybeInitialCompile(ctx, st, dir, nil); err != nil {
		t.Fatalf("seed compile: %v", err)
	}

	before, err := st.GetRootNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Add a new file, call MaybeInitialCompile again — must skip.
	writeFile(t, dir, "new.md", "# New\n")

	if err := MaybeInitialCompile(ctx, st, dir, nil); err != nil {
		t.Fatalf("MaybeInitialCompile: %v", err)
	}

	after, err := st.GetRootNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Errorf("root count changed: before=%d after=%d (MaybeInitialCompile ran on non-empty DB)", len(before), len(after))
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
