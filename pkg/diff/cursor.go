package diff

import "github.com/radimsem/remindb/pkg/parser"

// Build a NodeState map from enriched nodes.
func SnapshotFromNodes(roots []*parser.ContextNode) map[string]NodeState {
	flat := parser.Flatten(roots)
	m := make(map[string]NodeState, len(flat))

	for _, n := range flat {
		m[n.ID] = NodeState{Hash: n.ContentHash, Content: n.Content}
	}

	return m
}
