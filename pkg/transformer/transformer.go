// Package transformer enriches parsed ContextNodes with anchors, labels,
// token counts, and compressed paths.
package transformer

import (
	"context"

	"golang.org/x/sync/errgroup"

	"github.com/radimsem/remindb/pkg/parser"
)

func Transform(ctx context.Context, roots []*parser.ContextNode) error {
	flat := parser.Flatten(roots)
	if len(flat) == 0 {
		return nil
	}

	// Normalize whitespace
	for _, n := range flat {
		compress(n)
	}

	// Per-node enrichment — independent
	var g errgroup.Group
	for _, n := range flat {
		g.Go(func() error {
			setAnchor(n)
			setLabel(n)
			setTokenCount(n)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	// Wire parent IDs
	wireParents(roots, "")

	// Strip common directory prefix from source file
	compressPrefix(flat)

	return nil
}

func wireParents(nodes []*parser.ContextNode, parentID string) {
	for _, n := range nodes {
		n.ParentID = parentID
		wireParents(n.Children, n.ID)
	}
}
