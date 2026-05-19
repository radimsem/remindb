package inspect_test

import (
	"context"
	"strings"
	"testing"

	"github.com/radimsem/remindb/internal/testutil"
	"github.com/radimsem/remindb/pkg/inspect"
	"github.com/radimsem/remindb/pkg/store"
)

func testNode(id, parent, nodeType string) *store.Node {
	return &store.Node{
		ID: id, ParentID: parent,
		SourceFile: "test.md", NodeType: nodeType, Depth: 1,
		Label: "label " + id, Content: "content " + id,
		Format: "plain", TokenCount: 10, ContentHash: "hash" + id,
	}
}

func TestCollect_Empty(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()

	s, err := inspect.Collect(ctx, st)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	if s.NodeCount != 0 {
		t.Errorf("NodeCount = %d, want 0", s.NodeCount)
	}
	if s.Latest != nil {
		t.Errorf("Latest = %+v, want nil", s.Latest)
	}
}

func TestCollect_PopulatesAllFields(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()

	n1 := testNode("aaaaaaaa", "", "heading")
	n2 := testNode("bbbbbbbb", "", "list")
	n3 := testNode("cccccccc", "", "list")
	for _, n := range []*store.Node{n1, n2, n3} {
		if err := st.UpsertNode(ctx, n); err != nil {
			t.Fatalf("UpsertNode: %v", err)
		}
	}

	if err := st.UpdateTemperature(ctx, "aaaaaaaa", 0.8); err != nil {
		t.Fatalf("UpdateTemperature: %v", err)
	}
	if err := st.SetPinned(ctx, "aaaaaaaa", true, nil); err != nil {
		t.Fatalf("SetPinned: %v", err)
	}

	if err := st.UpsertRelation(ctx, &store.Relation{
		SourceNodeID: "aaaaaaaa", TargetNodeID: "bbbbbbbb",
		Weight: 1.0, Origin: store.OriginParsed,
	}); err != nil {
		t.Fatalf("UpsertRelation: %v", err)
	}
	if err := st.UpsertRelation(ctx, &store.Relation{
		SourceNodeID: "aaaaaaaa", TargetNodeID: "cccccccc",
		Weight: 1.0, Origin: store.OriginManual,
	}); err != nil {
		t.Fatalf("UpsertRelation: %v", err)
	}

	s, err := inspect.Collect(ctx, st)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	if s.NodeCount != 3 {
		t.Errorf("NodeCount = %d, want 3", s.NodeCount)
	}
	if s.NodeCountsByType["heading"] != 1 || s.NodeCountsByType["list"] != 2 {
		t.Errorf("NodeCountsByType = %v, want heading:1 list:2", s.NodeCountsByType)
	}
	if s.PinnedCount != 1 {
		t.Errorf("PinnedCount = %d, want 1", s.PinnedCount)
	}
	if s.RelationCount != 2 {
		t.Errorf("RelationCount = %d, want 2", s.RelationCount)
	}
	if s.RelationsByOrigin[store.OriginParsed] != 1 || s.RelationsByOrigin[store.OriginManual] != 1 {
		t.Errorf("RelationsByOrigin = %v, want parsed:1 manual:1", s.RelationsByOrigin)
	}
	if s.TokenCountTotal != 30 {
		t.Errorf("TokenCountTotal = %d, want 30", s.TokenCountTotal)
	}
	if s.FTSRowCount != 3 {
		t.Errorf("FTSRowCount = %d, want 3", s.FTSRowCount)
	}
}

func TestCollect_RelationCountIncludesPending(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()

	for _, n := range []*store.Node{
		testNode("aaaaaaaa", "", "heading"),
		testNode("bbbbbbbb", "", "list"),
	} {
		if err := st.UpsertNode(ctx, n); err != nil {
			t.Fatalf("UpsertNode: %v", err)
		}
	}

	if err := st.UpsertRelation(ctx, &store.Relation{
		SourceNodeID: "aaaaaaaa", TargetNodeID: "bbbbbbbb",
		Weight: 1.0, Origin: store.OriginParsed,
	}); err != nil {
		t.Fatalf("UpsertRelation: %v", err)
	}
	for range 3 {
		if err := st.InsertPendingRelation(ctx, &store.PendingRelation{
			SourceNodeID: "aaaaaaaa", TargetLabel: "missing",
			Weight: 1.0, Origin: store.OriginParsed,
		}); err != nil {
			t.Fatalf("InsertPendingRelation: %v", err)
		}
	}

	s, err := inspect.Collect(ctx, st)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	originSum := 0
	for _, v := range s.RelationsByOrigin {
		originSum += v
	}

	want := originSum + s.PendingRelationCount
	if s.RelationCount != want {
		t.Errorf("RelationCount = %d, want Σorigins(%d) + pending(%d) = %d",
			s.RelationCount, originSum, s.PendingRelationCount, want)
	}
	if s.PendingRelationCount != 3 {
		t.Errorf("PendingRelationCount = %d, want 3", s.PendingRelationCount)
	}
}

// MemoryStats stays lossless: large token totals render as exact integers.
func TestFormat_TokenCountExact(t *testing.T) {
	s := &inspect.Stats{
		DBPath:          ":memory:",
		NodeCount:       7,
		TokenCountTotal: 359297,
	}

	out := inspect.Format(s)
	if !strings.Contains(out, "359297 tokens") {
		t.Errorf("Format must emit exact token count, got:\n%s", out)
	}
}

func TestFormat_TreeBranches(t *testing.T) {
	s := &inspect.Stats{
		DBPath:        ":memory:",
		NodeCount:     12,
		SnapshotCount: 0,
		HotCount:      4,
		ColdCount:     2,
		PinnedCount:   1,
		NodeCountsByType: map[string]int{
			"heading": 7,
			"list":    5,
		},
		RelationCount: 3,
		RelationsByOrigin: map[string]int{
			store.OriginParsed: 2,
			store.OriginManual: 1,
		},
		TokenCountTotal: 100,
		FTSRowCount:     12,
	}

	out := inspect.Format(s)

	for _, want := range []string{
		"Database: :memory:",
		"Nodes:",
		"12 (100 tokens)",
		"heading:",
		"list:",
		"Relations:",
		store.OriginManual + ":",
		store.OriginParsed + ":",
		"pinned:",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Format missing %q in:\n%s", want, out)
		}
	}

	if !strings.Contains(out, "├─") || !strings.Contains(out, "└─") {
		t.Errorf("Format missing tree glyphs in:\n%s", out)
	}
}
