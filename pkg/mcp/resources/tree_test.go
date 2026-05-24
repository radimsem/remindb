package resources

import (
	"testing"

	"github.com/radimsem/remindb/pkg/store"
)

// chain R(0) → A(1) → B(2) → C(3), plus a sibling leaf R → D(1).
func depthFixture() (root *store.Node, children map[string][]*store.Node) {
	r := &store.Node{ID: "R", Depth: 0}
	a := &store.Node{ID: "A", ParentID: "R", Depth: 1}
	b := &store.Node{ID: "B", ParentID: "A", Depth: 2}
	c := &store.Node{ID: "C", ParentID: "B", Depth: 3}
	d := &store.Node{ID: "D", ParentID: "R", Depth: 1}

	roots, ch := store.BuildTree([]*store.Node{r, a, b, c, d})
	return roots[0], ch
}

// Longest root→leaf edge count in the produced JSON tree.
func reach(n *treeNode) int {
	max := 0
	for _, c := range n.Children {
		if d := reach(c) + 1; d > max {
			max = d
		}
	}
	return max
}

func TestBuildTreeJSON_DepthBounding(t *testing.T) {
	root, children := depthFixture()

	cases := []struct {
		maxDepth  int
		wantReach int // generations below the requested root
	}{
		{0, 3}, // unbounded: R→A→B→C
		{1, 1}, // R + direct children only
		{2, 2}, // R + 2 generations
		{3, 3}, // R + 3 generations (== full here)
		{4, 3}, // bound beyond the tree: still full, no over-run
	}

	for _, tc := range cases {
		got := buildTreeJSON(children, root, "", tc.maxDepth)

		if r := reach(got); r != tc.wantReach {
			t.Errorf("maxDepth=%d: reach=%d, want %d", tc.maxDepth, r, tc.wantReach)
		}
	}
}

func TestBuildTreeJSON_ShapeInvariants(t *testing.T) {
	root, children := depthFixture()
	got := buildTreeJSON(children, root, "", 1)

	// Children is always a non-nil slice (JSON [] not null), even when bounded.
	if got.Children == nil {
		t.Fatal("root.Children is nil; want [] for stable JSON shape")
	}
	for _, c := range got.Children {
		if c.Children == nil {
			t.Errorf("bounded child %q has nil Children; want []", c.ID)
		}
	}

	// Emitted Depth is the node's absolute tree depth, not the relative bound.
	if got.Depth != 0 {
		t.Errorf("root Depth=%d, want 0 (absolute)", got.Depth)
	}
	for _, c := range got.Children {
		if c.Depth != 1 {
			t.Errorf("child %q Depth=%d, want 1 (absolute, unaffected by maxDepth)", c.ID, c.Depth)
		}
	}
}
