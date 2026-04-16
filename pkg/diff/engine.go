package diff

import (
	"sort"

	"github.com/radimsem/remindb/pkg/parser"
)

// Compare enriched nodes against the previous snapshot and returns one.
func Diff(roots []*parser.ContextNode, prev map[string]NodeState) []Delta {
	flat := flatten(roots)
	seen := make(map[string]bool, len(flat))
	deltas := make([]Delta, 0, len(flat))

	for _, n := range flat {
		seen[n.ID] = true
		old, exists := prev[n.ID]

		if !exists {
			deltas = append(deltas, Delta{
				NodeID:     n.ID,
				Op:         OpAdd,
				NewHash:    n.ContentHash,
				NewContent: n.Content,
			})
			continue
		}

		if old.Hash != n.ContentHash {
			deltas = append(deltas, Delta{
				NodeID:     n.ID,
				Op:         OpMod,
				OldHash:    old.Hash,
				NewHash:    n.ContentHash,
				OldContent: old.Content,
				NewContent: n.Content,
			})
		}
	}

	// Removals in sorted order for deterministic output.
	removed := make([]string, 0)
	for id := range prev {
		if !seen[id] {
			removed = append(removed, id)
		}
	}
	sort.Strings(removed)

	for _, id := range removed {
		old := prev[id]
		deltas = append(deltas, Delta{
			NodeID:     id,
			Op:         OpRem,
			OldHash:    old.Hash,
			OldContent: old.Content,
		})
	}

	return deltas
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
