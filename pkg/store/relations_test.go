package store

import (
	"context"
	"database/sql"
	"testing"
)

func mustNode(t *testing.T, st *Store, id, parent string) *Node {
	t.Helper()
	n := testNode(id, parent)
	must(t, st.UpsertNode(context.Background(), n))
	return n
}

func countRelations(t *testing.T, st *Store, source, target string) int {
	t.Helper()

	query := `SELECT count(*) FROM relations WHERE 1=1`
	var args []any
	if source != "" {
		query += ` AND source_node_id = ?`
		args = append(args, source)
	}
	if target != "" {
		query += ` AND target_node_id = ?`
		args = append(args, target)
	}

	var n int
	if err := st.db.QueryRowContext(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("countRelations: %v", err)
	}
	return n
}

func TestUpsertRelation_Resolved(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	mustNode(t, st, "aaaaaaaaaa1", "")
	mustNode(t, st, "bbbbbbbbbb1", "")

	r := &Relation{SourceNodeID: "aaaaaaaaaa1", TargetNodeID: "bbbbbbbbbb1", Weight: 1.5, Origin: OriginParsed}
	must(t, st.UpsertRelation(ctx, r))

	related, err := st.GetRelatedNodes(ctx, "aaaaaaaaaa1", DirectionOut, 1, 0, 10)
	if err != nil {
		t.Fatalf("GetRelatedNodes: %v", err)
	}

	if len(related) != 1 || related[0].Node.ID != "bbbbbbbbbb1" {
		t.Fatalf("related = %+v, want [bbbbbbbbbb1]", related)
	}
	if related[0].Weight != 1.5 {
		t.Errorf("weight = %f, want 1.5", related[0].Weight)
	}
	if related[0].Hop != 1 {
		t.Errorf("hop = %d, want 1", related[0].Hop)
	}
}

func TestUpsertRelation_DuplicateUpdatesWeight(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	mustNode(t, st, "aaaaaaaaaa1", "")
	mustNode(t, st, "bbbbbbbbbb1", "")

	must(t, st.UpsertRelation(ctx, &Relation{SourceNodeID: "aaaaaaaaaa1", TargetNodeID: "bbbbbbbbbb1", Weight: 1.0, Origin: OriginParsed}))
	must(t, st.UpsertRelation(ctx, &Relation{SourceNodeID: "aaaaaaaaaa1", TargetNodeID: "bbbbbbbbbb1", Weight: 2.5, Origin: OriginParsed}))

	if n := countRelations(t, st, "aaaaaaaaaa1", "bbbbbbbbbb1"); n != 1 {
		t.Errorf("relations row count = %d, want 1 (UNIQUE(source, target, origin) blocks duplicate row)", n)
	}

	related, _ := st.GetRelatedNodes(ctx, "aaaaaaaaaa1", DirectionOut, 1, 0, 10)
	if len(related) != 1 {
		t.Fatalf("got %d related, want 1", len(related))
	}
	if related[0].Weight != 2.5 {
		t.Errorf("weight = %f, want 2.5 (ON CONFLICT DO UPDATE should overwrite weight)", related[0].Weight)
	}
}

func TestUpsertRelation_ParsedAndManualCoexist(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	mustNode(t, st, "aaaaaaaaaa1", "")
	mustNode(t, st, "bbbbbbbbbb1", "")

	must(t, st.UpsertRelation(ctx, &Relation{SourceNodeID: "aaaaaaaaaa1", TargetNodeID: "bbbbbbbbbb1", Weight: 1.0, Origin: OriginParsed}))
	must(t, st.UpsertRelation(ctx, &Relation{SourceNodeID: "aaaaaaaaaa1", TargetNodeID: "bbbbbbbbbb1", Weight: 1.0, Origin: OriginManual}))

	if n := countRelations(t, st, "aaaaaaaaaa1", "bbbbbbbbbb1"); n != 2 {
		t.Errorf("relations row count = %d, want 2 (parsed + manual coexist)", n)
	}

	related, _ := st.GetRelatedNodes(ctx, "aaaaaaaaaa1", DirectionOut, 1, 0, 10)
	if len(related) != 1 {
		t.Errorf("got %d unique targets from GetRelatedNodes, want 1 (GROUP BY dedupes)", len(related))
	}
}

