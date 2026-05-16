package bench

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/radimsem/remindb/internal/testutil"
	"github.com/radimsem/remindb/pkg/compiler"
)

func TestStageRewritesCompileRootSoBaselineSurvivesRelocation(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()

	dirA := t.TempDir()
	dirB := t.TempDir()
	for _, d := range []string{dirA, dirB} {
		writeFile(t, d, "a.md", "# A\n\nalpha content\n")
		writeFile(t, d, "b.md", "# B\n\nbeta content\n")
	}

	absA, err := filepath.Abs(dirA)
	if err != nil {
		t.Fatal(err)
	}
	absB, err := filepath.Abs(dirB)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Initial compile at path A (mirrors `remindb compile testdata/<x>`).
	r1, err := compiler.CompileDir(ctx, st, dirA, "initial")
	if err != nil {
		t.Fatalf("compile A: %v", err)
	}
	nodeCount := r1.Added
	if nodeCount == 0 {
		t.Fatal("initial compile added no nodes")
	}

	// 2. Mirror stageBench's relocation.
	if err := st.ExecRewriteSourcePaths(ctx, absA, absB); err != nil {
		t.Fatalf("rewrite source paths: %v", err)
	}
	if err := st.ExecRewriteCompileRoots(ctx, absA, absB); err != nil {
		t.Fatalf("rewrite compile roots: %v", err)
	}

	// The diff baseline must now resolve under the staged compile root.
	base, err := st.GetNodesByCompileRoot(ctx, absB)
	if err != nil {
		t.Fatalf("GetNodesByCompileRoot: %v", err)
	}
	if len(base) != nodeCount {
		t.Fatalf("baseline not repointed by compile-root rewrite: got %d nodes, want %d", len(base), nodeCount)
	}

	// 3. Change exactly one file and recompile the relocated tree.
	writeFile(t, dirB, "a.md", "# A\n\nalpha content\n\n## Added\n\nnew section\n")
	r2, err := compiler.CompileDir(ctx, st, dirB, "change")
	if err != nil {
		t.Fatalf("compile B: %v", err)
	}

	total := r2.Added + r2.Modified + r2.Removed
	if total == 0 {
		t.Fatal("expected the a.md change to produce diffs")
	}
	if total >= nodeCount {
		t.Fatalf("staged recompile re-added the whole tree (%d ops >= %d nodes): baseline lost", total, nodeCount)
	}
}
