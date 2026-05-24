package resources

import (
	"database/sql"
	"testing"

	"github.com/radimsem/remindb/pkg/store"
)

func parentInt64(v int64) sql.NullInt64 { return sql.NullInt64{Int64: v, Valid: true} }

// chain: 1 (root) → 2 → 3, with 3 at HEAD; store yields newest-first.
func snapshotFixture() []*store.Snapshot {
	return []*store.Snapshot{
		{ID: 3, ParentID: parentInt64(2), Message: "third", CompileRoot: "/repo"},
		{ID: 2, ParentID: parentInt64(1), Message: "second", CompileRoot: "/repo"},
		{ID: 1, Message: "first", CompileRoot: "/repo"},
	}
}

func TestNewSnapshotsEnvelope_OrderAndLinks(t *testing.T) {
	env := newSnapshotsEnvelope(snapshotFixture(), 3)

	wantIDs := []int64{3, 2, 1}
	if len(env.Snapshots) != len(wantIDs) {
		t.Fatalf("len=%d, want %d", len(env.Snapshots), len(wantIDs))
	}
	for i, id := range wantIDs {
		if env.Snapshots[i].ID != id {
			t.Errorf("position %d: id=%d, want %d (store order must be preserved)", i, env.Snapshots[i].ID, id)
		}
	}

	root := env.Snapshots[2]
	if root.ParentID != nil {
		t.Errorf("root snapshot parent_id=%v, want nil", *root.ParentID)
	}

	child := env.Snapshots[1]
	if child.ParentID == nil || *child.ParentID != 1 {
		t.Errorf("child snapshot parent_id=%v, want 1", child.ParentID)
	}
}

func TestNewSnapshotsEnvelope_HeadMarker(t *testing.T) {
	env := newSnapshotsEnvelope(snapshotFixture(), 3)

	for _, s := range env.Snapshots {
		want := s.ID == 3
		if s.IsHead != want {
			t.Errorf("snapshot %d is_head=%v, want %v", s.ID, s.IsHead, want)
		}
	}

	// No HEAD recorded yet: nothing is marked.
	none := newSnapshotsEnvelope(snapshotFixture(), 0)
	for _, s := range none.Snapshots {
		if s.IsHead {
			t.Errorf("snapshot %d is_head=true with headID=0, want false", s.ID)
		}
	}
}

func TestNewSnapshotDiffsEnvelope_Mapping(t *testing.T) {
	diffs := []*store.DiffRecord{
		{NodeID: "n1", Op: "add", NewHash: "h1", NewContent: "added"},
		{NodeID: "n2", Op: "mod", OldHash: "h2a", NewHash: "h2b", OldContent: "before", NewContent: "after"},
	}

	env := newSnapshotDiffsEnvelope(7, diffs)

	if env.SnapshotID != 7 {
		t.Errorf("snapshot_id=%d, want 7", env.SnapshotID)
	}
	if len(env.Diffs) != 2 {
		t.Fatalf("len=%d, want 2", len(env.Diffs))
	}

	mod := env.Diffs[1]
	if mod.Op != "mod" || mod.NodeID != "n2" ||
		mod.OldHash != "h2a" || mod.NewHash != "h2b" ||
		mod.OldContent != "before" || mod.NewContent != "after" {
		t.Errorf("diff[1] mismapped: %+v", mod)
	}
}