func TestInsertPendingRelation(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	mustNode(t, st, "aaaaaaaaaa1", "")

	p := &PendingRelation{
		SourceNodeID: "aaaaaaaaaa1",
		TargetLabel:  "Architecture",
		TargetSource: "docs/ARCH.md",
		Weight:       2.0,
		Origin:       OriginParsed,
	}
	must(t, st.InsertPendingRelation(ctx, p))

	pending, err := st.GetAllPendingRelations(ctx)
	if err != nil {
		t.Fatalf("GetAllPendingRelations: %v", err)
	}

	if len(pending) != 1 {
		t.Fatalf("len(pending) = %d, want 1", len(pending))
	}

	got := pending[0]
	if got.SourceNodeID != "aaaaaaaaaa1" {
		t.Errorf("source = %q, want aaaaaaaaaa1", got.SourceNodeID)
	}
	if got.TargetLabel != "Architecture" {
		t.Errorf("target_label = %q, want Architecture", got.TargetLabel)
	}
	if got.TargetSource != "docs/ARCH.md" {
		t.Errorf("target_source = %q, want docs/ARCH.md", got.TargetSource)
	}
	if got.Weight != 2.0 {
		t.Errorf("weight = %f, want 2.0", got.Weight)
	}
	if got.Origin != OriginParsed {
		t.Errorf("origin = %q, want %s", got.Origin, OriginParsed)
	}
	if got.CreatedAt == 0 {
		t.Errorf("created_at = 0, want non-zero (default unixepoch())")
	}
}

func TestInsertPendingRelation_NullableFields(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	mustNode(t, st, "aaaaaaaaaa1", "")

	// label only, no source, no id_hint
	must(t, st.InsertPendingRelation(ctx, &PendingRelation{
		SourceNodeID: "aaaaaaaaaa1",
		TargetLabel:  "Lonely",
		Weight:       1.0,
		Origin:       OriginParsed,
	}))

	pending, _ := st.GetAllPendingRelations(ctx)
	if len(pending) != 1 {
		t.Fatalf("len(pending) = %d", len(pending))
	}

	got := pending[0]
	if got.TargetLabel != "Lonely" {
		t.Errorf("target_label = %q, want Lonely", got.TargetLabel)
	}
	if got.TargetSource != "" {
		t.Errorf("target_source = %q, want empty (NULL round-trips to \"\")", got.TargetSource)
	}
	if got.TargetIDHint != "" {
		t.Errorf("target_id_hint = %q, want empty (NULL round-trips to \"\")", got.TargetIDHint)
	}
}

func TestDeleteParsedPendingForSource(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	mustNode(t, st, "aaaaaaaaaa1", "")

	must(t, st.InsertPendingRelation(ctx, &PendingRelation{
		SourceNodeID: "aaaaaaaaaa1", TargetLabel: "X", Weight: 1, Origin: OriginParsed,
	}))
	must(t, st.InsertPendingRelation(ctx, &PendingRelation{
		SourceNodeID: "aaaaaaaaaa1", TargetLabel: "Y", Weight: 1, Origin: OriginManual,
	}))

	must(t, st.Tx(ctx, func(tx *sql.Tx) error {
		return st.DeleteParsedPendingForSourceTx(ctx, tx, "aaaaaaaaaa1")
	}))

	pending, _ := st.GetAllPendingRelations(ctx)
	if len(pending) != 1 {
		t.Fatalf("len(pending) = %d, want 1 (manual survives, parsed gone)", len(pending))
	}
	if pending[0].Origin != OriginManual {
		t.Errorf("origin = %s, want %s", pending[0].Origin, OriginManual)
	}
}

