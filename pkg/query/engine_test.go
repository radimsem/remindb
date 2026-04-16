package query

import (
	"context"
	"database/sql"
	"testing"

	"github.com/radimsem/remindb/pkg/store"
)

func openTestDB(t *testing.T) *store.Store {
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

func seedTree(t *testing.T, st *store.Store) {
	t.Helper()
	ctx := context.Background()

	nodes := []*store.Node{
		{ID: "rootroor", SourceFile: "doc.md", NodeType: "heading", Depth: 1,
			Label: "root heading", Content: "root content about architecture",
			Format: "plain", TokenCount: 20, ContentHash: "h_root"},
		{ID: "child001", ParentID: "rootroor", SourceFile: "doc.md", NodeType: "text", Depth: 2,
			Label: "first section", Content: "text about database design patterns",
			Format: "plain", TokenCount: 30, ContentHash: "h_child1"},
		{ID: "child002", ParentID: "rootroor", SourceFile: "doc.md", NodeType: "code", Depth: 2,
			Label: "code example", Content: "SELECT * FROM nodes",
			Format: "plain", TokenCount: 15, ContentHash: "h_child2"},
		{ID: "grand001", ParentID: "child001", SourceFile: "doc.md", NodeType: "text", Depth: 3,
			Label: "subsection", Content: "details about query optimization",
			Format: "plain", TokenCount: 25, ContentHash: "h_grand1"},
	}

	for _, n := range nodes {
		if err := st.UpsertNode(ctx, n); err != nil {
			t.Fatalf("UpsertNode %s: %v", n.ID, err)
		}
	}

	// Set varying temperatures.
	if err := st.UpdateTemperature(ctx, "rootroor", 0.8); err != nil {
		t.Fatalf("UpdateTemperature: %v", err)
	}
	if err := st.UpdateTemperature(ctx, "child001", 0.6); err != nil {
		t.Fatalf("UpdateTemperature: %v", err)
	}
	if err := st.UpdateTemperature(ctx, "child002", 0.3); err != nil {
		t.Fatalf("UpdateTemperature: %v", err)
	}
	if err := st.UpdateTemperature(ctx, "grand001", 0.9); err != nil {
		t.Fatalf("UpdateTemperature: %v", err)
	}
}

func TestFetch_IncludesAnchor(t *testing.T) {
	st := openTestDB(t)
	seedTree(t, st)

	eng := NewEngine(st)
	ctx := context.Background()

	result, err := eng.Fetch(ctx, "child001", 1000)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if len(result.Nodes) == 0 {
		t.Fatal("empty result")
	}
	if result.Nodes[0].Node.ID != "child001" {
		t.Errorf("first node = %q, want anchor child001", result.Nodes[0].Node.ID)
	}
}

func TestFetch_RespectsbudgetBudget(t *testing.T) {
	st := openTestDB(t)
	seedTree(t, st)

	eng := NewEngine(st)
	ctx := context.Background()

	// Budget of 50: anchor (30) + maybe one small context node.
	result, err := eng.Fetch(ctx, "child001", 50)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if result.TokensUsed > 50 {
		t.Errorf("TokensUsed = %d, exceeds budget 50", result.TokensUsed)
	}
}

func TestFetch_IncludesContext(t *testing.T) {
	st := openTestDB(t)
	seedTree(t, st)

	eng := NewEngine(st)
	ctx := context.Background()

	result, err := eng.Fetch(ctx, "child001", 1000)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	// Should include ancestor (rootroor), descendant (grand001), sibling (child002).
	ids := make(map[string]bool)
	for _, sn := range result.Nodes {
		ids[sn.Node.ID] = true
	}

	for _, want := range []string{"rootroor", "grand001", "child002"} {
		if !ids[want] {
			t.Errorf("missing context node %s", want)
		}
	}
}

func TestSearch_ReturnsRankedResults(t *testing.T) {
	st := openTestDB(t)
	seedTree(t, st)

	eng := NewEngine(st)
	ctx := context.Background()

	result, err := eng.Search(ctx, "database", 1000)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(result.Nodes) == 0 {
		t.Fatal("no results for 'database'")
	}
}

func TestSearch_RespectsBudget(t *testing.T) {
	st := openTestDB(t)
	seedTree(t, st)

	eng := NewEngine(st)
	ctx := context.Background()

	result, err := eng.Search(ctx, "about", 25)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if result.TokensUsed > 25 {
		t.Errorf("TokensUsed = %d, exceeds budget 25", result.TokensUsed)
	}
}

func TestDelta(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	// Create two snapshots with diffs.
	err := st.Tx(ctx, func(tx *sql.Tx) error {
		id, err := st.CreateSnapshotTx(ctx, tx, "hash1111", "v1")
		if err != nil {
			return err
		}
		if err := st.InsertDiffTx(ctx, tx, &store.DiffRecord{
			SnapshotID: id, NodeID: "node0001", Op: "add",
			NewHash: "h1", NewContent: "hello",
		}); err != nil {
			return err
		}
		return st.AdvanceCursorTx(ctx, tx, id)
	})
	if err != nil {
		t.Fatalf("Tx v1: %v", err)
	}

	err = st.Tx(ctx, func(tx *sql.Tx) error {
		id, err := st.CreateSnapshotTx(ctx, tx, "hash2222", "v2")
		if err != nil {
			return err
		}
		if err := st.InsertDiffTx(ctx, tx, &store.DiffRecord{
			SnapshotID: id, NodeID: "node0002", Op: "add",
			NewHash: "h2", NewContent: "world",
		}); err != nil {
			return err
		}
		return st.AdvanceCursorTx(ctx, tx, id)
	})
	if err != nil {
		t.Fatalf("Tx v2: %v", err)
	}

	eng := NewEngine(st)

	diffs, err := eng.Delta(ctx, 1)
	if err != nil {
		t.Fatalf("Delta: %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("len = %d, want 1", len(diffs))
	}
	if diffs[0].NodeID != "node0002" {
		t.Errorf("NodeID = %q, want node0002", diffs[0].NodeID)
	}
}

func TestFormat(t *testing.T) {
	result := &Result{
		Nodes: []ScoredNode{
			{Node: &store.Node{NodeType: "heading", Label: "Title", Content: "hello world"}},
			{Node: &store.Node{NodeType: "code", Label: "Example", Content: "x := 1"}},
		},
	}

	out := Format(result)
	if out == "" {
		t.Fatal("empty output")
	}

	expected := "[heading] Title\nhello world\n\n---\n\n[code] Example\nx := 1\n"
	if out != expected {
		t.Errorf("Format =\n%q\nwant\n%q", out, expected)
	}
}
