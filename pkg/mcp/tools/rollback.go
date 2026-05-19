package tools

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/pkg/diff"
	"github.com/radimsem/remindb/pkg/store"
)

type RollbackInput struct {
	SnapshotID int64 `json:"snapshot_id" jsonschema:"Target snapshot ID to restore"`
	DropAfter  bool  `json:"drop_after,omitempty" jsonschema:"If true, hard-delete snapshots between target and HEAD; if false (default), keep them as branched history reachable via MemoryHistory"`
}

func (d *Deps) HandleRollback(ctx context.Context, _ *gomcp.CallToolRequest, input RollbackInput) (_ *gomcp.CallToolResult, _ any, err error) {
	defer d.logCall(ctx, "MemoryRollback", &err, time.Now(), "snapshot_id", input.SnapshotID, "drop_after", input.DropAfter)

	if input.SnapshotID <= 0 {
		return nil, nil, fmt.Errorf("invalid snapshot_id %d: expected positive integer", input.SnapshotID)
	}
	targetID := input.SnapshotID

	d.Store.OpMu.Lock()
	defer d.Store.OpMu.Unlock()

	prevHeadID, err := d.Store.GetHeadSnapshotID(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch: head snapshot id: %w", err)
	}

	if prevHeadID == targetID {
		return textResult(fmt.Sprintf("already at snapshot %d; nothing to do", targetID)), nil, nil
	}
	if targetID > prevHeadID {
		return nil, nil, fmt.Errorf("snapshot %d is ahead of HEAD %d", targetID, prevHeadID)
	}

	restore, err := d.Store.RestoreToSnapshot(ctx, targetID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to restore: %w", err)
	}

	current, err := loadCurrentNodeMap(ctx, d.Store)
	if err != nil {
		return nil, nil, err
	}

	deltas := computeRollbackDeltas(current, restore.Nodes)
	if len(deltas) == 0 {
		return textResult(formatNoChange(targetID, restore.Skipped)), nil, nil
	}

	cursorHash := diff.CursorHashForRollback(prevHeadID, targetID, deltas)
	parentID := prevHeadID
	if input.DropAfter {
		parentID = targetID
	}

	var newSnapID int64
	var pruned int

	err = d.Store.Tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `PRAGMA defer_foreign_keys = 1`); err != nil {
			return fmt.Errorf("failed to defer: foreign keys: %w", err)
		}

		preState, err := capturePreStateForRollback(ctx, d.Store, tx, deltas)
		if err != nil {
			return err
		}
		if err := applyRollbackMutations(ctx, d.Store, tx, deltas, restore.Nodes); err != nil {
			return err
		}

		msg := fmt.Sprintf("rollback to %d", targetID)
		newSnapID, err = d.Store.CreateSnapshotTx(ctx, tx,
			store.WithCursorHash(cursorHash),
			store.WithMessage(msg),
			store.WithParent(parentID),
		)
		if err != nil {
			return fmt.Errorf("failed to create: snapshot: %w", err)
		}

		if err := insertRollbackDiffs(ctx, d.Store, tx, newSnapID, deltas, preState); err != nil {
			return err
		}
		if err := d.Store.AdvanceCursorTx(ctx, tx, newSnapID); err != nil {
			return fmt.Errorf("failed to advance: cursor: %w", err)
		}

		if input.DropAfter {
			n, err := d.Store.PruneSnapshotsAfterTx(ctx, tx, targetID, newSnapID)
			if err != nil {
				return err
			}

			pruned = n
		}
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to rollback: %w", err)
	}

	d.touchSnapshot()

	return textResult(formatRollbackResult(targetID, newSnapID, len(deltas), pruned, input.DropAfter, restore.Skipped)), nil, nil
}

func loadCurrentNodeMap(ctx context.Context, st *store.Store) (map[string]*store.Node, error) {
	nodes, err := st.GetAllNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load: head nodes: %w", err)
	}

	out := make(map[string]*store.Node, len(nodes))
	for _, n := range nodes {
		out[n.ID] = n
	}
	return out, nil
}

