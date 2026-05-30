package relations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/radimsem/remindb/pkg/parser"
	"github.com/radimsem/remindb/pkg/store"
)

// Run resolves wikilink refs into graph edges in its own transaction.
func Run(ctx context.Context, st *store.Store, sourceNodes []*parser.ContextNode) error {
	return st.Tx(ctx, func(tx *sql.Tx) error {
		return RunTx(ctx, st, tx, sourceNodes)
	})
}

// RunTx resolves wikilink refs within tx, so resolution sees nodes written
// earlier in the same transaction and a failure rolls those writes back too.
func RunTx(ctx context.Context, st *store.Store, tx *sql.Tx, sourceNodes []*parser.ContextNode) error {
	r := New(st)

	for _, n := range sourceNodes {
		if len(n.WikilinkRefs) == 0 {
			continue
		}
		if err := st.DeleteParsedPendingForSourceTx(ctx, tx, n.ID); err != nil {
			return fmt.Errorf("failed to clear: stale parsed pending for %s: %w", n.ID, err)
		}

		for _, ref := range n.WikilinkRefs {
			if err := r.emit(ctx, tx, n.ID, ref, store.OriginParsed); err != nil {
				return err
			}
		}
	}

	return r.retryPending(ctx, tx)
}

// Resolve ref and write either a relations row or a pending_relations row within tx.
func (r *Resolver) emit(ctx context.Context, tx *sql.Tx, sourceID string, ref parser.WikilinkRef, origin string) error {
	targetID, err := r.ResolveTx(ctx, tx, ref)
	if err != nil {
		return fmt.Errorf("failed to resolve: %w", err)
	}

	if targetID != "" {
		rel := &store.Relation{
			SourceNodeID: sourceID,
			TargetNodeID: targetID,
			Weight:       ref.Weight,
			Origin:       origin,
		}
		if err := r.store.UpsertRelationTx(ctx, tx, rel); err != nil {
			return fmt.Errorf("failed to upsert: relation %s -> %s: %w", sourceID, targetID, err)
		}
		return nil
	}

	pr := &store.PendingRelation{
		SourceNodeID: sourceID,
		TargetLabel:  ref.Label,
		TargetSource: ref.SourceQual,
		TargetIDHint: ref.IDHint,
		Weight:       ref.Weight,
		Origin:       origin,
	}
	if err := r.store.InsertPendingRelationTx(ctx, tx, pr); err != nil {
		return fmt.Errorf("failed to insert: pending relation from %s: %w", sourceID, err)
	}
	return nil
}

// Replay every pending row through the resolver within tx.
func (r *Resolver) retryPending(ctx context.Context, tx *sql.Tx) error {
	pending, err := r.store.GetAllPendingRelationsTx(ctx, tx)
	if err != nil {
		return fmt.Errorf("failed to read: pending relations: %w", err)
	}

	for _, p := range pending {
		ref := parser.WikilinkRef{
			Label:      p.TargetLabel,
			SourceQual: p.TargetSource,
			IDHint:     p.TargetIDHint,
			Weight:     p.Weight,
		}

		targetID, err := r.ResolveTx(ctx, tx, ref)
		if err != nil {
			return fmt.Errorf("failed to resolve: pending %d: %w", p.ID, err)
		}
		if targetID == "" {
			continue
		}

		rel := &store.Relation{
			SourceNodeID: p.SourceNodeID,
			TargetNodeID: targetID,
			Weight:       p.Weight,
			Origin:       p.Origin,
		}
		if err := r.store.UpsertRelationTx(ctx, tx, rel); err != nil {
			return fmt.Errorf("failed to upsert: relation from pending %d: %w", p.ID, err)
		}
		if err := r.store.DeletePendingByIDTx(ctx, tx, p.ID); err != nil {
			return fmt.Errorf("failed to delete: pending %d after resolution: %w", p.ID, err)
		}
	}
	return nil
}
