package compiler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/radimsem/remindb/internal/ignore"
	"github.com/radimsem/remindb/internal/tempfile"
	"github.com/radimsem/remindb/internal/testutil"
	"github.com/radimsem/remindb/pkg/config"
	"github.com/radimsem/remindb/pkg/store"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func writeTempfile(t *testing.T, dir, content string) {
	t.Helper()

	stateDir := filepath.Join(dir, config.DirName)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(stateDir, tempfile.FileName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeIgnoreFile(t *testing.T, dir, content string) {
	t.Helper()

	stateDir := filepath.Join(dir, config.DirName)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(stateDir, ignore.FileName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCompile(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	p := writeFile(t, dir, "doc.md", "# Hello\n\nSome content here.\n")

	result, err := Compile(ctx, st, WithPaths([]string{p}), WithMessage("initial"))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if result.Added == 0 {
		t.Error("expected nodes added")
	}
	if result.Total == 0 {
		t.Error("expected total > 0")
	}

	// Verify nodes in DB.
	roots, err := st.GetRootNodes(ctx)
	if err != nil {
		t.Fatalf("GetRootNodes: %v", err)
	}
	if len(roots) == 0 {
		t.Error("no root nodes in DB")
	}

	// Verify snapshot created.
	snaps, _ := st.ListSnapshots(ctx, 10)
	if len(snaps) != 1 {
		t.Errorf("snapshots = %d, want 1", len(snaps))
	}
}

func TestCompileDir(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	writeFile(t, dir, "a.md", "# First\n\nContent A.\n")
	writeFile(t, dir, "b.json", `{"key": "value"}`)
	writeFile(t, dir, "skip.txt", "not a supported format")

	result, err := CompileDir(ctx, st, dir, "batch")
	if err != nil {
		t.Fatalf("CompileDir: %v", err)
	}
	if result.Added < 2 {
		t.Errorf("Added = %d, want >= 2 (from 2 files)", result.Added)
	}
}

func TestCompile_TotalEqualsSumOfOps(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	p := writeFile(t, dir, "doc.md", "# Hi\n\nA.\n\nB.\n")

	first, err := Compile(ctx, st, WithPaths([]string{p}), WithMessage("v1"))
	if err != nil {
		t.Fatalf("Compile v1: %v", err)
	}

	want := first.Added + first.Modified + first.Removed
	if first.Total != want {
		t.Errorf("Total = %d, want %d (added+modified+removed on first compile)", first.Total, want)
	}

	writeFile(t, dir, "doc.md", "# Hi\n\nA edited.\n\nB.\n")
	second, err := Compile(ctx, st, WithPaths([]string{p}), WithMessage("v2"))
	if err != nil {
		t.Fatalf("Compile v2: %v", err)
	}

	want = second.Added + second.Modified + second.Removed
	if second.Total != want {
		t.Errorf("Total = %d, want %d (added+modified+removed on recompile)", second.Total, want)
	}
}

func TestCompile_Recompile(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	p := writeFile(t, dir, "doc.md", "# Hello\n\nOriginal content.\n")

	_, err := Compile(ctx, st, WithPaths([]string{p}), WithMessage("v1"))
	if err != nil {
		t.Fatalf("Compile v1: %v", err)
	}

	// Modify and recompile.
	writeFile(t, dir, "doc.md", "# Hello\n\nUpdated content.\n")

	result, err := Compile(ctx, st, WithPaths([]string{p}), WithMessage("v2"))
	if err != nil {
		t.Fatalf("Compile v2: %v", err)
	}

	snaps, _ := st.ListSnapshots(ctx, 10)
	if len(snaps) != 2 {
		t.Errorf("snapshots = %d, want 2", len(snaps))
	}

	if result.Modified == 0 {
		t.Errorf("Modified = %d, want > 0 (content edit at stable structural position)", result.Modified)
	}
	if result.Added != 0 || result.Removed != 0 {
		t.Errorf("Added = %d, Removed = %d, want both 0 (no insertions or deletions)", result.Added, result.Removed)
	}
}

func TestCompile_SingleFileRescanAfterBatch(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	// Batch compile multiple files — ALERT is not the first walked alphabetically.
	writeFile(t, dir, "ALERT.md", "# Alert\n\nUrgent line.\n")
	writeFile(t, dir, "other1.md", "# Other 1\n\nSome body.\n")
	writeFile(t, dir, "other2.md", "# Other 2\n\nMore body.\n")

	if _, err := CompileDir(ctx, st, dir, "batch"); err != nil {
		t.Fatalf("CompileDir: %v", err)
	}

	alertPath := filepath.Join(dir, "ALERT.md")
	writeFile(t, dir, "ALERT.md", "# Alert\n\nPrompt line.\n")

	// Simulate rescan: recompile only the changed file.
	result, err := Compile(ctx, st,
		WithPaths([]string{alertPath}),
		WithMessage("rescan"),
		WithCompileRoot(dir),
	)
	if err != nil {
		t.Fatalf("Compile rescan: %v", err)
	}

	if result.Modified != 1 {
		t.Errorf("Modified = %d, want 1 (paragraph edit in a single file)", result.Modified)
	}
	if result.Added != 0 || result.Removed != 0 {
		t.Errorf("Added = %d, Removed = %d, want both 0 (root ID must be stable whether compiled alone or with siblings)", result.Added, result.Removed)
	}
}

func TestCompileFile_AnchorsCompileRoot(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	p := writeFile(t, sub, "doc.md", "# Hello\n\nContent.\n")

	if _, err := CompileFile(ctx, st, p, "v1"); err != nil {
		t.Fatalf("CompileFile: %v", err)
	}

	got, err := st.GetLatestCompileRoot(ctx)
	if err != nil {
		t.Fatalf("GetLatestCompileRoot: %v", err)
	}
	if got != sub {
		t.Errorf("compile_root = %q, want %q (file's parent dir)", got, sub)
	}
}

func TestCompile_PersistsCompileRoot(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	p := writeFile(t, dir, "doc.md", "# Hello\n\nContent.\n")

	if _, err := Compile(ctx, st,
		WithPaths([]string{p}),
		WithMessage("v1"),
		WithCompileRoot(dir),
	); err != nil {
		t.Fatalf("Compile: %v", err)
	}

	got, err := st.GetLatestCompileRoot(ctx)
	if err != nil {
		t.Fatalf("GetLatestCompileRoot: %v", err)
	}
	if got != dir {
		t.Errorf("compile_root = %q, want %q", got, dir)
	}
}

func TestCompile_HeadingEditDoesNotCascade(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	body := "# Hello\n\nBody one.\n\nBody two.\n"
	p := writeFile(t, dir, "doc.md", body)

	if _, err := Compile(ctx, st, WithPaths([]string{p}), WithMessage("v1")); err != nil {
		t.Fatalf("Compile v1: %v", err)
	}

	writeFile(t, dir, "doc.md", "# Goodbye\n\nBody one.\n\nBody two.\n")

	result, err := Compile(ctx, st, WithPaths([]string{p}), WithMessage("v2"))
	if err != nil {
		t.Fatalf("Compile v2: %v", err)
	}

	if result.Modified != 1 {
		t.Errorf("Modified = %d, want 1 (only the heading changed)", result.Modified)
	}
	if result.Added != 0 || result.Removed != 0 {
		t.Errorf("Added = %d, Removed = %d, want both 0 (no subtree cascade)", result.Added, result.Removed)
	}
}

func TestCompileDir_SkipsHiddenDirs(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	writeFile(t, dir, "notes.md", "# Notes\n\nVisible.\n")

	hidden := filepath.Join(dir, ".obsidian")
	if err := os.MkdirAll(hidden, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, hidden, "prefs.json", `{"hidden": true}`)

	nested := filepath.Join(dir, "node_modules", "pkg")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, nested, "pkg.json", `{"name": "pkg"}`)

	result, err := CompileDir(ctx, st, dir, "skip")
	if err != nil {
		t.Fatalf("CompileDir: %v", err)
	}

	nodes, err := st.GetAllNodes(ctx)
	if err != nil {
		t.Fatalf("GetAllNodes: %v", err)
	}
	for _, n := range nodes {
		if filepath.Base(filepath.Dir(n.SourceFile)) == ".obsidian" {
			t.Errorf("indexed hidden dir entry: %s", n.SourceFile)
		}
		if strings.Contains(n.SourceFile, "node_modules") {
			t.Errorf("indexed node_modules entry: %s", n.SourceFile)
		}
	}
	if result.Added == 0 {
		t.Error("expected visible file to contribute nodes")
	}
}

func TestCompileDir_RelativeDirInput(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()
	root := t.TempDir()

	const base = "proj"
	subDir := filepath.Join(root, base)
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, subDir, "doc.md", "# Hello\n\nContent.\n")

	t.Chdir(root)

	if _, err := CompileDir(ctx, st, base, "rel"); err != nil {
		t.Fatalf("CompileDir: %v", err)
	}

	nodes, err := st.GetAllNodes(ctx)
	if err != nil {
		t.Fatalf("GetAllNodes: %v", err)
	}
	if len(nodes) == 0 {
		t.Fatal("no nodes emitted")
	}

	prefix := base + string(filepath.Separator)
	for _, n := range nodes {
		if strings.HasPrefix(n.SourceFile, prefix) {
			t.Errorf("SourceFile = %q, want rel-to-compile-root (no leading %q)", n.SourceFile, prefix)
		}
	}
}

func TestCompileDir_RespectsIgnore(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	writeFile(t, dir, "kept.md", "# Kept\n\nVisible.\n")
	writeFile(t, dir, "session.jsonl", `{"event":"chat"}`)

	subdir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, subdir, "log.json", `{"id":1}`)

	writeIgnoreFile(t, dir, "*.jsonl\nsessions/\n")

	result, err := CompileDir(ctx, st, dir, "ignore-test")
	if err != nil {
		t.Fatalf("CompileDir: %v", err)
	}

	nodes, err := st.GetAllNodes(ctx)
	if err != nil {
		t.Fatalf("GetAllNodes: %v", err)
	}
	for _, n := range nodes {
		if strings.HasSuffix(n.SourceFile, ".jsonl") {
			t.Errorf("indexed excluded jsonl file: %s", n.SourceFile)
		}
		if strings.Contains(n.SourceFile, "sessions"+string(filepath.Separator)) {
			t.Errorf("indexed file in excluded dir: %s", n.SourceFile)
		}
		if strings.Contains(filepath.ToSlash(n.SourceFile), config.DirName+"/") {
			t.Errorf("indexed file inside %s/: %s", config.DirName, n.SourceFile)
		}
	}
	if result.Added == 0 {
		t.Error("expected kept.md to contribute nodes")
	}
}

func TestCompileDir_ExcludesSessionLogsDir(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	writeFile(t, dir, "kept.md", "# Kept\n\nVisible.\n")

	logsDir := filepath.Join(dir, config.DirName, "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, logsDir, "sess-abc.log", "2026-01-01T00:00:00Z DEBUG mcp call tool=MemoryWrite\n")

	result, err := CompileDir(ctx, st, dir, "logs-excl")
	if err != nil {
		t.Fatalf("CompileDir: %v", err)
	}

	nodes, err := st.GetAllNodes(ctx)
	if err != nil {
		t.Fatalf("GetAllNodes: %v", err)
	}

	for _, n := range nodes {
		if strings.Contains(filepath.ToSlash(n.SourceFile), config.DirName+"/logs/") {
			t.Errorf("indexed session log under %s/logs/: %s", config.DirName, n.SourceFile)
		}
	}
	if result.Added == 0 {
		t.Error("expected kept.md to contribute nodes")
	}
}

func TestCompileDir_MalformedIgnore(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	writeFile(t, dir, "doc.md", "# Hi\n")
	writeIgnoreFile(t, dir, "a//b\n")

	_, err := CompileDir(ctx, st, dir, "bad-ignore")
	if err == nil {
		t.Fatal("expected error for malformed ignore file")
	}
	if !strings.Contains(err.Error(), ignore.Path) {
		t.Errorf("error should mention %s, got: %v", ignore.Path, err)
	}
}

func TestCompile_DeterministicWithParallelParse(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	const fileCount = 30
	paths := make([]string, fileCount)
	for i := range fileCount {
		name := fmt.Sprintf("doc_%02d.md", i)
		content := fmt.Sprintf("# Heading %d\n\nBody paragraph %d.\n\nMore body %d.\n", i, i, i)
		paths[i] = writeFile(t, dir, name, content)
	}

	hashes := make([]string, 5)
	for i := range hashes {
		st := testutil.OpenTestDB(t)
		if _, err := Compile(ctx, st, WithPaths(paths), WithCompileRoot(dir), WithMessage("parallel")); err != nil {
			t.Fatalf("Compile: %v", err)
		}

		hash, err := st.GetHeadCursorHash(ctx)
		if err != nil {
			t.Fatalf("GetHeadCursorHash: %v", err)
		}
		hashes[i] = hash
	}
	for i := 1; i < len(hashes); i++ {
		if hashes[i] != hashes[0] {
			t.Errorf("run %d cursor_hash = %q, want %q (parallel parse must produce deterministic output)", i, hashes[i], hashes[0])
		}
	}
}

func TestCompile_CtxCancelStopsParseFanout(t *testing.T) {
	st := testutil.OpenTestDB(t)
	dir := t.TempDir()

	paths := make([]string, 50)
	for i := range paths {
		name := fmt.Sprintf("doc_%02d.md", i)
		paths[i] = writeFile(t, dir, name, "# Title\n\nBody.\n")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Compile(ctx, st, WithPaths(paths), WithCompileRoot(dir), WithMessage("cancelled"))
	if err == nil {
		t.Fatal("Compile: want error from cancelled ctx, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Compile error = %v, want errors.Is(err, context.Canceled)", err)
	}
}

func countSourceFile(nodes []*store.Node, suffix string) int {
	n := 0
	for _, x := range nodes {
		if strings.HasSuffix(x.SourceFile, suffix) {
			n++
		}
	}
	return n
}

func TestCompileDir_PrunesIgnored(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(dir, "drafts"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "kept.md", "# Kept\n\nVisible body.\n")
	writeFile(t, filepath.Join(dir, "drafts"), "notes.md", "# Draft\n\nDraft body.\n")

	if _, err := CompileDir(ctx, st, dir, "v1"); err != nil {
		t.Fatalf("CompileDir v1: %v", err)
	}

	before, err := st.GetAllNodes(ctx)
	if err != nil {
		t.Fatalf("GetAllNodes before: %v", err)
	}
	if countSourceFile(before, "notes.md") == 0 {
		t.Fatal("expected draft nodes after initial compile")
	}

	writeIgnoreFile(t, dir, "drafts/\n")
	result, err := CompileDir(ctx, st, dir, "v2")
	if err != nil {
		t.Fatalf("CompileDir v2: %v", err)
	}
	if result.Removed == 0 {
		t.Errorf("Removed = %d, want > 0 (drafts/ now ignored)", result.Removed)
	}

	after, err := st.GetAllNodes(ctx)
	if err != nil {
		t.Fatalf("GetAllNodes after: %v", err)
	}
	if got := countSourceFile(after, "notes.md"); got != 0 {
		t.Errorf("draft nodes still present after ignore: %d", got)
	}
	if countSourceFile(after, "kept.md") == 0 {
		t.Error("kept.md was pruned alongside the ignored file")
	}
}

func TestCompileDir_PrunesDeleted(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	writeFile(t, dir, "kept.md", "# Kept\n\nBody.\n")
	oldPath := writeFile(t, dir, "old.md", "# Old\n\nBody.\n")

	if _, err := CompileDir(ctx, st, dir, "v1"); err != nil {
		t.Fatalf("CompileDir v1: %v", err)
	}
	if err := os.Remove(oldPath); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	result, err := CompileDir(ctx, st, dir, "v2")
	if err != nil {
		t.Fatalf("CompileDir v2: %v", err)
	}
	if result.Removed == 0 {
		t.Errorf("Removed = %d, want > 0 (old.md deleted from disk)", result.Removed)
	}

	after, err := st.GetAllNodes(ctx)
	if err != nil {
		t.Fatalf("GetAllNodes: %v", err)
	}
	if got := countSourceFile(after, "old.md"); got != 0 {
		t.Errorf("orphaned nodes from old.md remain: %d", got)
	}
}

func TestCompileDir_PrunesRenamed(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	aPath := writeFile(t, dir, "a.md", "# A\n\nBody.\n")

	if _, err := CompileDir(ctx, st, dir, "v1"); err != nil {
		t.Fatalf("CompileDir v1: %v", err)
	}
	if err := os.Rename(aPath, filepath.Join(dir, "b.md")); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	result, err := CompileDir(ctx, st, dir, "v2")
	if err != nil {
		t.Fatalf("CompileDir v2: %v", err)
	}
	if result.Removed == 0 {
		t.Errorf("Removed = %d, want > 0 (a.md renamed to b.md)", result.Removed)
	}
	if result.Added == 0 {
		t.Errorf("Added = %d, want > 0 (b.md is a new file)", result.Added)
	}

	after, err := st.GetAllNodes(ctx)
	if err != nil {
		t.Fatalf("GetAllNodes: %v", err)
	}
	if got := countSourceFile(after, "a.md"); got != 0 {
		t.Errorf("orphaned nodes from a.md remain: %d", got)
	}
	if countSourceFile(after, "b.md") == 0 {
		t.Error("b.md not added after rename")
	}
}

func TestCompile_SkipsUnsupported(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	mdPath := writeFile(t, dir, "doc.md", "# Hello\n\nContent.\n")
	txtPath := writeFile(t, dir, "notes.txt", "plain text not supported")

	result, err := Compile(ctx, st, WithPaths([]string{mdPath, txtPath}), WithMessage("mixed"))
	if err != nil {
		t.Fatalf("Compile should not fail on unsupported files: %v", err)
	}
	if result.Added == 0 {
		t.Error("expected nodes from the .md file")
	}
}

func nodeTemps(t *testing.T, ctx context.Context, st *store.Store, sourceFile string) []float64 {
	t.Helper()

	nodes, err := st.GetNodesByFile(ctx, sourceFile)
	if err != nil {
		t.Fatalf("GetNodesByFile %s: %v", sourceFile, err)
	}
	if len(nodes) == 0 {
		t.Fatalf("no nodes for %s", sourceFile)
	}

	out := make([]float64, len(nodes))
	for i, n := range nodes {
		out[i] = n.Temperature
	}
	return out
}

func setAllTemps(t *testing.T, ctx context.Context, st *store.Store, sourceFile string, temp float64) {
	t.Helper()

	nodes, err := st.GetNodesByFile(ctx, sourceFile)
	if err != nil {
		t.Fatalf("GetNodesByFile %s: %v", sourceFile, err)
	}
	for _, n := range nodes {
		if err := st.UpdateTemperature(ctx, n.ID, temp); err != nil {
			t.Fatalf("UpdateTemperature %s: %v", n.ID, err)
		}
	}
}

func assertAllTempsEqual(t *testing.T, got []float64, want float64) {
	t.Helper()

	for i, g := range got {
		if g != want {
			t.Errorf("node[%d].Temperature = %g, want %g", i, g, want)
		}
	}
}

func TestCompileDir_ReseedTemperatures_OverridesUnchanged(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	writeTempfile(t, dir, `{"doc.md": 0.9}`)
	writeFile(t, dir, "doc.md", "# Hi\n\nBody one.\n\nBody two.\n")

	if _, err := CompileDir(ctx, st, dir, "v1"); err != nil {
		t.Fatalf("CompileDir v1: %v", err)
	}
	assertAllTempsEqual(t, nodeTemps(t, ctx, st, "doc.md"), 0.9)

	setAllTemps(t, ctx, st, "doc.md", 0.3)
	writeTempfile(t, dir, `{"doc.md": 0.1}`)

	if _, err := CompileDir(ctx, st, dir, "v2", WithReseedTemperatures()); err != nil {
		t.Fatalf("CompileDir v2: %v", err)
	}
	assertAllTempsEqual(t, nodeTemps(t, ctx, st, "doc.md"), 0.1)
}

func TestCompileDir_ReseedTemperatures_DefaultPreservesExisting(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	writeTempfile(t, dir, `{"doc.md": 0.9}`)
	writeFile(t, dir, "doc.md", "# Hi\n\nBody one.\n\nBody two.\n")

	if _, err := CompileDir(ctx, st, dir, "v1"); err != nil {
		t.Fatalf("CompileDir v1: %v", err)
	}

	setAllTemps(t, ctx, st, "doc.md", 0.3)
	writeTempfile(t, dir, `{"doc.md": 0.1}`)

	if _, err := CompileDir(ctx, st, dir, "v2"); err != nil {
		t.Fatalf("CompileDir v2: %v", err)
	}
	assertAllTempsEqual(t, nodeTemps(t, ctx, st, "doc.md"), 0.3)
}

func TestCompileDir_ReseedTemperatures_LeavesUnseededFilesAlone(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	writeTempfile(t, dir, `{"a.md": 0.9}`)
	writeFile(t, dir, "a.md", "# A\n\nAlpha.\n")
	writeFile(t, dir, "b.md", "# B\n\nBeta.\n")

	if _, err := CompileDir(ctx, st, dir, "v1"); err != nil {
		t.Fatalf("CompileDir v1: %v", err)
	}

	setAllTemps(t, ctx, st, "a.md", 0.3)
	setAllTemps(t, ctx, st, "b.md", 0.4)

	writeTempfile(t, dir, `{"a.md": 0.1}`)

	if _, err := CompileDir(ctx, st, dir, "v2", WithReseedTemperatures()); err != nil {
		t.Fatalf("CompileDir v2: %v", err)
	}

	assertAllTempsEqual(t, nodeTemps(t, ctx, st, "a.md"), 0.1)
	assertAllTempsEqual(t, nodeTemps(t, ctx, st, "b.md"), 0.4)
}

func TestCompileDir_ReseedTemperatures_NoTempFile(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	writeFile(t, dir, "doc.md", "# Hi\n\nBody.\n")

	if _, err := CompileDir(ctx, st, dir, "v1"); err != nil {
		t.Fatalf("CompileDir v1: %v", err)
	}

	setAllTemps(t, ctx, st, "doc.md", 0.3)

	if _, err := CompileDir(ctx, st, dir, "v2", WithReseedTemperatures()); err != nil {
		t.Fatalf("CompileDir v2: %v", err)
	}
	assertAllTempsEqual(t, nodeTemps(t, ctx, st, "doc.md"), 0.3)
}

func TestCompileDir_ReseedTemperatures_NoNewSnapshot(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	writeTempfile(t, dir, `{"doc.md": 0.9}`)
	writeFile(t, dir, "doc.md", "# Hi\n\nBody.\n")

	if _, err := CompileDir(ctx, st, dir, "v1"); err != nil {
		t.Fatalf("CompileDir v1: %v", err)
	}

	setAllTemps(t, ctx, st, "doc.md", 0.3)

	before, err := st.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats before: %v", err)
	}

	if _, err := CompileDir(ctx, st, dir, "v2", WithReseedTemperatures()); err != nil {
		t.Fatalf("CompileDir v2: %v", err)
	}

	after, err := st.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats after: %v", err)
	}

	if got := after.SnapshotCount - before.SnapshotCount; got != 0 {
		t.Errorf("SnapshotCount delta = %d, want 0 (reseed-only run must not emit)", got)
	}
	assertAllTempsEqual(t, nodeTemps(t, ctx, st, "doc.md"), 0.9)
}

func TestCompile_WikilinkResolvesCrossFile(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	a := writeFile(t, dir, "a.md", "# Source\n\nSee [[Architecture]] for details.\n")
	b := writeFile(t, dir, "b.md", "# Architecture\n\nThe details live here.\n")

	if _, err := Compile(ctx, st, WithPaths([]string{a, b}), WithCompileRoot(dir)); err != nil {
		t.Fatalf("Compile: %v", err)
	}

	// Compiler strips compileRoot prefix; nodes carry basename as source_file.
	aNodes, err := st.GetNodesByFile(ctx, "a.md")
	if err != nil {
		t.Fatalf("GetNodesByFile(a): %v", err)
	}

	var sourceID string
	for _, n := range aNodes {
		if n.NodeType == "text" && strings.Contains(n.Content, "[[Architecture]]") {
			sourceID = n.ID
			break
		}
	}
	if sourceID == "" {
		t.Fatalf("no source paragraph found in a.md: %+v", aNodes)
	}

	related, err := st.GetRelatedNodes(ctx, sourceID, store.WithDirection(store.DirectionOut), store.WithMaxDepth(1), store.WithLimit(10))
	if err != nil {
		t.Fatalf("GetRelatedNodes: %v", err)
	}

	if len(related) != 1 {
		t.Fatalf("len(related) = %d, want 1; sourceID=%s aNodes=%+v", len(related), sourceID, aNodes)
	}

	got := related[0]
	if got.Hop != 1 {
		t.Errorf("hop = %d, want 1 (direct edge)", got.Hop)
	}
	if got.Weight != 1.0 {
		t.Errorf("weight = %f, want 1.0 (default, no w= in link)", got.Weight)
	}
	if got.Node.Label != "Architecture" {
		t.Errorf("target label = %q, want Architecture", got.Node.Label)
	}
	if got.Node.NodeType != "heading" {
		t.Errorf("target node_type = %q, want heading", got.Node.NodeType)
	}
	if got.Node.SourceFile != "b.md" {
		t.Errorf("target source_file = %q, want b.md (after compileRoot strip)", got.Node.SourceFile)
	}
	if got.Node.Content != "Architecture" {
		t.Errorf("target content = %q, want Architecture", got.Node.Content)
	}

	// Pending should be empty since the cross-file reference resolved.
	pending, _ := st.GetAllPendingRelations(ctx)
	if len(pending) != 0 {
		t.Errorf("pending = %+v, want empty", pending)
	}
}

func TestCompile_WikilinkPendingResolvesOnLaterCompile(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	a := writeFile(t, dir, "a.md", "# Source\n\nLink to [[FutureHeading]]\n")

	if _, err := Compile(ctx, st, WithPaths([]string{a}), WithCompileRoot(dir)); err != nil {
		t.Fatalf("Compile(a): %v", err)
	}
	pending, _ := st.GetAllPendingRelations(ctx)
	if len(pending) != 1 {
		t.Fatalf("after first compile: pending = %d, want 1", len(pending))
	}

	// Now add the target file and recompile both.
	b := writeFile(t, dir, "b.md", "# FutureHeading\n\nNow exists.\n")

	if _, err := Compile(ctx, st, WithPaths([]string{a, b}), WithCompileRoot(dir)); err != nil {
		t.Fatalf("Compile(a,b): %v", err)
	}

	pending, _ = st.GetAllPendingRelations(ctx)
	if len(pending) != 0 {
		t.Errorf("pending should be empty after b.md compiled, got %+v", pending)
	}

	// The pending row should have *moved* to the relations table, not just disappeared.
	aNodes, err := st.GetNodesByFile(ctx, "a.md")
	if err != nil {
		t.Fatalf("GetNodesByFile(a): %v", err)
	}
	var sourceID string
	for _, n := range aNodes {
		if n.NodeType == "text" && strings.Contains(n.Content, "[[FutureHeading]]") {
			sourceID = n.ID
			break
		}
	}
	if sourceID == "" {
		t.Fatalf("no source paragraph found in a.md after second compile: %+v", aNodes)
	}

	related, err := st.GetRelatedNodes(ctx, sourceID, store.WithDirection(store.DirectionOut), store.WithMaxDepth(1), store.WithLimit(10))
	if err != nil {
		t.Fatalf("GetRelatedNodes: %v", err)
	}

	if len(related) != 1 {
		t.Fatalf("len(related) = %d, want 1 (pending should have moved to relations)", len(related))
	}
	if related[0].Node.Label != "FutureHeading" || related[0].Node.NodeType != "heading" {
		t.Errorf("resolved target = {%q, %s}, want {FutureHeading, heading}",
			related[0].Node.Label, related[0].Node.NodeType)
	}
	if related[0].Node.SourceFile != "b.md" {
		t.Errorf("source_file = %q, want b.md", related[0].Node.SourceFile)
	}
}

func TestCompile_SkipsOversizeFile(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	small := writeFile(t, dir, "small.md", "# Small\n\ntiny.\n")
	big := writeFile(t, dir, "big.md", "# Big\n\n"+strings.Repeat("padding ", 2000))

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn}))

	result, err := Compile(ctx, st,
		WithPaths([]string{small, big}),
		WithMessage("initial"),
		WithMaxFileSize(64),
		WithLogger(logger),
	)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if result.Added == 0 {
		t.Fatal("expected the small file to compile")
	}

	out := logs.String()
	if !strings.Contains(out, "skipping oversize file") || !strings.Contains(out, big) {
		t.Errorf("expected oversize warn naming %s, got: %s", big, out)
	}

	nodes, err := st.GetNodesByFiles(ctx, []string{big})
	if err != nil {
		t.Fatalf("GetNodesByFiles: %v", err)
	}
	if len(nodes) != 0 {
		t.Errorf("oversize file produced %d nodes, want 0", len(nodes))
	}
}

