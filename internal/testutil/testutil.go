package testutil

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/radimsem/remindb/pkg/store"
)

const logLabelMaxLen = 60

func OpenTestDB(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	t.Cleanup(func() { _ = st.Close() })
	return st
}

func LogTree(t *testing.T, st *store.Store) {
	t.Helper()
	ctx := context.Background()

	roots, err := st.GetRootNodes(ctx)
	if err != nil {
		t.Logf("tree: error: %v", err)
		return
	}
	if len(roots) == 0 {
		t.Logf("tree: (empty)")
		return
	}

	var b strings.Builder
	for _, root := range roots {
		logTreeNode(&b, st, ctx, root, 0)
	}
	t.Logf("tree:\n%s", b.String())
}

func logTreeNode(b *strings.Builder, st *store.Store, ctx context.Context, n *store.Node, depth int) {
	indent := strings.Repeat("  ", depth)
	label := n.Label
	if len(label) > logLabelMaxLen {
		label = label[:logLabelMaxLen] + "..."
	}
	fmt.Fprintf(b, "%s[%s] %s (id=%s temp=%.2f tok=%d)\n",
		indent, n.NodeType, label, n.ID, n.Temperature, n.TokenCount)

	children, err := st.GetChildren(ctx, n.ID)
	if err != nil {
		return
	}
	for _, child := range children {
		logTreeNode(b, st, ctx, child, depth+1)
	}
}

func LogDiffs(t *testing.T, diffs []*store.DiffRecord) {
	t.Helper()
	if len(diffs) == 0 {
		t.Logf("delta: no changes")
		return
	}

	var b strings.Builder
	fmt.Fprintf(&b, "delta: %d changes\n", len(diffs))
	for _, d := range diffs {
		fmt.Fprintf(&b, "  [%s] node=%s snapshot=%d\n", d.Op, d.NodeID, d.SnapshotID)
	}
	t.Logf("%s", b.String())
}
