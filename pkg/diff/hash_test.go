package diff

import (
	"testing"

	"github.com/radimsem/remindb/pkg/parser"
)

func TestCursorHash_Deterministic(t *testing.T) {
	roots := []*parser.ContextNode{
		{ContentHash: "aaaa"},
		{ContentHash: "bbbb"},
	}

	a := CursorHash(roots)
	b := CursorHash(roots)
	if a != b {
		t.Errorf("non-deterministic: %q vs %q", a, b)
	}
	if len(a) != 16 {
		t.Errorf("len = %d, want 16", len(a))
	}
}

func TestCursorHash_DifferentContent(t *testing.T) {
	a := CursorHash([]*parser.ContextNode{{ContentHash: "aaaa"}})
	b := CursorHash([]*parser.ContextNode{{ContentHash: "bbbb"}})
	if a == b {
		t.Errorf("different nodes same hash: %q", a)
	}
}

func TestCursorHash_OrderIndependent(t *testing.T) {
	a := CursorHash([]*parser.ContextNode{
		{ContentHash: "aaaa"},
		{ContentHash: "bbbb"},
	})
	b := CursorHash([]*parser.ContextNode{
		{ContentHash: "bbbb"},
		{ContentHash: "aaaa"},
	})
	if a != b {
		t.Errorf("order-dependent: %q vs %q", a, b)
	}
}

// Two states with identical content-hash multisets but swapped node IDs
// must produce different cursor hashes — otherwise MemoryDelta clients
// silently miss the identity change.
func TestCursorHash_DetectsIdentitySwap(t *testing.T) {
	a := CursorHash([]*parser.ContextNode{
		{ID: "n1", ContentHash: "aaaa"},
		{ID: "n2", ContentHash: "bbbb"},
	})
	b := CursorHash([]*parser.ContextNode{
		{ID: "n1", ContentHash: "bbbb"},
		{ID: "n2", ContentHash: "aaaa"},
	})
	if a == b {
		t.Errorf("identity swap not detected: %q", a)
	}
}

// A repeated transition (same op/id/old/new deltas) must hash differently
// under a different parent snapshot id — this is what keeps an A→B→A→B toggle
// from colliding on snapshots.cursor_hash.
func TestCursorHashForChange_DistinctAcrossRepeatTransition(t *testing.T) {
	deltas := []Delta{{NodeID: "n1", Op: OpMod, OldHash: "aaaa", NewHash: "bbbb"}}

	a := CursorHashForChange(2, deltas)
	b := CursorHashForChange(4, deltas)
	if a == b {
		t.Errorf("repeated transition shares hash across parents: %q", a)
	}
	if len(a) != 16 {
		t.Errorf("len = %d, want 16", len(a))
	}
}

func TestCursorHashForChange_OrderStable(t *testing.T) {
	a := CursorHashForChange(7, []Delta{
		{NodeID: "n1", Op: OpMod, OldHash: "aaaa", NewHash: "bbbb"},
		{NodeID: "n2", Op: OpAdd, NewHash: "cccc"},
	})
	b := CursorHashForChange(7, []Delta{
		{NodeID: "n2", Op: OpAdd, NewHash: "cccc"},
		{NodeID: "n1", Op: OpMod, OldHash: "aaaa", NewHash: "bbbb"},
	})
	if a != b {
		t.Errorf("order-dependent: %q vs %q", a, b)
	}
}

func TestSnapshotFromNodes(t *testing.T) {
	roots := []*parser.ContextNode{
		{
			ID: "root", ContentHash: "hr", Content: "root",
			Children: []*parser.ContextNode{
				{ID: "child", ContentHash: "hc", Content: "child"},
			},
		},
	}

	snap := SnapshotFromNodes(roots)
	if len(snap) != 2 {
		t.Fatalf("len = %d, want 2", len(snap))
	}
	if snap["root"].Hash != "hr" {
		t.Errorf("root.Hash = %q", snap["root"].Hash)
	}
	if snap["child"].Content != "child" {
		t.Errorf("child.Content = %q", snap["child"].Content)
	}
}