func TestCompile_WallClockTimeout(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	p := writeFile(t, dir, "doc.md", "# Doc\n\nbody\n")

	_, err := Compile(ctx, st,
		WithPaths([]string{p}),
		WithMessage("initial"),
		WithWallClockTimeout(time.Nanosecond),
	)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
	if !strings.Contains(err.Error(), "wall-clock timeout") {
		t.Errorf("expected wall-clock timeout message, got %v", err)
	}

	snaps, _ := st.ListSnapshots(ctx, 10)
	if len(snaps) != 0 {
		t.Errorf("snapshots = %d, want 0 (timeout must not commit partial state)", len(snaps))
	}
}

func TestCompile_MaxParallelismOne(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	paths := make([]string, 5)
	for i := range paths {
		name := fmt.Sprintf("doc%d.md", i)
		paths[i] = writeFile(t, dir, name, fmt.Sprintf("# Doc %d\n\nbody %d\n", i, i))
	}

	result, err := Compile(ctx, st,
		WithPaths(paths),
		WithMessage("initial"),
		WithMaxParallelism(1),
	)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if result.Added == 0 {
		t.Error("expected nodes added with serial compile")
	}
}

func TestConfigOptions_MapsCompileBlock(t *testing.T) {
	mb := config.ByteSize(2 << 20)
	par := 3
	to := config.Duration(45 * time.Second)
	cc := config.CompileConfig{MaxFileSize: &mb, MaxParallelism: &par, WallClockTimeout: &to}

	var o options
	for _, opt := range ConfigOptions(cc) {
		opt(&o)
	}

	if o.maxFileSize != 2<<20 {
		t.Errorf("maxFileSize = %d, want %d", o.maxFileSize, 2<<20)
	}
	if o.maxParallel != 3 {
		t.Errorf("maxParallel = %d, want 3", o.maxParallel)
	}
	if o.timeout != 45*time.Second {
		t.Errorf("timeout = %s, want 45s", o.timeout)
	}

	if got := ConfigOptions(config.CompileConfig{}); len(got) != 0 {
		t.Errorf("empty config produced %d options, want 0", len(got))
	}
}
