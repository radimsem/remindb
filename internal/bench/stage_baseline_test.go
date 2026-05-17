package bench

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/radimsem/remindb/pkg/compiler"
	"github.com/radimsem/remindb/pkg/store"
)

func TestStageBenchFreshCompilesMatchingBaseline(t *testing.T) {
	ctx := context.Background()

	vault := filepath.Join(t.TempDir(), "vault")
	writeFile(t, vault, "a.md", "# A\n\nalpha content\n")
	writeFile(t, vault, "b.md", "# B\n\nbeta content\n")
	writeFile(t, vault, "sub/c.md", "# C\n\ngamma content\n")

	stage, err := stageBench(ctx, vault)
	if err != nil {
		t.Fatalf("stageBench: %v", err)
	}
	defer stage.cleanup()

	st, err := store.Open(stage.dbPath)
	if err != nil {
		t.Fatalf("open staged db: %v", err)
	}
	defer func() { _ = st.Close() }()

	base, err := st.GetNodesByCompileRoot(ctx, stage.srcDir)
	if err != nil {
		t.Fatalf("GetNodesByCompileRoot: %v", err)
	}
	if len(base) == 0 {
		t.Fatal("staged baseline empty: fresh compile did not populate the DB under stage.srcDir")
	}
	nodeCount := len(base)

	// A single-file change must recompile as a small delta.
	writeFile(t, stage.srcDir, "a.md", "# A\n\nalpha content\n\n## Added\n\nnew section\n")
	r, err := compiler.CompileDir(ctx, st, stage.srcDir, "change")
	if err != nil {
		t.Fatalf("staged recompile: %v", err)
	}

	total := r.Added + r.Modified + r.Removed
	if total == 0 {
		t.Fatal("expected the a.md change to produce diffs")
	}
	if total >= nodeCount {
		t.Fatalf("recompile re-added the whole tree (%d ops >= %d nodes): delta would report negative saved%%", total, nodeCount)
	}
}