func TestCascadeRePendOnTargetDelete(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	src := mustNode(t, st, "src11111111", "")
	tgt := mustNode(t, st, "tgt11111111", "")

	tgt.Label = "DeletedHeading"
	tgt.SourceFile = "docs/file.md"
	must(t, st.UpsertNode(ctx, tgt))

	must(t, st.UpsertRelation(ctx, &Relation{
		SourceNodeID: src.ID, TargetNodeID: tgt.ID, Weight: 2.5, Origin: OriginParsed,
	}))

	must(t, st.DeleteNode(ctx, tgt.ID))

	if n := countRelations(t, st, src.ID, ""); n != 0 {
		t.Errorf("relations row count = %d, want 0 (cascade should fire after BEFORE trigger)", n)
	}

	pending, err := st.GetAllPendingRelations(ctx)
	if err != nil {
		t.Fatalf("GetAllPendingRelations: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("len(pending) = %d, want 1 (trigger should re-pend)", len(pending))
	}

	p := pending[0]
	if p.SourceNodeID != src.ID {
		t.Errorf("source = %s, want %s", p.SourceNodeID, src.ID)
	}
	if p.TargetLabel != "DeletedHeading" {
		t.Errorf("target_label = %s, want DeletedHeading", p.TargetLabel)
	}
	if p.TargetSource != "docs/file.md" {
		t.Errorf("target_source = %s, want docs/file.md", p.TargetSource)
	}
	if p.Weight != 2.5 {
		t.Errorf("weight = %f, want 2.5", p.Weight)
	}
	if p.Origin != OriginParsed {
		t.Errorf("origin = %s, want %s", p.Origin, OriginParsed)
	}
}

func TestCascadeNoRePendOnSourceDelete(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	src := mustNode(t, st, "src11111111", "")
	tgt := mustNode(t, st, "tgt11111111", "")

	must(t, st.UpsertRelation(ctx, &Relation{
		SourceNodeID: src.ID, TargetNodeID: tgt.ID, Weight: 1, Origin: OriginParsed,
	}))

	if n := countRelations(t, st, "", ""); n != 1 {
		t.Fatalf("relations row count before delete = %d, want 1", n)
	}

	must(t, st.DeleteNode(ctx, src.ID))

	if n := countRelations(t, st, "", tgt.ID); n != 0 {
		t.Errorf("relations row count = %d, want 0 (FK cascade should drop the edge)", n)
	}

	pending, _ := st.GetAllPendingRelations(ctx)
	if len(pending) != 0 {
		t.Errorf("len(pending) = %d, want 0 (deleting source should not re-pend)", len(pending))
	}
}

func TestGetRelatedNodes_MultiHop(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	mustNode(t, st, "aaa11111111", "")
	mustNode(t, st, "bbb11111111", "")
	mustNode(t, st, "ccc11111111", "")

	must(t, st.UpsertRelation(ctx, &Relation{SourceNodeID: "aaa11111111", TargetNodeID: "bbb11111111", Weight: 1, Origin: OriginParsed}))
	must(t, st.UpsertRelation(ctx, &Relation{SourceNodeID: "bbb11111111", TargetNodeID: "ccc11111111", Weight: 1, Origin: OriginParsed}))

	// depth=1: only direct
	d1, _ := st.GetRelatedNodes(ctx, "aaa11111111", DirectionOut, 1, 0, 10)
	if len(d1) != 1 || d1[0].Node.ID != "bbb11111111" {
		t.Errorf("depth=1: %+v, want [bbb11111111]", d1)
	}

	// depth=2: includes 2-hop
	d2, _ := st.GetRelatedNodes(ctx, "aaa11111111", DirectionOut, 2, 0, 10)
	if len(d2) != 2 {
		t.Fatalf("depth=2: got %d results, want 2", len(d2))
	}

	ids := map[string]int{}
	for _, r := range d2 {
		ids[r.Node.ID] = r.Hop
	}
	if ids["bbb11111111"] != 1 {
		t.Errorf("hop for bbb = %d, want 1", ids["bbb11111111"])
	}
	if ids["ccc11111111"] != 2 {
		t.Errorf("hop for ccc = %d, want 2", ids["ccc11111111"])
	}
	if _, ok := ids["aaa11111111"]; ok {
		t.Errorf("anchor included in results, want excluded")
	}
}

func TestGetRelatedNodes_Incoming(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	mustNode(t, st, "aaa11111111", "")
	mustNode(t, st, "bbb11111111", "")
	mustNode(t, st, "ccc11111111", "")

	must(t, st.UpsertRelation(ctx, &Relation{SourceNodeID: "aaa11111111", TargetNodeID: "ccc11111111", Weight: 1, Origin: OriginParsed}))
	must(t, st.UpsertRelation(ctx, &Relation{SourceNodeID: "bbb11111111", TargetNodeID: "ccc11111111", Weight: 1, Origin: OriginParsed}))

	in, _ := st.GetRelatedNodes(ctx, "ccc11111111", DirectionIn, 1, 0, 10)
	if len(in) != 2 {
		t.Fatalf("incoming: got %d results, want 2 (both source nodes)", len(in))
	}

	ids := map[string]bool{}
	for _, r := range in {
		ids[r.Node.ID] = true
	}
	if !ids["aaa11111111"] || !ids["bbb11111111"] {
		t.Errorf("incoming = %+v, want both aaa and bbb", ids)
	}
	if ids["ccc11111111"] {
		t.Errorf("anchor included in incoming results, want excluded")
	}
}

func TestGetRelatedNodes_WeightMin(t *testing.T) {
	st := openTestDB(t)
	ctx := context.Background()

	mustNode(t, st, "aaa11111111", "")
	mustNode(t, st, "bbb11111111", "")
	mustNode(t, st, "ccc11111111", "")

	must(t, st.UpsertRelation(ctx, &Relation{SourceNodeID: "aaa11111111", TargetNodeID: "bbb11111111", Weight: 0.5, Origin: OriginParsed}))
	must(t, st.UpsertRelation(ctx, &Relation{SourceNodeID: "aaa11111111", TargetNodeID: "ccc11111111", Weight: 2.0, Origin: OriginParsed}))

	out, _ := st.GetRelatedNodes(ctx, "aaa11111111", DirectionOut, 1, 1.0, 10)
	if len(out) != 1 || out[0].Node.ID != "ccc11111111" {
		t.Errorf("weight_min=1.0: %+v, want only ccc11111111", out)
	}
}
