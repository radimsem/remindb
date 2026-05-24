package treewalk

import (
	"path/filepath"
	"strings"

	"github.com/radimsem/remindb/pkg/store"
)

const MaxDepth = 128

// Make an absolute source path relative to the compile root.
func RelativeTo(source, root string) string {
	if root == "" || !filepath.IsAbs(source) {
		return source
	}

	rel, err := filepath.Rel(root, source)
	if err != nil || strings.HasPrefix(rel, "..") {
		return source
	}

	return rel
}

// Walk visits root and its descendants.
func Walk[T any](
	children map[string][]*store.Node,
	root *store.Node,
	maxDepth int,
	fn func(n, parent *store.Node, depth int, descend func() []T) T,
) T {

	return walk(children, root, nil, 0, maxDepth, fn)
}

func walk[T any](
	children map[string][]*store.Node,
	n, parent *store.Node,
	depth, maxDepth int,
	fn func(n, parent *store.Node, depth int, descend func() []T) T,
) T {
	descend := func() []T {
		if maxDepth > 0 && depth >= maxDepth {
			return nil
		}

		kids := children[n.ID]
		out := make([]T, 0, len(kids))

		for _, c := range kids {
			out = append(out, walk(children, c, n, depth+1, maxDepth, fn))
		}
		return out
	}

	return fn(n, parent, depth, descend)
}

// ClampDepth resolves a requested depth: in <= 0 yields def; a positive max
// caps the result; max <= 0 leaves the upper end unbounded.
func ClampDepth(in, def, maximum int) int {
	if in <= 0 {
		return def
	}
	if maximum > 0 && in > maximum {
		return maximum
	}

	return in
}
