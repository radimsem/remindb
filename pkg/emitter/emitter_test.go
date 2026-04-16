package emitter

import (
	"context"
	"testing"

	"github.com/radimsem/remindb/internal/testutil"
	"github.com/radimsem/remindb/pkg/diff"
	"github.com/radimsem/remindb/pkg/parser"
)

func TestEmit_FirstCompilation(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()

	roots := []*parser.ContextNode{
		{
			ID: "rootroot", ContentHash: "h1", Content: "hello",
			SourceFile: "doc.md", NodeType: parser.NodeHeading,
			Label: "hello", Format: "plain", Depth: 1, TokenCount: 4,
		},
	}
	deltas := []diff.Delta{
		{NodeID: "rootroot", Op: diff.OpAdd, NewHash: "h1", NewContent: "hello"},
	}

	if err := Emit(ctx, st, roots, deltas, "cursor0123456789", "initial"); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	n, err := st.GetNode(ctx, "rootroot")
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if n.Content != "hello" {
		t.Errorf("Content = %q", n.Content)
	}

	hash, _ := st.GetHeadCursorHash(ctx)
	if hash != "cursor0123456789" {
		t.Errorf("cursor = %q", hash)
	}

	snaps, _ := st.ListSnapshots(ctx, 10)
	if len(snaps) != 1 {
		t.Fatalf("snapshots = %d, want 1", len(snaps))
	}
	if snaps[0].Message != "initial" {
		t.Errorf("message = %q", snaps[0].Message)
	}
}

func TestEmit_ModifyAndRemove(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()

	roots := []*parser.ContextNode{
		{
			ID: "node0001", ContentHash: "h1", Content: "original",
			SourceFile: "a.md", NodeType: parser.NodeText,
			Label: "orig", Format: "plain", Depth: 1, TokenCount: 5,
		},
		{
			ID: "node0002", ContentHash: "h2", Content: "to remove",
			SourceFile: "b.md", NodeType: parser.NodeText,
			Label: "rem", Format: "plain", Depth: 1, TokenCount: 5,
		},
	}
	if err := Emit(ctx, st, roots, []diff.Delta{
		{NodeID: "node0001", Op: diff.OpAdd, NewHash: "h1", NewContent: "original"},
		{NodeID: "node0002", Op: diff.OpAdd, NewHash: "h2", NewContent: "to remove"},
	}, "cursor1111111111", "v1"); err != nil {
		t.Fatalf("Emit v1: %v", err)
	}

	roots2 := []*parser.ContextNode{
		{
			ID: "node0001", ContentHash: "h3", Content: "modified",
			SourceFile: "a.md", NodeType: parser.NodeText,
			Label: "mod", Format: "plain", Depth: 1, TokenCount: 5,
		},
	}
	if err := Emit(ctx, st, roots2, []diff.Delta{
		{NodeID: "node0001", Op: diff.OpMod, OldHash: "h1", NewHash: "h3", OldContent: "original", NewContent: "modified"},
		{NodeID: "node0002", Op: diff.OpRem, OldHash: "h2", OldContent: "to remove"},
	}, "cursor2222222222", "v2"); err != nil {
		t.Fatalf("Emit v2: %v", err)
	}

	n, _ := st.GetNode(ctx, "node0001")
	if n.Content != "modified" {
		t.Errorf("Content = %q, want %q", n.Content, "modified")
	}

	_, err := st.GetNode(ctx, "node0002")
	if err == nil {
		t.Errorf("node0002 should be deleted")
	}

	hash, _ := st.GetHeadCursorHash(ctx)
	if hash != "cursor2222222222" {
		t.Errorf("cursor = %q", hash)
	}

	snaps, _ := st.ListSnapshots(ctx, 10)
	if len(snaps) != 2 {
		t.Errorf("snapshots = %d, want 2", len(snaps))
	}
}
