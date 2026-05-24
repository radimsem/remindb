package mcp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/radimsem/remindb/internal/testutil"
)

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

	if err := MaybeInitialCompile(ctx, st, dir, nil); err != nil {
		t.Fatalf("seed compile: %v", err)
	}

	before, err := st.GetRootNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}

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

	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
