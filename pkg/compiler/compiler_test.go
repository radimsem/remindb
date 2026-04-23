package compiler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/radimsem/remindb/internal/testutil"
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
