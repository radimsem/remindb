// Package emitter applies diff deltas to the store in a single transaction.
package emitter

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/radimsem/remindb/pkg/diff"
	"github.com/radimsem/remindb/pkg/parser"
	"github.com/radimsem/remindb/pkg/store"
)

type Option func(*options)

type options struct {
	roots       []*parser.ContextNode
	deltas      []diff.Delta
	cursorHash  string
	message     string
	compileRoot string
}

func WithRoots(r []*parser.ContextNode) Option {
	return func(o *options) { o.roots = r }
}

func WithDeltas(d []diff.Delta) Option {
	return func(o *options) { o.deltas = d }
}

func WithCursorHash(h string) Option {
	return func(o *options) { o.cursorHash = h }
}

func WithMessage(m string) Option {
	return func(o *options) { o.message = m }
}

func WithCompileRoot(r string) Option {
	return func(o *options) { o.compileRoot = r }
}

func Emit(ctx context.Context, st *store.Store, opts ...Option) error {
	var o options
	for _, opt := range opts {
		opt(&o)
	}

	if len(o.deltas) == 0 {
		return nil
	}

	nodeMap := buildNodeMap(o.roots)

	return st.Tx(ctx, func(tx *sql.Tx) error {
		for i := range o.deltas {
			d := &o.deltas[i]

			switch d.Op {
			case diff.OpAdd, diff.OpMod:
				cn, ok := nodeMap[d.NodeID]
				if !ok {
					return fmt.Errorf("emitter: node %s not found in tree", d.NodeID)
				}
				n := nodeFromContext(cn)
				if err := st.UpsertNodeTx(ctx, tx, n); err != nil {
					return fmt.Errorf("failed to upsert: node %s: %w", d.NodeID, err)
				}

			case diff.OpRem:
				if err := st.DeleteNodeTx(ctx, tx, d.NodeID); err != nil {
					return fmt.Errorf("failed to delete: node %s: %w", d.NodeID, err)
				}
			}
		}

		snapID, err := st.CreateSnapshotTx(ctx, tx, o.cursorHash, o.message, o.compileRoot)
		if err != nil {
			return fmt.Errorf("failed to create: snapshot: %w", err)
		}

		for i := range o.deltas {
			d := &o.deltas[i]
			rec := &store.DiffRecord{
				SnapshotID: snapID,
				NodeID:     d.NodeID,
				Op:         string(d.Op),
				OldHash:    d.OldHash,
				NewHash:    d.NewHash,
				OldContent: d.OldContent,
				NewContent: d.NewContent,
			}
			if err := st.InsertDiffTx(ctx, tx, rec); err != nil {
				return fmt.Errorf("failed to insert: diff %s: %w", d.NodeID, err)
			}
		}

		if err := st.AdvanceCursorTx(ctx, tx, snapID); err != nil {
			return fmt.Errorf("failed to advance: cursor: %w", err)
		}

		return nil
	})
}

func buildNodeMap(roots []*parser.ContextNode) map[string]*parser.ContextNode {
	return parser.FlattenMap(roots)
}

func nodeFromContext(cn *parser.ContextNode) *store.Node {
	return &store.Node{
		ID:          cn.ID,
		ParentID:    cn.ParentID,
		SourceFile:  cn.SourceFile,
		NodeType:    string(cn.NodeType),
		Depth:       cn.Depth,
		Label:       cn.Label,
		Content:     cn.Content,
		Format:      cn.Format,
		TokenCount:  cn.TokenCount,
		ContentHash: cn.ContentHash,
		SeedTemp:    cn.Temperature,
	}
}
