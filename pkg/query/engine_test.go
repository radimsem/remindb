package query

import (
	"context"
	"database/sql"
	"testing"

	"github.com/radimsem/remindb/internal/testutil"
	"github.com/radimsem/remindb/pkg/store"
)

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
	st := testutil.OpenTestDB(t)
	seedTree(t, st)

	eng := NewEngine(st)
	ctx := context.Background()

	result, err := eng.Fetch(ctx, "child001", 1000, 0)
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
	st := testutil.OpenTestDB(t)
	seedTree(t, st)

	eng := NewEngine(st)
	ctx := context.Background()

	// Budget of 50: anchor (30) + maybe one small context node.
	result, err := eng.Fetch(ctx, "child001", 50, 0)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if result.TokensUsed > 50 {
		t.Errorf("TokensUsed = %d, exceeds budget 50", result.TokensUsed)
	}
}

func TestFetch_IncludesContext(t *testing.T) {
	st := testutil.OpenTestDB(t)
	seedTree(t, st)

	eng := NewEngine(st)
	ctx := context.Background()

	result, err := eng.Fetch(ctx, "child001", 1000, 0)
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

func TestFetch_DepthBoundary(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()

	// Build a linear chain: root -> d1 -> d2 -> d3 -> d4.
	chain := []*store.Node{
		{ID: "rootroor", SourceFile: "doc.md", NodeType: "heading", Depth: 1,
			Label: "root", Content: "root", Format: "plain", TokenCount: 5, ContentHash: "hr"},
		{ID: "d1000000", ParentID: "rootroor", SourceFile: "doc.md", NodeType: "text", Depth: 2,
			Label: "d1", Content: "d1", Format: "plain", TokenCount: 5, ContentHash: "h1"},
		{ID: "d2000000", ParentID: "d1000000", SourceFile: "doc.md", NodeType: "text", Depth: 3,
			Label: "d2", Content: "d2", Format: "plain", TokenCount: 5, ContentHash: "h2"},
		{ID: "d3000000", ParentID: "d2000000", SourceFile: "doc.md", NodeType: "text", Depth: 4,
			Label: "d3", Content: "d3", Format: "plain", TokenCount: 5, ContentHash: "h3"},
		{ID: "d4000000", ParentID: "d3000000", SourceFile: "doc.md", NodeType: "text", Depth: 5,
			Label: "d4", Content: "d4", Format: "plain", TokenCount: 5, ContentHash: "h4"},
	}
	for _, n := range chain {
		if err := st.UpsertNode(ctx, n); err != nil {
			t.Fatalf("UpsertNode %s: %v", n.ID, err)
		}
	}

	eng := NewEngine(st)

	tests := []struct {
		name          string
		depth         int
		wantDescs     []string
		dontWantDescs []string
	}{
		{"depth_2", 2, []string{"d1000000", "d2000000"}, []string{"d3000000", "d4000000"}},
		{"depth_3", 3, []string{"d1000000", "d2000000", "d3000000"}, []string{"d4000000"}},
		{"depth_4", 4, []string{"d1000000", "d2000000", "d3000000", "d4000000"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := eng.Fetch(ctx, "rootroor", 10000, tt.depth)
			if err != nil {
				t.Fatalf("Fetch: %v", err)
			}

			ids := make(map[string]bool)
			for _, sn := range result.Nodes {
				ids[sn.Node.ID] = true
			}

			for _, want := range tt.wantDescs {
				if !ids[want] {
					t.Errorf("depth=%d: missing %s", tt.depth, want)
				}
			}

			for _, skip := range tt.dontWantDescs {
				if ids[skip] {
					t.Errorf("depth=%d: unexpected %s", tt.depth, skip)
				}
			}
		})
	}
}

func TestClampDepth(t *testing.T) {
	tests := []struct {
		in, want int
	}{
		{0, DefaultMaxDepth},
		{-5, DefaultMaxDepth},
		{1, 1},
		{50, 50},
		{MaxDepthCap, MaxDepthCap},
		{MaxDepthCap + 1, MaxDepthCap},
		{10000, MaxDepthCap},
	}

	for _, tt := range tests {
		if got := clampDepth(tt.in); got != tt.want {
			t.Errorf("clampDepth(%d) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestSearch_ReturnsRankedResults(t *testing.T) {
	st := testutil.OpenTestDB(t)
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
	st := testutil.OpenTestDB(t)
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
	st := testutil.OpenTestDB(t)
	ctx := context.Background()

	// Create two snapshots with diffs.
	err := st.Tx(ctx, func(tx *sql.Tx) error {
		id, err := st.CreateSnapshotTx(ctx, tx, "hash1111", "v1", "")
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
		id, err := st.CreateSnapshotTx(ctx, tx, "hash2222", "v2", "")
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

func TestFormatCompact(t *testing.T) {
	t.Run("with_results", func(t *testing.T) {
		result := &Result{
			Nodes: []ScoredNode{
				{Node: &store.Node{
					ID: "abc123", NodeType: "heading", Label: "Auth Config",
					SourceFile: "docs/auth.md", Temperature: 0.75, TokenCount: 42,
				}},
				{Node: &store.Node{
					ID: "def456", NodeType: "text", Label: "Rate limiting overview.",
					SourceFile: "docs/api.md", Temperature: 0.30, TokenCount: 110,
				}},
			},
		}

		out := FormatCompact(result)
		expected := "[heading] Auth Config (id=abc123 file=docs/auth.md temp=0.75 tok=42)\n" +
			"[text] Rate limiting overview. (id=def456 file=docs/api.md temp=0.30 tok=110)\n"

		if out != expected {
			t.Errorf("FormatCompact =\n%q\nwant\n%q", out, expected)
		}
	})

	t.Run("empty", func(t *testing.T) {
		out := FormatCompact(&Result{})
		if out != "no results" {
			t.Errorf("FormatCompact empty = %q, want %q", out, "no results")
		}
	})
}
