package compiler

import (
	"context"
	"os"
	"path/filepath"
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

	result, err := Compile(ctx, st, []string{p}, "initial")
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

func TestCompile_Recompile(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	p := writeFile(t, dir, "doc.md", "# Hello\n\nOriginal content.\n")

	_, err := Compile(ctx, st, []string{p}, "v1")
	if err != nil {
		t.Fatalf("Compile v1: %v", err)
	}

	// Modify and recompile.
	writeFile(t, dir, "doc.md", "# Hello\n\nUpdated content.\n")

	result, err := Compile(ctx, st, []string{p}, "v2")
	if err != nil {
		t.Fatalf("Compile v2: %v", err)
	}

	snaps, _ := st.ListSnapshots(ctx, 10)
	if len(snaps) != 2 {
		t.Errorf("snapshots = %d, want 2", len(snaps))
	}

	// Content-addressed IDs: changes are adds+removes, not mods.
	if result.Added+result.Removed == 0 {
		t.Error("expected changes on recompile")
	}
}

func TestCompile_SkipsUnsupported(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	mdPath := writeFile(t, dir, "doc.md", "# Hello\n\nContent.\n")
	txtPath := writeFile(t, dir, "notes.txt", "plain text not supported")

	result, err := Compile(ctx, st, []string{mdPath, txtPath}, "mixed")
	if err != nil {
		t.Fatalf("Compile should not fail on unsupported files: %v", err)
	}
	if result.Added == 0 {
		t.Error("expected nodes from the .md file")
	}
}
