package remindb_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/radimsem/remindb/pkg/compiler"
	"github.com/radimsem/remindb/pkg/query"
	"github.com/radimsem/remindb/pkg/store"
)

func openBenchStore(b *testing.B) *store.Store {
	b.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	if err := st.Migrate(context.Background()); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = st.Close() })
	return st
}

func BenchmarkCompileDir(b *testing.B) {
	dirs := []struct {
		name string
		dir  string
	}{
		{"bench", "testdata/bench"},
		{"openclaw", "testdata/openclaw"},
		{"claude_code", "testdata/claude-code"},
		{"codex", "testdata/codex"},
		{"gemini_cli", "testdata/gemini-cli"},
	}

	for _, tc := range dirs {
		dir, _ := filepath.Abs(tc.dir)
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				st := openBenchStore(b)
				b.StartTimer()
				_, _ = compiler.CompileDir(context.Background(), st, dir, "bench")
			}
		})
	}
}

func BenchmarkSearchWorkflow(b *testing.B) {
	st := openBenchStore(b)
	dir, _ := filepath.Abs("testdata/bench")
	_, _ = compiler.CompileDir(context.Background(), st, dir, "bench-init")
	eng := query.NewEngine(st)
	ctx := context.Background()

	queries := []struct {
		name  string
		query string
	}{
		{"single_term", "authentication"},
		{"multi_term", "rate limiting requests"},
		{"specific", "circuit breaker retry"},
	}

	for _, tc := range queries {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, _ = eng.Search(ctx, tc.query, 4000)
			}
		})
	}
}

func BenchmarkFetchWorkflow(b *testing.B) {
	st := openBenchStore(b)
	dir, _ := filepath.Abs("testdata/bench")
	_, _ = compiler.CompileDir(context.Background(), st, dir, "bench-init")
	eng := query.NewEngine(st)
	ctx := context.Background()

	roots, err := st.GetRootNodes(ctx)
	if err != nil || len(roots) == 0 {
		b.Fatal("no root nodes after compile")
	}

	children, _ := st.GetChildren(ctx, roots[0].ID)
	anchor := roots[0].ID
	if len(children) > 0 {
		anchor = children[0].ID
	}

	for _, budget := range []int{1000, 4000, 10000} {
		b.Run(fmt.Sprintf("budget/%d", budget), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, _ = eng.Fetch(ctx, anchor, budget, 0)
			}
		})
	}
}
