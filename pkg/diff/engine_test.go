package diff

import (
	"testing"

	"github.com/radimsem/remindb/pkg/parser"
)

func TestDiff_FirstCompilation(t *testing.T) {
	roots := []*parser.ContextNode{
		{ID: "aaa", ContentHash: "h1", Content: "hello"},
		{ID: "bbb", ContentHash: "h2", Content: "world"},
	}

	deltas := Diff(roots, nil)
	if len(deltas) != 2 {
		t.Fatalf("len = %d, want 2", len(deltas))
	}

	for _, d := range deltas {
		if d.Op != OpAdd {
			t.Errorf("delta %s: Op = %q, want %q", d.NodeID, d.Op, OpAdd)
		}
	}
}

func TestDiff_NoChanges(t *testing.T) {
	roots := []*parser.ContextNode{
		{ID: "aaa", ContentHash: "h1", Content: "hello"},
	}
	prev := map[string]NodeState{
		"aaa": {Hash: "h1", Content: "hello"},
	}

	deltas := Diff(roots, prev)
	if len(deltas) != 0 {
		t.Errorf("len = %d, want 0", len(deltas))
	}
}

func TestDiff_Modified(t *testing.T) {
	roots := []*parser.ContextNode{
		{ID: "aaa", ContentHash: "h2", Content: "hello updated"},
	}
	prev := map[string]NodeState{
		"aaa": {Hash: "h1", Content: "hello"},
	}

	deltas := Diff(roots, prev)
	if len(deltas) != 1 {
		t.Fatalf("len = %d, want 1", len(deltas))
	}

	d := deltas[0]
	if d.Op != OpMod {
		t.Errorf("Op = %q, want %q", d.Op, OpMod)
	}
	if d.OldHash != "h1" || d.NewHash != "h2" {
		t.Errorf("OldHash=%q NewHash=%q", d.OldHash, d.NewHash)
	}
	if d.OldContent != "hello" || d.NewContent != "hello updated" {
		t.Errorf("OldContent=%q NewContent=%q", d.OldContent, d.NewContent)
	}
}

func TestDiff_Removed(t *testing.T) {
	roots := []*parser.ContextNode{
		{ID: "aaa", ContentHash: "h1", Content: "hello"},
	}
	prev := map[string]NodeState{
		"aaa": {Hash: "h1", Content: "hello"},
		"bbb": {Hash: "h2", Content: "gone"},
	}

	deltas := Diff(roots, prev)
	if len(deltas) != 1 {
		t.Fatalf("len = %d, want 1", len(deltas))
	}

	d := deltas[0]
	if d.Op != OpRem {
		t.Errorf("Op = %q, want %q", d.Op, OpRem)
	}
	if d.NodeID != "bbb" {
		t.Errorf("NodeID = %q, want %q", d.NodeID, "bbb")
	}
}

func TestDiff_Mixed(t *testing.T) {
	roots := []*parser.ContextNode{
		{ID: "aaa", ContentHash: "h1", Content: "unchanged"},
		{ID: "bbb", ContentHash: "h3", Content: "modified"},
		{ID: "ddd", ContentHash: "h4", Content: "new node"},
	}
	prev := map[string]NodeState{
		"aaa": {Hash: "h1", Content: "unchanged"},
		"bbb": {Hash: "h2", Content: "old content"},
		"ccc": {Hash: "hx", Content: "removed"},
	}

	deltas := Diff(roots, prev)

	ops := make(map[Op]int)
	for _, d := range deltas {
		ops[d.Op]++
	}

	if ops[OpAdd] != 1 {
		t.Errorf("adds = %d, want 1", ops[OpAdd])
	}
	if ops[OpMod] != 1 {
		t.Errorf("mods = %d, want 1", ops[OpMod])
	}
	if ops[OpRem] != 1 {
		t.Errorf("rems = %d, want 1", ops[OpRem])
	}
}

func TestDiff_Children(t *testing.T) {
	roots := []*parser.ContextNode{
		{
			ID: "root", ContentHash: "hr", Content: "root",
			Children: []*parser.ContextNode{
				{ID: "child", ContentHash: "hc", Content: "child"},
			},
		},
	}

	deltas := Diff(roots, nil)
	if len(deltas) != 2 {
		t.Fatalf("len = %d, want 2 (root + child)", len(deltas))
	}
}
