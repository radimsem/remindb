package relations

import (
	"context"
	"fmt"

	"github.com/radimsem/remindb/pkg/parser"
	"github.com/radimsem/remindb/pkg/store"
)

func Run(ctx context.Context, st *store.Store, sourceNodes []*parser.ContextNode) error {
	r := New(st)

	for _, n := range sourceNodes {
		if len(n.WikilinkRefs) == 0 {
			continue
		}
		if err := st.DeleteParsedPendingForSource(ctx, n.ID); err != nil {
			return fmt.Errorf("failed to clear: stale parsed pending for %s: %w", n.ID, err)
		}

		for _, ref := range n.WikilinkRefs {
			if err := r.emit(ctx, n.ID, ref, store.OriginParsed); err != nil {
				return err
			}
		}
	}

	return r.retryPending(ctx)
}

// Resolve ref and write either a relations row or a pending_relations row.
func (r *Resolver) emit(ctx context.Context, sourceID string, ref parser.WikilinkRef, origin string) error {
	targetID, err := r.Resolve(ctx, ref)
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
		if err := r.store.UpsertRelation(ctx, rel); err != nil {
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
	if err := r.store.InsertPendingRelation(ctx, pr); err != nil {
		return fmt.Errorf("failed to insert: pending relation from %s: %w", sourceID, err)
	}
	return nil
}

// Replay every pending row through the resolver.
func (r *Resolver) retryPending(ctx context.Context) error {
	pending, err := r.store.GetAllPendingRelations(ctx)
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

		targetID, err := r.Resolve(ctx, ref)
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
		if err := r.store.UpsertRelation(ctx, rel); err != nil {
			return fmt.Errorf("failed to upsert: relation from pending %d: %w", p.ID, err)
		}
		if err := r.store.DeletePendingByID(ctx, p.ID); err != nil {
			return fmt.Errorf("failed to delete: pending %d after resolution: %w", p.ID, err)
		}
	}
	return nil
}