func computeRollbackDeltas(current, target map[string]*store.Node) []diff.Delta {
	deltas := make([]diff.Delta, 0, len(current)+len(target))

	for id, c := range current {
		if _, ok := target[id]; !ok {
			deltas = append(deltas, diff.Delta{
				NodeID:     id,
				Op:         diff.OpRem,
				OldHash:    c.ContentHash,
				OldContent: c.Content,
			})
		}
	}

	for id, t := range target {
		c, ok := current[id]
		if !ok {
			deltas = append(deltas, diff.Delta{
				NodeID:     id,
				Op:         diff.OpAdd,
				NewHash:    t.ContentHash,
				NewContent: t.Content,
			})
			continue
		}

		if !nodeEqual(c, t) {
			deltas = append(deltas, diff.Delta{
				NodeID:     id,
				Op:         diff.OpMod,
				OldHash:    c.ContentHash,
				NewHash:    t.ContentHash,
				OldContent: c.Content,
				NewContent: t.Content,
			})
		}
	}

	sort.Slice(deltas, func(i, j int) bool { return deltas[i].NodeID < deltas[j].NodeID })
	return deltas
}

func nodeEqual(a, b *store.Node) bool {
	return a.ContentHash == b.ContentHash &&
		a.ParentID == b.ParentID &&
		a.SourceFile == b.SourceFile &&
		a.NodeType == b.NodeType &&
		a.Depth == b.Depth &&
		a.Label == b.Label &&
		a.Format == b.Format &&
		a.TokenCount == b.TokenCount
}

func capturePreStateForRollback(ctx context.Context, st *store.Store, tx *sql.Tx, deltas []diff.Delta) (map[string]*store.Node, error) {
	out := make(map[string]*store.Node, len(deltas))

	for i := range deltas {
		d := &deltas[i]
		if d.Op == diff.OpAdd {
			continue
		}

		n, err := st.GetNodeTx(ctx, tx, d.NodeID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}

			return nil, fmt.Errorf("failed to load: pre-state %s: %w", d.NodeID, err)
		}
		out[d.NodeID] = n
	}
	return out, nil
}

func applyRollbackMutations(ctx context.Context, st *store.Store, tx *sql.Tx, deltas []diff.Delta, target map[string]*store.Node) error {
	for i := range deltas {
		d := &deltas[i]

		switch d.Op {
		case diff.OpRem:
			if err := st.DeleteNodeTx(ctx, tx, d.NodeID); err != nil {
				return fmt.Errorf("failed to delete: node %s: %w", d.NodeID, err)
			}

		case diff.OpAdd, diff.OpMod:
			n, ok := target[d.NodeID]
			if !ok {
				return fmt.Errorf("failed to apply: rollback target missing node %s", d.NodeID)
			}
			if err := st.UpsertNodeTx(ctx, tx, n); err != nil {
				return fmt.Errorf("failed to upsert: node %s: %w", d.NodeID, err)
			}
		}
	}
	return nil
}

func insertRollbackDiffs(ctx context.Context, st *store.Store, tx *sql.Tx, snapID int64, deltas []diff.Delta, preState map[string]*store.Node) error {
	for i := range deltas {
		d := &deltas[i]
		rec := &store.DiffRecord{
			SnapshotID: snapID,
			NodeID:     d.NodeID,
			Op:         string(d.Op),
			OldHash:    d.OldHash,
			NewHash:    d.NewHash,
			OldContent: d.OldContent,
			NewContent: d.NewContent,
		}

		if pre, ok := preState[d.NodeID]; ok {
			rec.SetOldMetadata(pre)
		}

		if err := st.InsertDiffTx(ctx, tx, rec); err != nil {
			return fmt.Errorf("failed to insert: diff %s: %w", d.NodeID, err)
		}
	}
	return nil
}

func formatRollbackResult(targetID, newSnapID int64, affected, pruned int, dropAfter bool, skipped []store.SkippedRestore) string {
	var b strings.Builder
	fmt.Fprintf(&b, "rolled back to snapshot %d (new snapshot %d, %d nodes affected)", targetID, newSnapID, affected)

	if dropAfter {
		fmt.Fprintf(&b, "; pruned %d intermediate snapshot(s)", pruned)
	}
	appendSkippedWarnings(&b, skipped)

	return b.String()
}

func formatNoChange(targetID int64, skipped []store.SkippedRestore) string {
	var b strings.Builder
	fmt.Fprintf(&b, "no rollback applied; computed state matches HEAD for snapshot %d", targetID)
	appendSkippedWarnings(&b, skipped)
	return b.String()
}

func appendSkippedWarnings(b *strings.Builder, skipped []store.SkippedRestore) {
	if len(skipped) == 0 {
		return
	}

	fmt.Fprintf(b, "\nwarning: %d node(s) could not be fully restored:", len(skipped))
	for _, s := range skipped {
		fmt.Fprintf(b, "\n  - %s: %s", s.NodeID, s.Reason)
	}
}
