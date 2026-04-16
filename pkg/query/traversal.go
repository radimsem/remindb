package query

import (
	"context"

	"github.com/radimsem/remindb/pkg/store"
)

const maxTraversalDepth = 10

func (e *Engine) collectContext(ctx context.Context, anchor *store.Node) ([]*store.Node, error) {
	ancestors, err := e.store.GetAncestors(ctx, anchor.ID)
	if err != nil {
		return nil, err
	}

	descendants, err := e.store.GetDescendants(ctx, anchor.ID, maxTraversalDepth)
	if err != nil {
		return nil, err
	}

	siblings, err := e.store.GetSiblings(ctx, anchor.ID)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	seen[anchor.ID] = true
	var out []*store.Node

	for _, batch := range [][]*store.Node{ancestors, descendants, siblings} {
		for _, n := range batch {
			if seen[n.ID] {
				continue
			}
			seen[n.ID] = true
			out = append(out, n)
		}
	}
	return out, nil
}
