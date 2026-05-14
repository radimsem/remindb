package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
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

func TestGetNodesByIDs(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	must(t, st.UpsertNode(ctx, testNode("aaaaaaaa", "")))
	must(t, st.UpsertNode(ctx, testNode("bbbbbbbb", "")))
	must(t, st.UpsertNode(ctx, testNode("cccccccc", "")))

	nodes, err := st.GetNodesByIDs(ctx, []string{"aaaaaaaa", "cccccccc", "missing0"})
	if err != nil {
		t.Fatalf("GetNodesByIDs: %v", err)
	}
	if len(nodes) != 2 {
		t.Errorf("len = %d, want 2 (missing IDs silently dropped)", len(nodes))
	}

	got := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		got[n.ID] = true
	}

	if !got["aaaaaaaa"] || !got["cccccccc"] {
		t.Errorf("returned IDs = %v, want aaaaaaaa and cccccccc", got)
	}

	empty, err := st.GetNodesByIDs(ctx, nil)
	if err != nil {
		t.Fatalf("GetNodesByIDs(nil): %v", err)
	}
	if empty != nil {
		t.Errorf("GetNodesByIDs(nil) = %v, want nil", empty)
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

func TestDeleteNodesByFiles(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	a := testNode("aaaaaaaa", "")
	a.SourceFile = "a.md"
	b := testNode("bbbbbbbb", "")
	b.SourceFile = "b.md"
	c := testNode("cccccccc", "")
	c.SourceFile = "c.md"

	must(t, st.UpsertNode(ctx, a))
	must(t, st.UpsertNode(ctx, b))
	must(t, st.UpsertNode(ctx, c))

	must(t, st.DeleteNodesByFiles(ctx, []string{"a.md", "b.md"}))

	if _, err := st.GetNode(ctx, "aaaaaaaa"); err == nil {
		t.Error("a.md node still present")
	}
	if _, err := st.GetNode(ctx, "bbbbbbbb"); err == nil {
		t.Error("b.md node still present")
	}
	if _, err := st.GetNode(ctx, "cccccccc"); err != nil {
		t.Errorf("c.md node unexpectedly removed: %v", err)
	}
}

func TestDeleteNodesByFiles_Empty(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	must(t, st.DeleteNodesByFiles(ctx, nil))
	must(t, st.DeleteNodesByFiles(ctx, []string{}))
}

func TestDeleteNodesByFiles_CascadesChildren(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	parent := testNode("parentaaaa", "")
	parent.SourceFile = "doc.md"
	child := testNode("childbbbbb", "parentaaaa")
	child.SourceFile = "doc.md"

	must(t, st.UpsertNode(ctx, parent))
	must(t, st.UpsertNode(ctx, child))

	must(t, st.DeleteNodesByFiles(ctx, []string{"doc.md"}))

	if _, err := st.GetNode(ctx, "childbbbbb"); err == nil {
		t.Error("child node should have been cascade-deleted")
	}
}

func TestStore_OpMuSerializes(t *testing.T) {
	st := openTestDB(t)

	const N = 3
	const hold = 20 * time.Millisecond

	start := time.Now()
	var wg sync.WaitGroup
	for range N {
		wg.Go(func() {
			st.OpMu.Lock()
			defer st.OpMu.Unlock()
			time.Sleep(hold)
		})
	}
	wg.Wait()

	elapsed := time.Since(start)
	if min := time.Duration(N) * hold; elapsed < min-5*time.Millisecond {
		t.Errorf("elapsed = %v, want >= %v (OpMu did not serialize goroutines)", elapsed, min)
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
		snapID, err := st.CreateSnapshotTx(ctx, tx, "abcdef0123456789", "first", "")
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

	cold, err := st.GetColdNodes(ctx, 0.5, 50)
	if err != nil {
		t.Fatalf("GetColdNodes: %v", err)
	}
	if len(cold) != 0 {
		t.Errorf("cold = %d, want 0 (node temp is 0.9)", len(cold))
	}

	must(t, st.UpdateTemperature(ctx, "aaaaaaaa", 0.1))

	cold, err = st.GetColdNodes(ctx, 0.5, 50)
	if err != nil {
		t.Fatalf("GetColdNodes: %v", err)
	}
	if len(cold) != 1 {
		t.Errorf("cold = %d, want 1", len(cold))
	}
}

func TestGetColdNodes_RespectsLimit(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	temps := []float64{0.05, 0.02, 0.04, 0.01, 0.03}
	ids := []string{"node0001", "node0002", "node0003", "node0004", "node0005"}
	for i, id := range ids {
		must(t, st.UpsertNode(ctx, testNode(id, "")))
		must(t, st.UpdateTemperature(ctx, id, temps[i]))
	}

	cold, err := st.GetColdNodes(ctx, 0.1, 3)
	if err != nil {
		t.Fatalf("GetColdNodes: %v", err)
	}
	if len(cold) != 3 {
		t.Fatalf("cold = %d, want 3", len(cold))
	}

	want := []string{"node0004", "node0002", "node0005"}
	for i, n := range cold {
		if n.ID != want[i] {
			t.Errorf("cold[%d].ID = %q, want %q", i, n.ID, want[i])
		}
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

	// Negative boost should floor at 0.0.
	must(t, st.UpdateTemperature(ctx, "aaaaaaaa", 0.1))
	must(t, st.BoostTemperature(ctx, "aaaaaaaa", -0.5))

	got, _ = st.GetNode(ctx, "aaaaaaaa")
	if got.Temperature != 0.0 {
		t.Errorf("Temperature = %f, want 0.0 (floored)", got.Temperature)
	}
}

func TestBoostTemperatureBatch_NegativeFloors(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	must(t, st.UpsertNode(ctx, testNode("aaaaaaaa", "")))
	must(t, st.UpsertNode(ctx, testNode("bbbbbbbb", "")))
	must(t, st.UpdateTemperature(ctx, "aaaaaaaa", 0.1))
	must(t, st.UpdateTemperature(ctx, "bbbbbbbb", 0.2))

	must(t, st.BoostTemperatureBatch(ctx, []string{"aaaaaaaa", "bbbbbbbb"}, -0.5))

	a, _ := st.GetNode(ctx, "aaaaaaaa")
	if a.Temperature != 0.0 {
		t.Errorf("a.Temperature = %f, want 0.0", a.Temperature)
	}
	b, _ := st.GetNode(ctx, "bbbbbbbb")
	if b.Temperature != 0.0 {
		t.Errorf("b.Temperature = %f, want 0.0", b.Temperature)
	}
}

// Verify foreign_keys=ON applies to every connection in the file-backed pool.
func TestOpen_FileBackedEnforcesForeignKeys(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "fk.db")
	st, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	ctx := context.Background()

	// Run several upserts concurrently to fan out across pool connections.
	const N = 8
	errs := make(chan error, N)
	for i := range N {
		go func() {
			n := testNode("orphan_x", "ghost_parent")
			n.ID = n.ID + string(rune('a'+i))
			errs <- st.UpsertNode(ctx, n)
		}()
	}

	gotViolation := false
	for range N {
		if err := <-errs; err != nil && strings.Contains(err.Error(), "FOREIGN KEY") {
			gotViolation = true
		}
	}
	if !gotViolation {
		t.Error("expected FOREIGN KEY violation on at least one connection; foreign_keys may be OFF in the pool")
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

func TestDecayTemperatures_FactorAboveOneCaps(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	must(t, st.UpsertNode(ctx, testNode("aaaaaaaa", "")))
	must(t, st.UpdateTemperature(ctx, "aaaaaaaa", 0.8))

	if _, err := st.DecayTemperatures(ctx, 2.0); err != nil {
		t.Fatalf("DecayTemperatures: %v", err)
	}

	got, _ := st.GetNode(ctx, "aaaaaaaa")
	if got.Temperature != 1.0 {
		t.Errorf("Temperature = %f, want 1.0 (capped)", got.Temperature)
	}
}

func TestGetDescendants(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	must(t, st.UpsertNode(ctx, testNode("rootroor", "")))
	must(t, st.UpsertNode(ctx, testNode("child001", "rootroor")))
	must(t, st.UpsertNode(ctx, testNode("child002", "rootroor")))
	must(t, st.UpsertNode(ctx, testNode("grand001", "child001")))

	desc, err := st.GetDescendants(ctx, "rootroor", 10)
	if err != nil {
		t.Fatalf("GetDescendants: %v", err)
	}
	if len(desc) != 3 {
		t.Errorf("len = %d, want 3 (2 children + 1 grandchild)", len(desc))
	}

	// Depth-limited: only direct children.
	desc, err = st.GetDescendants(ctx, "rootroor", 1)
	if err != nil {
		t.Fatalf("GetDescendants depth=1: %v", err)
	}
	if len(desc) != 2 {
		t.Errorf("len = %d, want 2 (direct children only)", len(desc))
	}
}

func TestGetSiblings(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	must(t, st.UpsertNode(ctx, testNode("rootroor", "")))
	must(t, st.UpsertNode(ctx, testNode("child001", "rootroor")))
	must(t, st.UpsertNode(ctx, testNode("child002", "rootroor")))
	must(t, st.UpsertNode(ctx, testNode("child003", "rootroor")))

	sibs, err := st.GetSiblings(ctx, "child001")
	if err != nil {
		t.Fatalf("GetSiblings: %v", err)
	}
	if len(sibs) != 2 {
		t.Errorf("len = %d, want 2 (excludes self)", len(sibs))
	}
}

func TestSearchRanked(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	n := testNode("aaaaaaaa", "")
	n.Content = "the quick brown fox jumps over the lazy dog"
	n.Label = "fox sentence"
	must(t, st.UpsertNode(ctx, n))

	results, err := st.SearchRanked(ctx, "fox", 10)
	if err != nil {
		t.Fatalf("SearchRanked: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len = %d, want 1", len(results))
	}
	if results[0].Node.ID != "aaaaaaaa" {
		t.Errorf("ID = %q", results[0].Node.ID)
	}
	if results[0].Rank >= 0 {
		t.Errorf("Rank = %f, want negative (BM25)", results[0].Rank)
	}
}

func TestGetDiffsBySnapshot(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	err := st.Tx(ctx, func(tx *sql.Tx) error {
		snapID, err := st.CreateSnapshotTx(ctx, tx, "hash1111", "v1", "")
		if err != nil {
			return err
		}
		if err := st.InsertDiffTx(ctx, tx, &DiffRecord{
			SnapshotID: snapID, NodeID: "node0001", Op: "add",
			NewHash: "h1", NewContent: "hello",
		}); err != nil {
			return err
		}
		return st.AdvanceCursorTx(ctx, tx, snapID)
	})
	must(t, err)

	diffs, err := st.GetDiffsBySnapshot(ctx, 1)
	if err != nil {
		t.Fatalf("GetDiffsBySnapshot: %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("len = %d, want 1", len(diffs))
	}
	if diffs[0].NodeID != "node0001" {
		t.Errorf("NodeID = %q", diffs[0].NodeID)
	}
}

func TestGetDiffsSince(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	// Create two snapshots.
	err := st.Tx(ctx, func(tx *sql.Tx) error {
		id, err := st.CreateSnapshotTx(ctx, tx, "hash1111", "v1", "")
		if err != nil {
			return err
		}
		if err := st.InsertDiffTx(ctx, tx, &DiffRecord{
			SnapshotID: id, NodeID: "node0001", Op: "add",
			NewHash: "h1", NewContent: "hello",
		}); err != nil {
			return err
		}
		return st.AdvanceCursorTx(ctx, tx, id)
	})
	must(t, err)

	err = st.Tx(ctx, func(tx *sql.Tx) error {
		id, err := st.CreateSnapshotTx(ctx, tx, "hash2222", "v2", "")
		if err != nil {
			return err
		}
		if err := st.InsertDiffTx(ctx, tx, &DiffRecord{
			SnapshotID: id, NodeID: "node0002", Op: "add",
			NewHash: "h2", NewContent: "world",
		}); err != nil {
			return err
		}
		return st.AdvanceCursorTx(ctx, tx, id)
	})
	must(t, err)

	// Diffs since snapshot 1 should only include snapshot 2's diffs.
	diffs, err := st.GetDiffsSince(ctx, 1)
	if err != nil {
		t.Fatalf("GetDiffsSince: %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("len = %d, want 1", len(diffs))
	}
	if diffs[0].NodeID != "node0002" {
		t.Errorf("NodeID = %q, want node0002", diffs[0].NodeID)
	}
}

func TestGetRootNodes(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	must(t, st.UpsertNode(ctx, testNode("rootroor", "")))
	must(t, st.UpsertNode(ctx, testNode("root0002", "")))
	must(t, st.UpsertNode(ctx, testNode("child001", "rootroor")))

	roots, err := st.GetRootNodes(ctx)
	if err != nil {
		t.Fatalf("GetRootNodes: %v", err)
	}
	if len(roots) != 2 {
		t.Errorf("len = %d, want 2", len(roots))
	}
}

func TestGetStats(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	// Empty DB.
	stats, err := st.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.NodeCount != 0 {
		t.Errorf("NodeCount = %d, want 0", stats.NodeCount)
	}

	// Add nodes with varying temperature.
	must(t, st.UpsertNode(ctx, testNode("aaaaaaaa", "")))
	must(t, st.UpdateTemperature(ctx, "aaaaaaaa", 0.8))
	must(t, st.UpsertNode(ctx, testNode("bbbbbbbb", "")))
	must(t, st.UpdateTemperature(ctx, "bbbbbbbb", 0.05))

	stats, err = st.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.NodeCount != 2 {
		t.Errorf("NodeCount = %d, want 2", stats.NodeCount)
	}
	if stats.HotCount != 1 {
		t.Errorf("HotCount = %d, want 1", stats.HotCount)
	}
	if stats.ColdCount != 1 {
		t.Errorf("ColdCount = %d, want 1", stats.ColdCount)
	}
}

func TestRewriteQuery(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"hello", "hello"},
		{"", ""},
		{"hello world", "hello OR world"},
		{"snapshot tests mock", "snapshot OR tests OR mock"},
		// FTS5 operators pass through unchanged.
		{"snapshot OR tests", "snapshot OR tests"},
		{"snapshot AND tests", "snapshot AND tests"},
		{"snapshot NOT mock", "snapshot NOT mock"},
		{`"exact phrase"`, `"exact phrase"`},
		{"label:snapshot", "label:snapshot"},
		{"snap*", "snap*"},
		{"NEAR(a b)", "NEAR(a b)"},
	}

	for _, tt := range tests {
		got := rewriteQuery(tt.in)
		if got != tt.want {
			t.Errorf("rewriteQuery(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSearchMultiWord(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	n1 := testNode("aaaaaaaa", "")
	n1.Content = "the quick brown fox jumps over the lazy dog"
	n1.Label = "fox sentence"
	must(t, st.UpsertNode(ctx, n1))

	n2 := testNode("bbbbbbbb", "")
	n2.Content = "a cat sat on a mat near the window"
	n2.Label = "cat sentence"
	must(t, st.UpsertNode(ctx, n2))

	// Multi-word query: "fox cat" should find both nodes via OR rewriting.
	results, err := st.Search(ctx, "fox cat", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("len = %d, want 2 (both nodes match via OR)", len(results))
	}

	// Single-word query still works.
	results, err = st.Search(ctx, "fox", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("len = %d, want 1", len(results))
	}
}

func TestListFileSummaries(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	upsert := func(id, file string, tokens int) {
		n := testNode(id, "")
		n.SourceFile = file
		n.TokenCount = tokens
		must(t, st.UpsertNode(ctx, n))
	}
	upsert("aaaaaaaa", "src/a.md", 7)
	upsert("bbbbbbbb", "src/a.md", 3)
	upsert("cccccccc", "src/b.md", 5)
	upsert("dddddddd", "orphan.md", 2)

	err := st.Tx(ctx, func(tx *sql.Tx) error {
		sidA, err := st.CreateSnapshotTx(ctx, tx, "hash1111", "first", "/repo/a")
		if err != nil {
			return err
		}

		for _, id := range []string{"aaaaaaaa", "bbbbbbbb", "cccccccc"} {
			if err := st.InsertDiffTx(ctx, tx, &DiffRecord{SnapshotID: sidA, NodeID: id, Op: "add"}); err != nil {
				return err
			}
		}

		sidB, err := st.CreateSnapshotTx(ctx, tx, "hash2222", "second", "/repo/b")
		if err != nil {
			return err
		}
		return st.InsertDiffTx(ctx, tx, &DiffRecord{SnapshotID: sidB, NodeID: "dddddddd", Op: "add"})
	})
	must(t, err)

	got, err := st.ListFileSummaries(ctx)
	if err != nil {
		t.Fatalf("ListFileSummaries: %v", err)
	}
	want := []FileSummary{
		{Path: "src/a.md", NodeCount: 2, TokenCount: 10, CompileRoot: "/repo/a"},
		{Path: "src/b.md", NodeCount: 1, TokenCount: 5, CompileRoot: "/repo/a"},
		{Path: "orphan.md", NodeCount: 1, TokenCount: 2, CompileRoot: "/repo/b"},
	}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (got %+v)", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("got[%d] = %+v, want %+v", i, got[i], w)
		}
	}
}

func TestListFileSummaries_NoDiffs(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	n := testNode("aaaaaaaa", "")
	n.SourceFile = "orphan.md"
	must(t, st.UpsertNode(ctx, n))

	got, err := st.ListFileSummaries(ctx)
	if err != nil {
		t.Fatalf("ListFileSummaries: %v", err)
	}

	if len(got) != 1 || got[0].CompileRoot != "" {
		t.Errorf("got = %+v, want one row with empty CompileRoot", got)
	}
}

func TestGetDiffsForNode(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	err := st.Tx(ctx, func(tx *sql.Tx) error {
		id, err := st.CreateSnapshotTx(ctx, tx, "hash1111", "v1", "")
		if err != nil {
			return err
		}
		if err := st.InsertDiffTx(ctx, tx, &DiffRecord{
			SnapshotID: id, NodeID: "node0001", Op: "add",
			NewHash: "h1", NewContent: "hello",
		}); err != nil {
			return err
		}
		if err := st.InsertDiffTx(ctx, tx, &DiffRecord{
			SnapshotID: id, NodeID: "node0002", Op: "add",
			NewHash: "h2", NewContent: "world",
		}); err != nil {
			return err
		}
		return st.AdvanceCursorTx(ctx, tx, id)
	})
	must(t, err)

	diffs, err := st.GetDiffsForNode(ctx, "node0001")
	if err != nil {
		t.Fatalf("GetDiffsForNode: %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("len = %d, want 1", len(diffs))
	}
	if diffs[0].Op != "add" {
		t.Errorf("Op = %q, want add", diffs[0].Op)
	}
}

func upsertNodeAt(t *testing.T, st *Store, ctx context.Context, id, compileRoot string) {
	t.Helper()
	err := st.Tx(ctx, func(tx *sql.Tx) error {
		if err := st.UpsertNodeTx(ctx, tx, testNode(id, "")); err != nil {
			return err
		}

		snapID, err := st.CreateSnapshotTx(ctx, tx, "h_"+id+"_"+compileRoot, "m", compileRoot)
		if err != nil {
			return err
		}

		if err := st.InsertDiffTx(ctx, tx, &DiffRecord{
			SnapshotID: snapID, NodeID: id, Op: "add", NewHash: "hash" + id, NewContent: "content " + id,
		}); err != nil {
			return err
		}

		return st.AdvanceCursorTx(ctx, tx, snapID)
	})
	must(t, err)
}

func TestGetNodesByCompileRoot(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	upsertNodeAt(t, st, ctx, "aaaaaaaa", "/abs/foo")
	upsertNodeAt(t, st, ctx, "bbbbbbbb", "/abs/foo")
	upsertNodeAt(t, st, ctx, "cccccccc", "/abs/bar")

	foo, err := st.GetNodesByCompileRoot(ctx, "/abs/foo")
	if err != nil {
		t.Fatalf("GetNodesByCompileRoot foo: %v", err)
	}
	if len(foo) != 2 {
		t.Errorf("foo nodes = %d, want 2", len(foo))
	}

	for _, n := range foo {
		if n.ID == "cccccccc" {
			t.Errorf("foo result includes node from bar: %s", n.ID)
		}
	}

	bar, err := st.GetNodesByCompileRoot(ctx, "/abs/bar")
	if err != nil {
		t.Fatalf("GetNodesByCompileRoot bar: %v", err)
	}
	if len(bar) != 1 || bar[0].ID != "cccccccc" {
		t.Errorf("bar nodes = %+v, want [cccccccc]", bar)
	}
}

func TestGetNodesByCompileRoot_SiblingPrefixNotMatched(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	upsertNodeAt(t, st, ctx, "aaaaaaaa", "/abs/foo")
	upsertNodeAt(t, st, ctx, "bbbbbbbb", "/abs/foo-bar")

	got, err := st.GetNodesByCompileRoot(ctx, "/abs/foo")
	if err != nil {
		t.Fatalf("GetNodesByCompileRoot: %v", err)
	}
	if len(got) != 1 || got[0].ID != "aaaaaaaa" {
		t.Errorf("got = %+v, want [aaaaaaaa] (no false positive on /abs/foo-bar)", got)
	}
}

func TestGetNodesByCompileRoot_LatestSnapshotWins(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	upsertNodeAt(t, st, ctx, "aaaaaaaa", "/abs/foo")
	upsertNodeAt(t, st, ctx, "aaaaaaaa", "/abs/bar")

	foo, err := st.GetNodesByCompileRoot(ctx, "/abs/foo")
	if err != nil {
		t.Fatalf("GetNodesByCompileRoot foo: %v", err)
	}
	if len(foo) != 0 {
		t.Errorf("foo nodes = %d, want 0 (node moved to bar)", len(foo))
	}

	bar, err := st.GetNodesByCompileRoot(ctx, "/abs/bar")
	if err != nil {
		t.Fatalf("GetNodesByCompileRoot bar: %v", err)
	}
	if len(bar) != 1 || bar[0].ID != "aaaaaaaa" {
		t.Errorf("bar nodes = %+v, want [aaaaaaaa] (latest snapshot wins)", bar)
	}
}

func TestSetPinned(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	must(t, st.UpsertNode(ctx, testNode("aaaaaaaa", "")))

	got, err := st.GetNode(ctx, "aaaaaaaa")
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if got.Pinned {
		t.Error("Pinned = true on a fresh node, want false")
	}

	must(t, st.SetPinned(ctx, "aaaaaaaa", true, nil))

	got, err = st.GetNode(ctx, "aaaaaaaa")
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if !got.Pinned {
		t.Error("Pinned = false after SetPinned(true)")
	}

	must(t, st.SetPinned(ctx, "aaaaaaaa", false, nil))

	got, err = st.GetNode(ctx, "aaaaaaaa")
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if got.Pinned {
		t.Error("Pinned = true after SetPinned(false)")
	}
}

func TestDecayTemperatures_SkipsPinned(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	must(t, st.UpsertNode(ctx, testNode("aaaaaaaa", "")))
	must(t, st.UpsertNode(ctx, testNode("bbbbbbbb", "")))
	must(t, st.UpdateTemperature(ctx, "aaaaaaaa", 0.8))
	must(t, st.UpdateTemperature(ctx, "bbbbbbbb", 0.8))
	must(t, st.SetPinned(ctx, "aaaaaaaa", true, nil))

	affected, err := st.DecayTemperatures(ctx, 0.5)
	if err != nil {
		t.Fatalf("DecayTemperatures: %v", err)
	}
	if affected != 1 {
		t.Errorf("affected = %d, want 1 (pinned row excluded)", affected)
	}

	a, _ := st.GetNode(ctx, "aaaaaaaa")
	if a.Temperature != 0.8 {
		t.Errorf("pinned Temperature = %f, want 0.8 (unchanged)", a.Temperature)
	}

	b, _ := st.GetNode(ctx, "bbbbbbbb")
	if b.Temperature != 0.4 {
		t.Errorf("unpinned Temperature = %f, want 0.4 (0.8 * 0.5)", b.Temperature)
	}
}

func TestGetColdNodes_SkipsPinned(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	must(t, st.UpsertNode(ctx, testNode("aaaaaaaa", "")))
	must(t, st.UpsertNode(ctx, testNode("bbbbbbbb", "")))
	must(t, st.UpdateTemperature(ctx, "aaaaaaaa", 0.05))
	must(t, st.UpdateTemperature(ctx, "bbbbbbbb", 0.05))
	must(t, st.SetPinned(ctx, "aaaaaaaa", true, nil))

	cold, err := st.GetColdNodes(ctx, 0.1, 50)
	if err != nil {
		t.Fatalf("GetColdNodes: %v", err)
	}

	if len(cold) != 1 {
		t.Fatalf("cold = %d, want 1 (pinned row excluded)", len(cold))
	}
	if cold[0].ID != "bbbbbbbb" {
		t.Errorf("cold[0].ID = %q, want bbbbbbbb (pinned aaaaaaaa excluded)", cold[0].ID)
	}
}

func TestSetPinned_WithTemperatureOverride(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	must(t, st.UpsertNode(ctx, testNode("aaaaaaaa", "")))
	must(t, st.UpdateTemperature(ctx, "aaaaaaaa", 0.2))

	target := 0.9
	must(t, st.SetPinned(ctx, "aaaaaaaa", true, &target))

	got, err := st.GetNode(ctx, "aaaaaaaa")
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}

	if !got.Pinned {
		t.Error("Pinned = false after SetPinned(true, &0.9)")
	}
	if got.Temperature != 0.9 {
		t.Errorf("Temperature = %f, want 0.9 (override applied)", got.Temperature)
	}
}
