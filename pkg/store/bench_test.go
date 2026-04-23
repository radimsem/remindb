package store

import (
	"context"
	"fmt"
	"testing"
)

func openBenchDB(b *testing.B) *Store {
	b.Helper()
	st, err := Open(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	if err := st.Migrate(context.Background()); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = st.Close() })
	return st
}

func seedNodes(b *testing.B, st *Store, n int) []*Node {
	b.Helper()
	ctx := context.Background()
	nodes := make([]*Node, n)

	for i := range n {
		parentID := ""
		if i > 0 {
			parentID = fmt.Sprintf("node%04d", (i-1)/3)
		}
		nodes[i] = &Node{
			ID:          fmt.Sprintf("node%04d", i),
			ParentID:    parentID,
			SourceFile:  fmt.Sprintf("bench/doc_%d.md", i/20),
			NodeType:    "heading",
			Depth:       (i % 5) + 1,
			Label:       fmt.Sprintf("Section %d: benchmark content for FTS indexing", i),
			Content:     fmt.Sprintf("Content for section %d with realistic text for full-text search benchmarking and token estimation.", i),
			Format:      "plain",
			TokenCount:  50 + (i % 150),
			ContentHash: fmt.Sprintf("%016x", i*12345),
			Temperature: float64(i%10) / 10.0,
		}
		if err := st.UpsertNode(ctx, nodes[i]); err != nil {
			b.Fatal(err)
		}
	}
	return nodes
}

func BenchmarkUpsertNode(b *testing.B) {
	st := openBenchDB(b)
	ctx := context.Background()

	n := &Node{
		ID:          "bench001",
		SourceFile:  "bench.md",
		NodeType:    "heading",
		Depth:       1,
		Label:       "Benchmark Section",
		Content:     "Benchmark content for upsert testing with realistic payload size.",
		Format:      "plain",
		TokenCount:  48,
		ContentHash: "abcdef0123456789",
	}

	ids := make([]string, 1000)
	for i := range ids {
		ids[i] = fmt.Sprintf("bench%04d", i)
	}

	b.ReportAllocs()

	for i := 0; b.Loop(); i++ {
		n.ID = ids[i%1000]
		_ = st.UpsertNode(ctx, n)
	}
}

func BenchmarkGetNode(b *testing.B) {
	st := openBenchDB(b)
	nodes := seedNodes(b, st, 200)
	ctx := context.Background()
	target := nodes[100].ID

	b.ReportAllocs()

	for b.Loop() {
		_, _ = st.GetNode(ctx, target)
	}
}

func BenchmarkSearch(b *testing.B) {
	for _, size := range []int{50, 200, 500} {
		st := openBenchDB(b)
		seedNodes(b, st, size)
		ctx := context.Background()

		b.Run(fmt.Sprintf("corpus/%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, _ = st.Search(ctx, "benchmark content section", 20)
			}
		})
	}
}

func BenchmarkSearchRanked(b *testing.B) {
	for _, size := range []int{50, 200, 500} {
		st := openBenchDB(b)
		seedNodes(b, st, size)
		ctx := context.Background()

		b.Run(fmt.Sprintf("corpus/%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, _ = st.SearchRanked(ctx, "benchmark content section", 20)
			}
		})
	}
}

func BenchmarkGetAncestors(b *testing.B) {
	st := openBenchDB(b)
	nodes := seedNodes(b, st, 200)
	ctx := context.Background()
	deep := nodes[len(nodes)-1].ID

	b.ReportAllocs()

	for b.Loop() {
		_, _ = st.GetAncestors(ctx, deep)
	}
}

func BenchmarkGetDescendants(b *testing.B) {
	st := openBenchDB(b)
	seedNodes(b, st, 200)
	ctx := context.Background()

	b.ReportAllocs()

	for b.Loop() {
		_, _ = st.GetDescendants(ctx, "node0000", 10)
	}
}

func BenchmarkBoostTemperatureBatch(b *testing.B) {
	for _, size := range []int{10, 50, 200} {
		st := openBenchDB(b)
		nodes := seedNodes(b, st, size)
		ctx := context.Background()

		ids := make([]string, len(nodes))
		for i, n := range nodes {
			ids[i] = n.ID
		}

		b.Run(fmt.Sprintf("nodes/%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				_, _ = st.db.ExecContext(ctx, `UPDATE nodes SET temperature = 0.5`)
				b.StartTimer()
				_ = st.BoostTemperatureBatch(ctx, ids, 0.15)
			}
		})
	}
}

func BenchmarkDecayTemperatures(b *testing.B) {
	for _, size := range []int{50, 200, 500} {
		st := openBenchDB(b)
		seedNodes(b, st, size)
		ctx := context.Background()

		b.Run(fmt.Sprintf("nodes/%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				_, _ = st.db.ExecContext(ctx, `UPDATE nodes SET temperature = 0.5`)
				b.StartTimer()
				_, _ = st.DecayTemperatures(ctx, 0.95)
			}
		})
	}
}

func BenchmarkBuildTree(b *testing.B) {
	for _, size := range []int{50, 200, 500} {
		st := openBenchDB(b)
		nodes := seedNodes(b, st, size)

		b.Run(fmt.Sprintf("nodes/%d", size), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				BuildTree(nodes)
			}
		})
	}
}

func BenchmarkRewriteQuery(b *testing.B) {
	cases := []struct {
		name  string
		query string
	}{
		{"single_term", "benchmark"},
		{"multi_term", "benchmark content section realistic"},
		{"with_operators", "benchmark OR content OR section"},
		{"quoted_phrase", "\"benchmark content\""},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				rewriteQuery(tc.query)
			}
		})
	}
}
