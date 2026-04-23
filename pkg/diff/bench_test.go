package diff

import (
	"fmt"
	"testing"

	"github.com/radimsem/remindb/pkg/parser"
)

func benchDiffNodes(n int) []*parser.ContextNode {
	nodes := make([]*parser.ContextNode, n)
	for i := range n {
		nodes[i] = &parser.ContextNode{
			ID:          fmt.Sprintf("node%04d", i),
			ContentHash: fmt.Sprintf("%016x", i*12345),
			Content:     fmt.Sprintf("Content for node %d with enough text.", i),
		}
	}
	return nodes
}

func benchPrevState(nodes []*parser.ContextNode) map[string]NodeState {
	m := make(map[string]NodeState, len(nodes))
	for _, n := range nodes {
		m[n.ID] = NodeState{Hash: n.ContentHash, Content: n.Content}
	}
	return m
}

func BenchmarkDiffFlat(b *testing.B) {
	for _, size := range []int{50, 200, 500} {
		nodes := benchDiffNodes(size)

		b.Run(fmt.Sprintf("no_changes/%d", size), func(b *testing.B) {
			prev := benchPrevState(nodes)
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				DiffFlat(nodes, prev)
			}
		})

		b.Run(fmt.Sprintf("all_new/%d", size), func(b *testing.B) {
			prev := make(map[string]NodeState)
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				DiffFlat(nodes, prev)
			}
		})

		b.Run(fmt.Sprintf("half_modified/%d", size), func(b *testing.B) {
			prev := benchPrevState(nodes)
			for i := 0; i < size/2; i++ {
				id := fmt.Sprintf("node%04d", i)
				prev[id] = NodeState{Hash: "changed", Content: "old content"}
			}
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				DiffFlat(nodes, prev)
			}
		})
	}
}

func BenchmarkCursorHashFlat(b *testing.B) {
	for _, size := range []int{50, 200, 500} {
		nodes := benchDiffNodes(size)
		b.Run(fmt.Sprintf("nodes/%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				CursorHashFlat(nodes)
			}
		})
	}
}
