package treewalk

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/radimsem/remindb/pkg/store"
)

func TestRelativeTo_NoRoot(t *testing.T) {
	if got := RelativeTo("/abs/doc.md", ""); got != "/abs/doc.md" {
		t.Errorf("got %q, want unchanged", got)
	}
}

func TestRelativeTo_RelativeSource(t *testing.T) {
	if got := RelativeTo("doc.md", "/root"); got != "doc.md" {
		t.Errorf("got %q, want unchanged", got)
	}
}

func TestRelativeTo_InsideRoot(t *testing.T) {
	root := "/root"
	src := filepath.Join(root, "sub", "doc.md")

	if got := RelativeTo(src, root); got != filepath.Join("sub", "doc.md") {
		t.Errorf("got %q, want sub/doc.md", got)
	}
}

func TestRelativeTo_EscapesRoot(t *testing.T) {
	if got := RelativeTo("/other/doc.md", "/root"); got != "/other/doc.md" {
		t.Errorf("got %q, want unchanged when path escapes root", got)
	}
}

func TestClampDepth(t *testing.T) {
	cases := []struct {
		in, def, maximum, want int
	}{
		{0, 5, 128, 5},     // non-positive -> default
		{-3, 10, 128, 10},  // negative -> default
		{3, 5, 128, 3},     // in range -> unchanged
		{200, 5, 128, 128}, // over cap -> cap
		{200, 5, 0, 200},   // max<=0 -> unbounded
	}

	for _, c := range cases {
		if got := ClampDepth(c.in, c.def, c.maximum); got != c.want {
			t.Errorf("ClampDepth(%d,%d,%d) = %d, want %d", c.in, c.def, c.maximum, got, c.want)
		}
	}
}

// A -> {B -> {D}, C}
func sampleTree() (map[string][]*store.Node, *store.Node) {
	a := &store.Node{ID: "A"}
	b := &store.Node{ID: "B", ParentID: "A"}
	c := &store.Node{ID: "C", ParentID: "A"}
	d := &store.Node{ID: "D", ParentID: "B"}

	children := map[string][]*store.Node{
		"A": {b, c},
		"B": {d},
	}
	return children, a
}

func TestWalk_PreOrderAndParent(t *testing.T) {
	children, root := sampleTree()

	var order []string
	parents := map[string]string{}

	Walk(children, root, 0, func(n, parent *store.Node, _ int, descend func() []struct{}) struct{} {
		order = append(order, n.ID)
		if parent != nil {
			parents[n.ID] = parent.ID
		}

		descend()
		return struct{}{}
	})

	if got := strings.Join(order, ","); got != "A,B,D,C" {
		t.Errorf("pre-order = %q, want A,B,D,C", got)
	}

	if parents["A"] != "" {
		t.Errorf("root parent = %q, want none", parents["A"])
	}
	if parents["D"] != "B" || parents["B"] != "A" {
		t.Errorf("parents = %v, want D<-B<-A", parents)
	}
}

func TestWalk_DepthCutoff(t *testing.T) {
	children, root := sampleTree()

	var order []string
	Walk(children, root, 1, func(n, _ *store.Node, _ int, descend func() []struct{}) struct{} {
		order = append(order, n.ID)
		descend()
		return struct{}{}
	})

	if got := strings.Join(order, ","); got != "A,B,C" {
		t.Errorf("depth-1 walk = %q, want A,B,C (D pruned)", got)
	}
}

func TestWalk_PostOrderCountAndNilAtCutoff(t *testing.T) {
	children, root := sampleTree()

	cutoffNil := false
	count := Walk(children, root, 0, func(_, _ *store.Node, _ int, descend func() []int) int {
		sub := descend()
		total := 1

		for _, s := range sub {
			total += s
		}
		return total
	})

	if count != 4 {
		t.Errorf("node count = %d, want 4", count)
	}

	Walk(children, root, 1, func(_, _ *store.Node, depth int, descend func() []int) int {
		if descend() == nil && depth >= 1 {
			cutoffNil = true
		}
		return 0
	})
	if !cutoffNil {
		t.Error("descend() should return nil at the depth cutoff")
	}
}
