package query

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/radimsem/remindb/pkg/parser"
	"github.com/radimsem/remindb/pkg/store"
)

func benchScoredNodes(n int) []ScoredNode {
	now := time.Now()
	out := make([]ScoredNode, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, ScoredNode{
			Node: &store.Node{
				ID:           fmt.Sprintf("%08d", i),
				NodeType:     string(parser.NodeHeading),
				Label:        fmt.Sprintf("Section %d: benchmark content", i),
				Content:      fmt.Sprintf("Content for section %d with enough text to be realistic.", i),
				TokenCount:   50 + (i % 200),
				Temperature:  float64(i%10) / 10.0,
				LastAccessed: sql.NullInt64{Int64: now.Add(-time.Duration(i) * time.Hour).Unix(), Valid: true},
			},
			Score: float64(n-i) * 0.1,
		})
	}
	return out
}

func benchStoreNodes(n int) []*store.Node {
	now := time.Now()
	nodes := make([]*store.Node, 0, n)
	for i := 0; i < n; i++ {
		nodes = append(nodes, &store.Node{
			ID:           fmt.Sprintf("%08d", i),
			TokenCount:   50 + (i % 200),
			Temperature:  float64(i%10) / 10.0,
			LastAccessed: sql.NullInt64{Int64: now.Add(-time.Duration(i) * time.Hour).Unix(), Valid: true},
		})
	}
	return nodes
}

func BenchmarkScoreNode(b *testing.B) {
	now := time.Now()
	n := &store.Node{
		Temperature:  0.8,
		LastAccessed: sql.NullInt64{Int64: now.Add(-2 * time.Hour).Unix(), Valid: true},
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		scoreNode(n, 2.5, now)
	}
}

func BenchmarkRankNodes(b *testing.B) {
	for _, size := range []int{10, 100, 500} {
		nodes := benchStoreNodes(size)
		now := time.Now()
		b.Run(fmt.Sprintf("nodes/%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				rankNodes(nodes, now)
			}
		})
	}
}

func BenchmarkFillBudget(b *testing.B) {
	for _, size := range []int{10, 100, 500} {
		scored := benchScoredNodes(size)
		budget := size * 75
		b.Run(fmt.Sprintf("nodes/%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				fillBudget(scored, budget)
			}
		})
	}
}

func BenchmarkFormat(b *testing.B) {
	scored := benchScoredNodes(50)
	result := &Result{Nodes: scored, TokensUsed: 5000}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Format(result)
	}
}
