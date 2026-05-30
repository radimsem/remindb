package compiler

import (
	"context"
	"database/sql"
	"testing"

	"github.com/radimsem/remindb/internal/testutil"
	"github.com/radimsem/remindb/pkg/diff"
	"github.com/radimsem/remindb/pkg/emitter"
	"github.com/radimsem/remindb/pkg/parser"
	"github.com/radimsem/remindb/pkg/relations"
	"github.com/radimsem/remindb/pkg/store"
	"github.com/radimsem/remindb/pkg/transformer"
)

// A relations failure after the snapshot writes must roll the whole compile
// back, leaving HEAD and the snapshot log untouched. Regression for #227.
func TestCompile_RelationsFailureRollsBackSnapshot(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	a := writeFile(t, dir, "a.md", "# Topic\n\nAnchor content.\n")
	if _, err := Compile(ctx, st, WithPaths([]string{a}), WithCompileRoot(dir), WithMessage("baseline")); err != nil {
		t.Fatalf("baseline Compile: %v", err)
	}

	snapsBefore, err := st.ListSnapshots(ctx, 100)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	headBefore, err := st.GetHeadCursorHash(ctx)
	if err != nil {
		t.Fatalf("GetHeadCursorHash: %v", err)
	}

	// A brand-new file whose nodes are all adds, with a wikilink so relations has work.
	data := []byte("# Beta\n\nSee [[Topic]].\n")
	b := writeFile(t, dir, "b.md", string(data))
	roots, err := parser.ParseBytes(b, data)
	if err != nil {
		t.Fatalf("ParseBytes: %v", err)
	}
	if err := transformer.Transform(ctx, roots, dir, nil); err != nil {
		t.Fatalf("Transform: %v", err)
	}
	flat := parser.Flatten(roots)
	deltas := diff.DiffFlat(flat, map[string]diff.NodeState{})

	// Emit succeeds inside the tx; relations fails on a cancelled context, so the
	// tx must roll back the snapshot the emit just wrote.
	relCtx, cancel := context.WithCancel(ctx)
	cancel()

	txErr := st.Tx(ctx, func(tx *sql.Tx) error {
		if err := emitter.EmitTx(ctx, st, tx,
			emitter.WithRoots(roots),
			emitter.WithDeltas(deltas),
			emitter.WithCursorHash(diff.CursorHashFlat(flat)),
			emitter.WithMessage("should-roll-back"),
			emitter.WithCompileRoot(dir),
		); err != nil {
			return err
		}
		return relations.RunTx(relCtx, st, tx, flat)
	})
	if txErr == nil {
		t.Fatal("expected relations failure, got nil error")
	}

	snapsAfter, err := st.ListSnapshots(ctx, 100)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snapsAfter) != len(snapsBefore) {
		t.Errorf("snapshot leaked on rollback: before=%d after=%d", len(snapsBefore), len(snapsAfter))
	}

	headAfter, err := st.GetHeadCursorHash(ctx)
	if err != nil {
		t.Fatalf("GetHeadCursorHash: %v", err)
	}
	if headAfter != headBefore {
		t.Errorf("HEAD advanced despite rollback: before=%q after=%q", headBefore, headAfter)
	}
}

// A successful compile commits the snapshot and the resolved relations together.
func TestCompile_WikilinkRelationCommitsWithSnapshot(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	writeFile(t, dir, "a.md", "# Topic\n\nAnchor content.\n")
	writeFile(t, dir, "b.md", "# Beta\n\nSee [[Topic]].\n")

	if _, err := CompileDir(ctx, st, dir, "batch"); err != nil {
		t.Fatalf("CompileDir: %v", err)
	}

	snaps, err := st.ListSnapshots(ctx, 100)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snaps) != 1 {
		t.Errorf("snapshots = %d, want 1", len(snaps))
	}

	rels, err := st.GetAllRelations(ctx)
	if err != nil {
		t.Fatalf("GetAllRelations: %v", err)
	}
	if len(rels) == 0 {
		t.Error("expected at least one resolved relation alongside the snapshot")
	}
	for _, r := range rels {
		if r.Origin != store.OriginParsed {
			t.Errorf("relation origin = %q, want %q", r.Origin, store.OriginParsed)
		}
	}
}
