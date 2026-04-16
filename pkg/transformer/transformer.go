// Package transformer enriches parsed ContextNodes with anchors, labels,
// token counts, and compressed paths.
package transformer

import (
	"context"
	"runtime"

	"golang.org/x/sync/errgroup"

	"github.com/radimsem/remindb/pkg/parser"
)

// Transform enriches nodes in place. Compress runs first (content-dependent
// steps need final Content), then anchor + label + tokenest fan out per node,
// then parent IDs are wired and path prefixes stripped.
func Transform(ctx context.Context, roots []*parser.ContextNode) error {
	flat := flatten(roots)
	if len(flat) == 0 {
		return nil
	}

	// Stage 1: normalize whitespace (must complete before content-dependent steps)
	for _, n := range flat {
		compress(n)
	}

	// Stage 2: per-node enrichment — independent, fan out via errgroup
	var g errgroup.Group
	g.SetLimit(runtime.GOMAXPROCS(0))

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

	// Stage 3: wire parent→child IDs (needs anchors from stage 2)
	wireParents(roots, "")

	// Stage 4: strip common directory prefix from SourceFile
	compressPrefix(flat)

	return nil
}

func flatten(roots []*parser.ContextNode) []*parser.ContextNode {
	return collectNodes(nil, roots)
}

func collectNodes(out []*parser.ContextNode, nodes []*parser.ContextNode) []*parser.ContextNode {
	for _, n := range nodes {
		out = append(out, n)
		out = collectNodes(out, n.Children)
	}
	return out
}

func wireParents(nodes []*parser.ContextNode, parentID string) {
	for _, n := range nodes {
		n.ParentID = parentID
		wireParents(n.Children, n.ID)
	}
}
