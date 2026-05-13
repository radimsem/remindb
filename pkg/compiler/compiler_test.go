package compiler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/radimsem/remindb/internal/ignore"
	"github.com/radimsem/remindb/internal/testutil"
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

	writeFile(t, dir, ignore.FileName, "*.jsonl\nsessions/\n")

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
		if filepath.Base(n.SourceFile) == ignore.FileName {
			t.Errorf("indexed the ignore file itself: %s", n.SourceFile)
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
	writeFile(t, dir, ignore.FileName, "a//b\n")

	_, err := CompileDir(ctx, st, dir, "bad-ignore")
	if err == nil {
		t.Fatal("expected error for malformed ignore file")
	}
	if !strings.Contains(err.Error(), ignore.FileName) {
		t.Errorf("error should mention %s, got: %v", ignore.FileName, err)
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

	writeFile(t, dir, ignore.FileName, "drafts/\n")
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

	writeFile(t, dir, ".temp.json", `{"doc.md": 0.9}`)
	writeFile(t, dir, "doc.md", "# Hi\n\nBody one.\n\nBody two.\n")

	if _, err := CompileDir(ctx, st, dir, "v1"); err != nil {
		t.Fatalf("CompileDir v1: %v", err)
	}
	assertAllTempsEqual(t, nodeTemps(t, ctx, st, "doc.md"), 0.9)

	setAllTemps(t, ctx, st, "doc.md", 0.3)
	writeFile(t, dir, ".temp.json", `{"doc.md": 0.1}`)

	if _, err := CompileDir(ctx, st, dir, "v2", WithReseedTemperatures()); err != nil {
		t.Fatalf("CompileDir v2: %v", err)
	}
	assertAllTempsEqual(t, nodeTemps(t, ctx, st, "doc.md"), 0.1)
}

func TestCompileDir_ReseedTemperatures_DefaultPreservesExisting(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	writeFile(t, dir, ".temp.json", `{"doc.md": 0.9}`)
	writeFile(t, dir, "doc.md", "# Hi\n\nBody one.\n\nBody two.\n")

	if _, err := CompileDir(ctx, st, dir, "v1"); err != nil {
		t.Fatalf("CompileDir v1: %v", err)
	}

	setAllTemps(t, ctx, st, "doc.md", 0.3)
	writeFile(t, dir, ".temp.json", `{"doc.md": 0.1}`)

	if _, err := CompileDir(ctx, st, dir, "v2"); err != nil {
		t.Fatalf("CompileDir v2: %v", err)
	}
	assertAllTempsEqual(t, nodeTemps(t, ctx, st, "doc.md"), 0.3)
}

func TestCompileDir_ReseedTemperatures_LeavesUnseededFilesAlone(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	writeFile(t, dir, ".temp.json", `{"a.md": 0.9}`)
	writeFile(t, dir, "a.md", "# A\n\nAlpha.\n")
	writeFile(t, dir, "b.md", "# B\n\nBeta.\n")

	if _, err := CompileDir(ctx, st, dir, "v1"); err != nil {
		t.Fatalf("CompileDir v1: %v", err)
	}

	setAllTemps(t, ctx, st, "a.md", 0.3)
	setAllTemps(t, ctx, st, "b.md", 0.4)

	writeFile(t, dir, ".temp.json", `{"a.md": 0.1}`)

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

	writeFile(t, dir, ".temp.json", `{"doc.md": 0.9}`)
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
