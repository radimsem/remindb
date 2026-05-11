// Package transformer enriches parsed ContextNodes with anchors, labels,
// token counts, and compressed paths.
package transformer

import (
	"context"

	"golang.org/x/sync/errgroup"

	"github.com/radimsem/remindb/pkg/parser"
)

func Transform(ctx context.Context, roots []*parser.ContextNode, compileRoot string) error {
	flat := parser.Flatten(roots)
	if len(flat) == 0 {
		return nil
	}

	// Normalize whitespace
	for _, n := range flat {
		compress(n)
	}

	compressPrefix(flat, compileRoot)

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
	// Top-level roots may come from multiple files when compiling a whole dir,
	// so index siblings per-file; otherwise a root's ID would depend on walk order.
	if parentID == "" {
		perFile := make(map[string]int, len(nodes))
		for _, n := range nodes {
			i := perFile[n.SourceFile]
			perFile[n.SourceFile] = i + 1

			setIdentity(n, "", i)
			wireIdentity(n.Children, n.ID)
		}
		return
	}

	for i, n := range nodes {
		setIdentity(n, parentID, i)
		wireIdentity(n.Children, n.ID)
	}
}
