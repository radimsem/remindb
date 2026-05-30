// Package relations resolves wiki-link refs into typed graph edges on top of the node tree.
package relations

import (
	"context"
	"database/sql"

	"github.com/radimsem/remindb/pkg/parser"
	"github.com/radimsem/remindb/pkg/store"
)

type Resolver struct {
	store *store.Store
}

func New(st *store.Store) *Resolver {
	return &Resolver{store: st}
}

// Resolve a single ref to a target node ID.
func (r *Resolver) Resolve(ctx context.Context, ref parser.WikilinkRef) (string, error) {
	if ref.IDHint != "" {
		n, err := r.store.GetNode(ctx, ref.IDHint)
		if err != nil || n == nil {
			return "", nil
		}

		return n.ID, nil
	}

	if ref.Label == "" {
		return "", nil
	}
	if ref.SourceQual != "" {
		return r.store.FindHeadingByLabelInFile(ctx, ref.SourceQual, ref.Label)
	}

	return r.store.FindHeadingByLabel(ctx, ref.Label)
}

// Resolve a single ref to a target node ID within tx, so resolution sees nodes
// written earlier in the same transaction.
func (r *Resolver) ResolveTx(ctx context.Context, tx *sql.Tx, ref parser.WikilinkRef) (string, error) {
	if ref.IDHint != "" {
		n, err := r.store.GetNodeTx(ctx, tx, ref.IDHint)
		if err != nil || n == nil {
			return "", nil
		}

		return n.ID, nil
	}

	if ref.Label == "" {
		return "", nil
	}
	if ref.SourceQual != "" {
		return r.store.FindHeadingByLabelInFileTx(ctx, tx, ref.SourceQual, ref.Label)
	}

	return r.store.FindHeadingByLabelTx(ctx, tx, ref.Label)
}
