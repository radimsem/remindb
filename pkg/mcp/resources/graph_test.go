package resources

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/radimsem/remindb/pkg/store"
)

func graphNode_(id string) *store.Node {
	return &store.Node{
		ID: id, SourceFile: "test.md", NodeType: "heading", Depth: 1,
		Label: "label " + id, Content: "content " + id,
		Format: "plain", TokenCount: 10, ContentHash: "hash" + id,
	}
}

func openGraphStore(t *testing.T) *store.Store {
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

// Fixture: parsed n1→n2, manual n2→n3, pending n1→"Unresolved"; n4 is orphan.
func TestHandleGraph_ParsedManualPending(t *testing.T) {
	st := openGraphStore(t)
	ctx := context.Background()

	for _, id := range []string{"aaaaaaaaaa1", "bbbbbbbbbb1", "cccccccccc1", "dddddddddd1"} {
		if err := st.UpsertNode(ctx, graphNode_(id)); err != nil {
			t.Fatalf("UpsertNode %s: %v", id, err)
		}
	}

	if err := st.UpsertRelation(ctx, &store.Relation{SourceNodeID: "aaaaaaaaaa1", TargetNodeID: "bbbbbbbbbb1", Weight: 1.5, Origin: store.OriginParsed}); err != nil {
		t.Fatalf("UpsertRelation parsed: %v", err)
	}
	if err := st.UpsertRelation(ctx, &store.Relation{SourceNodeID: "bbbbbbbbbb1", TargetNodeID: "cccccccccc1", Weight: 4.2, Origin: store.OriginManual}); err != nil {
		t.Fatalf("UpsertRelation manual: %v", err)
	}
	if err := st.InsertPendingRelation(ctx, &store.PendingRelation{SourceNodeID: "aaaaaaaaaa1", TargetLabel: "Unresolved", Weight: 2.43, Origin: store.OriginParsed}); err != nil {
		t.Fatalf("InsertPendingRelation: %v", err)
	}

	d := &Deps{Store: st}
	body, err := d.graphBody(ctx)
	if err != nil {
		t.Fatalf("graphBody: %v", err)
	}

	var env graphEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Node set is referenced-only: n1, n2, n3 — never the orphan n4.
	if len(env.Nodes) != 3 {
		t.Fatalf("nodes = %d, want 3 (referenced only, no orphan)", len(env.Nodes))
	}
	for _, n := range env.Nodes {
		if n.ID == "dddddddddd1" {
			t.Error("orphan node dddddddddd1 leaked into the graph")
		}

		if n.Label == "" || n.Type != "heading" {
			t.Errorf("node %s missing label/type: %+v", n.ID, n)
		}
	}

	// Resolved edges carry weight + origin; parsed and manual both present.
	if len(env.Edges) != 2 {
		t.Fatalf("edges = %d, want 2", len(env.Edges))
	}

	byOrigin := map[string]graphEdge{}
	for _, e := range env.Edges {
		byOrigin[e.Origin] = e
	}

	if e := byOrigin[store.OriginParsed]; e.Source != "aaaaaaaaaa1" || e.Target != "bbbbbbbbbb1" || e.Weight != 1.5 {
		t.Errorf("parsed edge = %+v, want aaaaaaaaaa1→bbbbbbbbbb1 w=1.5", e)
	}
	if e := byOrigin[store.OriginManual]; e.Source != "bbbbbbbbbb1" || e.Target != "cccccccccc1" || e.Weight != 4.2 {
		t.Errorf("manual edge = %+v, want bbbbbbbbbb1→cccccccccc1 w=4.2", e)
	}

	// Pending is distinguishable: no resolved target, carries the label + weight.
	if len(env.Pending) != 1 {
		t.Fatalf("pending = %d, want 1", len(env.Pending))
	}

	p := env.Pending[0]
	if p.Source != "aaaaaaaaaa1" || p.TargetLabel != "Unresolved" || p.Weight != 2.43 {
		t.Errorf("pending = %+v, want source=aaaaaaaaaa1 target_label=Unresolved w=2.43", p)
	}
}

func TestHandleGraph_EmptyDBStableShape(t *testing.T) {
	st := openGraphStore(t)
	d := &Deps{Store: st}

	body, err := d.graphBody(context.Background())
	if err != nil {
		t.Fatalf("graphBody: %v", err)
	}

	// Empty DB must still emit [] for all three keys, never null.
	got := string(body)
	want := `{"nodes":[],"edges":[],"pending":[]}`
	if got != want {
		t.Errorf("empty graph = %s, want %s", got, want)
	}
}
