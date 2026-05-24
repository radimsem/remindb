package query

import (
	"context"

	"golang.org/x/sync/errgroup"

	"github.com/radimsem/remindb/internal/treewalk"
	"github.com/radimsem/remindb/pkg/store"
)

const (
	DefaultMaxDepth = 32
	MaxDepthCap     = treewalk.MaxDepth
)

func clampDepth(d int) int {
	return treewalk.ClampDepth(d, DefaultMaxDepth, MaxDepthCap)
}

func (e *Engine) collectContext(ctx context.Context, anchor *store.Node, depth int) ([]*store.Node, error) {
	var ancestors, descendants, siblings []*store.Node

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		var err error
		ancestors, err = e.store.GetAncestors(ctx, anchor.ID)
		return err
	})
	g.Go(func() error {
		var err error
		descendants, err = e.store.GetDescendants(ctx, anchor.ID, depth)
		return err
	})
	g.Go(func() error {
		var err error
		siblings, err = e.store.GetSiblings(ctx, anchor.ID)
		return err
	})

	if err := g.Wait(); err != nil {
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
