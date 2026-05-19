package relations

import (
	"context"
	"testing"

	"github.com/radimsem/remindb/internal/testutil"
	"github.com/radimsem/remindb/pkg/parser"
	"github.com/radimsem/remindb/pkg/store"
)

func mustHeading(t *testing.T, st *store.Store, id, sourceFile, label string, depth int) *store.Node {
	t.Helper()

	n := &store.Node{
		ID:          id,
		SourceFile:  sourceFile,
		NodeType:    "heading",
		Depth:       depth,
		Label:       label,
		Content:     label,
		Format:      "plain",
		TokenCount:  10,
		ContentHash: "hash" + id,
	}
	if err := st.UpsertNode(context.Background(), n); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}
	return n
}

func TestResolve_ByIDHint_Hit(t *testing.T) {
	st := testutil.OpenTestDB(t)
	r := New(st)
	ctx := context.Background()

	target := mustHeading(t, st, "tgt11111111", "docs/x.md", "Architecture", 1)

	got, err := r.Resolve(ctx, parser.WikilinkRef{IDHint: target.ID})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != target.ID {
		t.Errorf("got %q, want %q", got, target.ID)
	}
}

// Per spec: missing IDHint does NOT fall back to label.
func TestResolve_ByIDHint_MissNoFallback(t *testing.T) {
	st := testutil.OpenTestDB(t)
	r := New(st)
	ctx := context.Background()

	mustHeading(t, st, "real1111111", "docs/x.md", "Architecture", 1)

	got, err := r.Resolve(ctx, parser.WikilinkRef{
		IDHint: "ghost111111",  // ID does not exist
		Label:  "Architecture", // would match but should not be used
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty (no fallback when IDHint misses)", got)
	}
}

func TestResolve_ByLabel(t *testing.T) {
	st := testutil.OpenTestDB(t)
	r := New(st)
	ctx := context.Background()

	target := mustHeading(t, st, "tgt11111111", "docs/x.md", "Architecture", 1)

	got, err := r.Resolve(ctx, parser.WikilinkRef{Label: "Architecture"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != target.ID {
		t.Errorf("got %q, want %q", got, target.ID)
	}
}

func TestResolve_ByLabel_CaseInsensitive(t *testing.T) {
	st := testutil.OpenTestDB(t)
	r := New(st)
	ctx := context.Background()

	target := mustHeading(t, st, "tgt11111111", "docs/x.md", "Architecture", 1)

	got, _ := r.Resolve(ctx, parser.WikilinkRef{Label: "ARCHITECTURE"})
	if got != target.ID {
		t.Errorf("case-insensitive: got %q, want %q", got, target.ID)
	}

	got, _ = r.Resolve(ctx, parser.WikilinkRef{Label: " architecture "})
	if got != target.ID {
		t.Errorf("whitespace-trimmed: got %q, want %q", got, target.ID)
	}
}

func TestResolve_ByLabel_OnlyHeadings(t *testing.T) {
	st := testutil.OpenTestDB(t)
	r := New(st)
	ctx := context.Background()

	// Non-heading node with the matching label.
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(st.UpsertNode(ctx, &store.Node{
		ID:          "para1111111",
		SourceFile:  "x.md",
		NodeType:    "text",
		Depth:       1,
		Label:       "Architecture",
		Content:     "irrelevant",
		Format:      "plain",
		TokenCount:  5,
		ContentHash: "h1",
	}))

	got, _ := r.Resolve(ctx, parser.WikilinkRef{Label: "Architecture"})
	if got != "" {
		t.Errorf("got %q, want empty (only headings should match)", got)
	}
}

func TestResolve_BySourceAndLabel(t *testing.T) {
	st := testutil.OpenTestDB(t)
	r := New(st)
	ctx := context.Background()

	// Two headings with the same label in different files
	mustHeading(t, st, "aaa11111111", "docs/a.md", "Architecture", 1)
	wanted := mustHeading(t, st, "bbb11111111", "docs/b.md", "Architecture", 1)

	got, err := r.Resolve(ctx, parser.WikilinkRef{
		Label: "Architecture", SourceQual: "docs/b.md",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != wanted.ID {
		t.Errorf("got %q, want %q (source qualifier should pick docs/b.md)", got, wanted.ID)
	}
}

// Suffix match: user provides "docs/x.md" and source_file is the absolute "/abs/docs/x.md".
func TestResolve_BySourceQual_SuffixMatch(t *testing.T) {
	st := testutil.OpenTestDB(t)
	r := New(st)
	ctx := context.Background()

	target := mustHeading(t, st, "tgt11111111", "/abs/docs/x.md", "Architecture", 1)

	got, _ := r.Resolve(ctx, parser.WikilinkRef{
		Label: "Architecture", SourceQual: "docs/x.md",
	})
	if got != target.ID {
		t.Errorf("suffix match failed: got %q, want %q", got, target.ID)
	}
}

func TestResolve_BySourceQual_MissNoFallback(t *testing.T) {
	st := testutil.OpenTestDB(t)
	r := New(st)
	ctx := context.Background()

	mustHeading(t, st, "real1111111", "docs/a.md", "Architecture", 1)

	got, err := r.Resolve(ctx, parser.WikilinkRef{
		Label: "Architecture", SourceQual: "nowhere.md",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty (no fallback when SourceQual misses)", got)
	}
}

func TestResolve_EmptyRefReturnsEmpty(t *testing.T) {
	st := testutil.OpenTestDB(t)
	r := New(st)

	got, err := r.Resolve(context.Background(), parser.WikilinkRef{})
	if err != nil {
		t.Fatalf("Resolve(empty): %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

// Disambiguation: same label in multiple files picks the lowest source_file first.
func TestResolve_ByLabel_DisambiguationBySourceFile(t *testing.T) {
	st := testutil.OpenTestDB(t)
	r := New(st)
	ctx := context.Background()

	mustHeading(t, st, "zzz11111111", "z.md", "Foo", 1)
	mustHeading(t, st, "aaa11111111", "a.md", "Foo", 1)
	mustHeading(t, st, "mmm11111111", "m.md", "Foo", 1)

	got, _ := r.Resolve(ctx, parser.WikilinkRef{Label: "Foo"})
	if got != "aaa11111111" {
		t.Errorf("got %q, want aaa11111111 (a.md sorts first)", got)
	}
}

// End-to-end: Run() phase 1 emits a relation row for a resolved ref.
func TestRun_Phase1_Hit(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()

	src := mustHeading(t, st, "src11111111", "x.md", "Source", 1)
	target := mustHeading(t, st, "tgt11111111", "x.md", "Architecture", 2)

	// Inject WikilinkRefs onto the source node (simulating the parser output).
	sourceNode := &parser.ContextNode{
		ID:           src.ID,
		WikilinkRefs: []parser.WikilinkRef{{Label: "Architecture", Weight: 2.5}},
	}

	if err := Run(ctx, st, []*parser.ContextNode{sourceNode}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	related, err := st.GetRelatedNodes(ctx, src.ID, store.WithDirection(store.DirectionOut), store.WithMaxDepth(1), store.WithLimit(10))
	if err != nil {
		t.Fatalf("GetRelatedNodes: %v", err)
	}
	if len(related) != 1 || related[0].Node.ID != target.ID {
		t.Fatalf("related = %+v, want [%s]", related, target.ID)
	}
	if related[0].Weight != 2.5 {
		t.Errorf("weight = %f, want 2.5", related[0].Weight)
	}
}

// Phase 1 miss → pending row gets inserted with origin=parsed.
func TestRun_Phase1_Miss_GoesPending(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()

	src := mustHeading(t, st, "src11111111", "x.md", "Source", 1)

	sourceNode := &parser.ContextNode{
		ID:           src.ID,
		WikilinkRefs: []parser.WikilinkRef{{Label: "NonExistent", Weight: 1.0}},
	}

	if err := Run(ctx, st, []*parser.ContextNode{sourceNode}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	pending, _ := st.GetAllPendingRelations(ctx)
	if len(pending) != 1 {
		t.Fatalf("len(pending) = %d, want 1", len(pending))
	}
	if pending[0].TargetLabel != "NonExistent" || pending[0].Origin != store.OriginParsed {
		t.Errorf("pending[0] = %+v", pending[0])
	}
}

// Recompiling the same source clears stale parsed pending entries first.
func TestRun_Phase1_ClearsStaleParsedPending(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()

	src := mustHeading(t, st, "src11111111", "x.md", "Source", 1)

	first := &parser.ContextNode{
		ID: src.ID,
		WikilinkRefs: []parser.WikilinkRef{
			{Label: "X", Weight: 1.0},
			{Label: "Y", Weight: 1.0},
		},
	}
	if err := Run(ctx, st, []*parser.ContextNode{first}); err != nil {
		t.Fatalf("Run(first): %v", err)
	}
	if pending, _ := st.GetAllPendingRelations(ctx); len(pending) != 2 {
		t.Fatalf("after first: pending = %d, want 2", len(pending))
	}

	// Recompile the source — now only refers to a single target.
	second := &parser.ContextNode{
		ID:           src.ID,
		WikilinkRefs: []parser.WikilinkRef{{Label: "Z", Weight: 1.0}},
	}
	if err := Run(ctx, st, []*parser.ContextNode{second}); err != nil {
		t.Fatalf("Run(second): %v", err)
	}

	pending, _ := st.GetAllPendingRelations(ctx)
	if len(pending) != 1 {
		t.Fatalf("after second: pending = %d, want 1 (stale parsed should be cleared)", len(pending))
	}
	if pending[0].TargetLabel != "Z" {
		t.Errorf("remaining pending label = %q, want Z", pending[0].TargetLabel)
	}
}

// Phase 2: a previously-pending row resolves on the next compile.
func TestRun_Phase2_RetriesPendingAndMoves(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()

	src := mustHeading(t, st, "src11111111", "x.md", "Source", 1)

	// First compile: target doesn't exist yet → goes pending.
	first := &parser.ContextNode{
		ID:           src.ID,
		WikilinkRefs: []parser.WikilinkRef{{Label: "Later", Weight: 3.0}},
	}
	if err := Run(ctx, st, []*parser.ContextNode{first}); err != nil {
		t.Fatalf("Run(first): %v", err)
	}

	// Now add the target node.
	target := mustHeading(t, st, "later111111", "y.md", "Later", 1)

	// Second compile: source node unchanged (no refs in input, but phase 2 retries pending).
	if err := Run(ctx, st, nil); err != nil {
		t.Fatalf("Run(second): %v", err)
	}

	// Pending should be empty, relation should exist.
	pending, _ := st.GetAllPendingRelations(ctx)
	if len(pending) != 0 {
		t.Errorf("pending should be empty, got %+v", pending)
	}

	related, _ := st.GetRelatedNodes(ctx, src.ID, store.WithDirection(store.DirectionOut), store.WithMaxDepth(1), store.WithLimit(10))
	if len(related) != 1 || related[0].Node.ID != target.ID {
		t.Fatalf("related = %+v, want [%s]", related, target.ID)
	}
	if related[0].Weight != 3.0 {
		t.Errorf("weight preserved across phases: got %f, want 3.0", related[0].Weight)
	}
}

// Manual pending rows survive phase 1 (which only clears parsed pending).
func TestRun_Phase1_DoesNotTouchManualPending(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()

	src := mustHeading(t, st, "src11111111", "x.md", "Source", 1)

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(st.InsertPendingRelation(ctx, &store.PendingRelation{
		SourceNodeID: src.ID, TargetLabel: "manual-target", Weight: 1.0, Origin: store.OriginManual,
	}))

	// Recompile with no parsed refs.
	sourceNode := &parser.ContextNode{ID: src.ID}
	must(Run(ctx, st, []*parser.ContextNode{sourceNode}))

	pending, _ := st.GetAllPendingRelations(ctx)
	if len(pending) != 1 || pending[0].Origin != store.OriginManual {
		t.Errorf("manual pending lost: %+v", pending)
	}
}
