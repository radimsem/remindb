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

	// Strip common directory prefix from source file.
	compressPrefix(flat)

	// Per-node enrichment — independent, content-only
	var g errgroup.Group
	for _, n := range flat {
		g.Go(func() error {
			setContentHash(n)
			setLabel(n)
			setTokenCount(n)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	// Wire IDs and parent IDs in a parent-first walk.
	wireIdentity(roots, "")

	return nil
}

func wireIdentity(nodes []*parser.ContextNode, parentID string) {
	for _, n := range nodes {
		setIdentity(n, parentID)
		wireIdentity(n.Children, n.ID)
	}
}
