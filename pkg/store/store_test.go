package store

import (
	"context"
	"database/sql"
	"testing"
)

func openTestDB(t *testing.T) *Store {
	t.Helper()
	st, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func testNode(id, parent string) *Node {
	return &Node{
		ID: id, ParentID: parent,
		SourceFile: "test.md", NodeType: "heading", Depth: 1,
		Label: "label " + id, Content: "content " + id,
		Format: "plain", TokenCount: 10, ContentHash: "hash" + id,
	}
}

func TestUpsertAndGetNode(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	must(t, st.UpsertNode(ctx, testNode("aaaaaaaa", "")))

	got, err := st.GetNode(ctx, "aaaaaaaa")
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if got.Content != "content aaaaaaaa" {
		t.Errorf("Content = %q", got.Content)
	}
	if got.Temperature != 0.5 {
		t.Errorf("Temperature = %f, want 0.5", got.Temperature)
	}
}

func TestUpsertNode_Update(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	n := testNode("aaaaaaaa", "")
	must(t, st.UpsertNode(ctx, n))

	n.Content = "updated"
	must(t, st.UpsertNode(ctx, n))

	got, err := st.GetNode(ctx, "aaaaaaaa")
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if got.Content != "updated" {
		t.Errorf("Content = %q, want %q", got.Content, "updated")
	}
}

func TestGetNodesByFile(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	must(t, st.UpsertNode(ctx, testNode("aaaaaaaa", "")))
	must(t, st.UpsertNode(ctx, testNode("bbbbbbbb", "")))

	nodes, err := st.GetNodesByFile(ctx, "test.md")
	if err != nil {
		t.Fatalf("GetNodesByFile: %v", err)
	}
	if len(nodes) != 2 {
		t.Errorf("len = %d, want 2", len(nodes))
	}
}

func TestGetChildren(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	must(t, st.UpsertNode(ctx, testNode("rootroor", "")))
	must(t, st.UpsertNode(ctx, testNode("child001", "rootroor")))
	must(t, st.UpsertNode(ctx, testNode("child002", "rootroor")))

	children, err := st.GetChildren(ctx, "rootroor")
	if err != nil {
		t.Fatalf("GetChildren: %v", err)
	}
	if len(children) != 2 {
		t.Errorf("len = %d, want 2", len(children))
	}
}

func TestGetAncestors(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	must(t, st.UpsertNode(ctx, testNode("rootroor", "")))
	must(t, st.UpsertNode(ctx, testNode("mid00001", "rootroor")))
	must(t, st.UpsertNode(ctx, testNode("leaf0001", "mid00001")))

	anc, err := st.GetAncestors(ctx, "leaf0001")
	if err != nil {
		t.Fatalf("GetAncestors: %v", err)
	}
	if len(anc) != 3 {
		t.Errorf("len = %d, want 3 (leaf + mid + root)", len(anc))
	}
}

func TestDeleteNode(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	must(t, st.UpsertNode(ctx, testNode("aaaaaaaa", "")))
	must(t, st.DeleteNode(ctx, "aaaaaaaa"))

	_, err := st.GetNode(ctx, "aaaaaaaa")
	if err == nil {
		t.Errorf("expected error after delete")
	}
}

func TestSearch(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	n := testNode("aaaaaaaa", "")
	n.Content = "the quick brown fox jumps over the lazy dog"
	n.Label = "fox sentence"
	must(t, st.UpsertNode(ctx, n))

	results, err := st.Search(ctx, "fox", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("len = %d, want 1", len(results))
	}
}

func TestSnapshotAndCursor(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	hash, err := st.GetHeadCursorHash(ctx)
	if err != nil {
		t.Fatalf("GetHeadCursorHash: %v", err)
	}
	if hash != "" {
		t.Errorf("hash = %q, want empty", hash)
	}

	err = st.Tx(ctx, func(tx *sql.Tx) error {
		snapID, err := st.CreateSnapshotTx(ctx, tx, "abcdef0123456789", "first")
		if err != nil {
			return err
		}
		return st.AdvanceCursorTx(ctx, tx, snapID)
	})
	if err != nil {
		t.Fatalf("Tx: %v", err)
	}

	hash, err = st.GetHeadCursorHash(ctx)
	if err != nil {
		t.Fatalf("GetHeadCursorHash: %v", err)
	}
	if hash != "abcdef0123456789" {
		t.Errorf("hash = %q", hash)
	}

	snap, err := st.GetSnapshot(ctx, 1)
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	if snap.Message != "first" {
		t.Errorf("Message = %q", snap.Message)
	}
}

func TestTemperature(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	must(t, st.UpsertNode(ctx, testNode("aaaaaaaa", "")))
	must(t, st.UpdateTemperature(ctx, "aaaaaaaa", 0.9))

	got, err := st.GetNode(ctx, "aaaaaaaa")
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if got.Temperature != 0.9 {
		t.Errorf("Temperature = %f, want 0.9", got.Temperature)
	}

	must(t, st.IncrementAccess(ctx, "aaaaaaaa"))

	got, err = st.GetNode(ctx, "aaaaaaaa")
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if got.AccessCount != 1 {
		t.Errorf("AccessCount = %d, want 1", got.AccessCount)
	}

	cold, err := st.GetColdNodes(ctx, 0.5)
	if err != nil {
		t.Fatalf("GetColdNodes: %v", err)
	}
	if len(cold) != 0 {
		t.Errorf("cold = %d, want 0 (node temp is 0.9)", len(cold))
	}

	must(t, st.UpdateTemperature(ctx, "aaaaaaaa", 0.1))

	cold, err = st.GetColdNodes(ctx, 0.5)
	if err != nil {
		t.Fatalf("GetColdNodes: %v", err)
	}
	if len(cold) != 1 {
		t.Errorf("cold = %d, want 1", len(cold))
	}
}

func TestBoostTemperature(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	must(t, st.UpsertNode(ctx, testNode("aaaaaaaa", "")))

	must(t, st.BoostTemperature(ctx, "aaaaaaaa", 0.15))

	got, err := st.GetNode(ctx, "aaaaaaaa")
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if got.Temperature != 0.65 {
		t.Errorf("Temperature = %f, want 0.65 (0.5 + 0.15)", got.Temperature)
	}
	if got.AccessCount != 1 {
		t.Errorf("AccessCount = %d, want 1", got.AccessCount)
	}
	if !got.LastAccessed.Valid {
		t.Error("LastAccessed should be set")
	}

	// Boost past 1.0 should cap at 1.0.
	must(t, st.UpdateTemperature(ctx, "aaaaaaaa", 0.95))
	must(t, st.BoostTemperature(ctx, "aaaaaaaa", 0.15))

	got, _ = st.GetNode(ctx, "aaaaaaaa")
	if got.Temperature != 1.0 {
		t.Errorf("Temperature = %f, want 1.0 (capped)", got.Temperature)
	}
}

func TestDecayTemperatures(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	must(t, st.UpsertNode(ctx, testNode("aaaaaaaa", "")))
	must(t, st.UpsertNode(ctx, testNode("bbbbbbbb", "")))
	must(t, st.UpdateTemperature(ctx, "aaaaaaaa", 1.0))
	must(t, st.UpdateTemperature(ctx, "bbbbbbbb", 0.0))

	affected, err := st.DecayTemperatures(ctx, 0.5)
	if err != nil {
		t.Fatalf("DecayTemperatures: %v", err)
	}
	if affected != 1 {
		t.Errorf("affected = %d, want 1 (only non-zero)", affected)
	}

	a, _ := st.GetNode(ctx, "aaaaaaaa")
	if a.Temperature != 0.5 {
		t.Errorf("Temperature = %f, want 0.5 (1.0 * 0.5)", a.Temperature)
	}

	b, _ := st.GetNode(ctx, "bbbbbbbb")
	if b.Temperature != 0.0 {
		t.Errorf("Temperature = %f, want 0.0 (unchanged)", b.Temperature)
	}
}
