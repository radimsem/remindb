package remindb_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/radimsem/remindb/internal/fileext"
	"github.com/radimsem/remindb/internal/tokens"
	"github.com/radimsem/remindb/pkg/compiler"
	"github.com/radimsem/remindb/pkg/parser"
	"github.com/radimsem/remindb/pkg/query"
	"github.com/radimsem/remindb/pkg/store"
)

// Naive cost: sum of all supported file tokens in dir.
func countDirTokens(b *testing.B, dir string) int {
	b.Helper()
	total := 0

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !fileext.Supported(path) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		total += tokens.Estimate(string(data))
		return nil
	})
	if err != nil {
		b.Fatalf("failed to walk %s: %v", dir, err)
	}

	return total
}

// renderTree produces the same output as the memory_tree MCP tool.
func renderTree(ctx context.Context, st *store.Store) string {
	all, err := st.GetAllNodes(ctx)
	if err != nil || len(all) == 0 {
		return "empty tree"
	}

	roots, children := store.BuildTree(all)

	var b strings.Builder
	for _, root := range roots {
		renderTreeNode(&b, children, root, 0, 5)
	}
	return b.String()
}

func renderTreeNode(b *strings.Builder, children map[string][]*store.Node, n *store.Node, depth, maxDepth int) {
	indent := strings.Repeat("  ", depth)
	fmt.Fprintf(b, "%s[%s] %s (%s)\n", indent, n.NodeType, n.Label, n.ID)

	if depth >= maxDepth {
		return
	}
	for _, child := range children[n.ID] {
		renderTreeNode(b, children, child, depth+1, maxDepth)
	}
}

// renderDelta produces the same output as the memory_delta MCP tool.
func renderDelta(diffs []*store.DiffRecord) string {
	var b strings.Builder
	for _, dr := range diffs {
		fmt.Fprintf(&b, "[%s] %s (snapshot %d)\n", dr.Op, dr.NodeID, dr.SnapshotID)
	}
	return b.String()
}

// Pick the depth>1 node with the most available context for realistic fetch budgets.
func findDeepNode(ctx context.Context, st *store.Store) *store.Node {
	all, _ := st.GetAllNodes(ctx)
	var best *store.Node
	bestCtx := 0

	for _, n := range all {
		if n.Depth < 2 {
			continue
		}
		desc, _ := st.GetDescendants(ctx, n.ID, 10)
		sib, _ := st.GetSiblings(ctx, n.ID)
		anc, _ := st.GetAncestors(ctx, n.ID)

		total := 0
		for _, d := range desc {
			total += d.TokenCount
		}
		for _, s := range sib {
			total += s.TokenCount
		}
		for _, a := range anc {
			total += a.TokenCount
		}

		if total > bestCtx {
			bestCtx = total
			best = n
		}
	}
	return best
}

// Create snapshot 2 with 2 realistic changes after the compile (snapshot 1).
func insertSmallDelta(b *testing.B, st *store.Store, ctx context.Context) int64 {
	b.Helper()
	var snapID int64

	err := st.Tx(ctx, func(tx *sql.Tx) error {
		_ = st.UpsertNodeTx(ctx, tx, &store.Node{
			ID:          "benchnew1",
			SourceFile:  "session.md",
			NodeType:    string(parser.NodeText),
			Depth:       1,
			Label:       "Session note: slog decision",
			Content:     "Agent decided to use structured logging with slog instead of log package.",
			Format:      parser.FormatPlain,
			TokenCount:  53,
			ContentHash: "a1b2c3d4e5f60001",
		})
		_ = st.UpsertNodeTx(ctx, tx, &store.Node{
			ID:          "benchnew2",
			SourceFile:  "session.md",
			NodeType:    string(parser.NodeText),
			Depth:       1,
			Label:       "Session note: pool size",
			Content:     "Increased connection pool from 25 to 50 after load test showed saturation at 30 concurrent.",
			Format:      parser.FormatPlain,
			TokenCount:  68,
			ContentHash: "a1b2c3d4e5f60002",
		})

		var err error
		snapID, err = st.CreateSnapshotTx(ctx, tx, "bench-delta", "bench-write")
		if err != nil {
			return err
		}

		for _, d := range []store.DiffRecord{
			{SnapshotID: snapID, NodeID: "benchnew1", Op: "add", NewHash: "a1b2c3d4e5f60001", NewContent: "Agent decided to use structured logging with slog instead of log package."},
			{SnapshotID: snapID, NodeID: "benchnew2", Op: "add", NewHash: "a1b2c3d4e5f60002", NewContent: "Increased connection pool from 25 to 50 after load test showed saturation at 30 concurrent."},
		} {
			if err := st.InsertDiffTx(ctx, tx, &d); err != nil {
				return err
			}
		}

		return st.AdvanceCursorTx(ctx, tx, snapID)
	})
	if err != nil {
		b.Fatal(err)
	}
	return snapID
}

// Naive: read all files. remindb: compact tree of labels + IDs.
func BenchmarkTokens_TreeVsRawFiles(b *testing.B) {
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
			st := openBenchStore(b)
			ctx := context.Background()
			_, _ = compiler.CompileDir(ctx, st, dir, "init")

			rawTok := countDirTokens(b, dir)
			treeOut := renderTree(ctx, st)
			treeTok := tokens.Estimate(treeOut)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				renderTree(ctx, st)
			}
			b.StopTimer()

			b.ReportMetric(float64(rawTok), "raw-tok")
			b.ReportMetric(float64(treeTok), "tree-tok")
			b.ReportMetric(100*(1-float64(treeTok)/float64(rawTok)), "pct-saved")
		})
	}
}

// Naive: all file tokens in context. remindb: budget-bounded FTS results.
func BenchmarkTokens_SearchVsReadAll(b *testing.B) {
	st := openBenchStore(b)
	ctx := context.Background()
	dir, _ := filepath.Abs("testdata/bench")
	_, _ = compiler.CompileDir(ctx, st, dir, "init")
	eng := query.NewEngine(st)

	rawTok := countDirTokens(b, dir)

	queries := []struct {
		name   string
		q      string
		budget int
	}{
		{"targeted/1k", "authentication JWT validation", 1000},
		{"broad/2k", "rate limiting configuration Redis", 2000},
		{"narrow/500", "circuit breaker retry", 500},
	}

	for _, tc := range queries {
		b.Run(tc.name, func(b *testing.B) {
			result, err := eng.Search(ctx, tc.q, tc.budget)
			if err != nil {
				b.Fatal(err)
			}
			searchTok := tokens.Estimate(query.Format(result))

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = eng.Search(ctx, tc.q, tc.budget)
			}
			b.StopTimer()

			b.ReportMetric(float64(rawTok), "raw-tok")
			b.ReportMetric(float64(searchTok), "search-tok")
			b.ReportMetric(100*(1-float64(searchTok)/float64(rawTok)), "pct-saved")
		})
	}
}

// Naive: read the entire source file. remindb: budget-bounded context fetch.
func BenchmarkTokens_FetchVsReadFile(b *testing.B) {
	st := openBenchStore(b)
	ctx := context.Background()
	dir, _ := filepath.Abs("testdata/bench")
	_, _ = compiler.CompileDir(ctx, st, dir, "init")
	eng := query.NewEngine(st)

	anchor := findDeepNode(ctx, st)
	if anchor == nil {
		b.Fatal("no suitable deep node found")
	}

	// Naive cost: read the whole source file that contains this node.
	sourceFile := filepath.Join(dir, anchor.SourceFile)
	fileData, err := os.ReadFile(sourceFile)
	if err != nil {
		b.Fatalf("read source file %s: %v", sourceFile, err)
	}
	fileTok := tokens.Estimate(string(fileData))

	for _, budget := range []int{500, 1000, 2000, 4000} {
		b.Run(fmt.Sprintf("budget/%d", budget), func(b *testing.B) {
			result, err := eng.Fetch(ctx, anchor.ID, budget)
			if err != nil {
				b.Fatal(err)
			}
			fetchTok := tokens.Estimate(query.Format(result))

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = eng.Fetch(ctx, anchor.ID, budget)
			}
			b.StopTimer()

			b.ReportMetric(float64(fileTok), "file-tok")
			b.ReportMetric(float64(fetchTok), "fetch-tok")
			b.ReportMetric(100*(1-float64(fetchTok)/float64(fileTok)), "pct-saved")
		})
	}
}

// Naive: re-read all files. remindb: compact delta of changed nodes only.
func BenchmarkTokens_DeltaVsReReadAll(b *testing.B) {
	st := openBenchStore(b)
	ctx := context.Background()
	dir, _ := filepath.Abs("testdata/bench")
	_, _ = compiler.CompileDir(ctx, st, dir, "init") // snapshot 1
	eng := query.NewEngine(st)

	rawTok := countDirTokens(b, dir)

	// Simulate two small changes after the initial compile.
	insertSmallDelta(b, st, ctx) // snapshot 2

	// Delta since snapshot 1: only the 2 new nodes, not the 221 initial ADDs.
	diffs, err := eng.Delta(ctx, 1)
	if err != nil {
		b.Fatal(err)
	}
	deltaTok := tokens.Estimate(renderDelta(diffs))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = eng.Delta(ctx, 1)
	}
	b.StopTimer()

	b.ReportMetric(float64(rawTok), "raw-tok")
	b.ReportMetric(float64(deltaTok), "delta-tok")
	b.ReportMetric(100*(1-float64(deltaTok)/float64(rawTok)), "pct-saved")
}

// Naive: read-all at startup + re-read for changes. remindb: tree + search + fetch + delta.
func BenchmarkTokens_AgentSession(b *testing.B) {
	dirs := []struct {
		name string
		dir  string
	}{
		{"bench", "testdata/bench"},
		{"openclaw", "testdata/openclaw"},
		{"codex", "testdata/codex"},
	}

	for _, tc := range dirs {
		dir, _ := filepath.Abs(tc.dir)
		b.Run(tc.name, func(b *testing.B) {
			st := openBenchStore(b)
			ctx := context.Background()
			_, _ = compiler.CompileDir(ctx, st, dir, "init")
			eng := query.NewEngine(st)

			rawTok := countDirTokens(b, dir)

			// --- Naive session cost ---
			// Read all files at startup + re-read all to detect changes.
			naiveTok := rawTok * 2

			// --- remindb session cost ---
			// 1. Tree overview at startup.
			treeOut := renderTree(ctx, st)
			sessionTok := tokens.Estimate(treeOut)

			// 2. Three targeted searches.
			for _, q := range []string{
				"authentication security",
				"configuration deployment",
				"error handling retry",
			} {
				result, err := eng.Search(ctx, q, 1000)
				if err != nil {
					continue
				}
				sessionTok += tokens.Estimate(query.Format(result))
			}

			// 3. Two context fetches.
			roots, _ := st.GetRootNodes(ctx)
			fetched := 0
			for _, root := range roots {
				if fetched >= 2 {
					break
				}
				result, err := eng.Fetch(ctx, root.ID, 1000)
				if err != nil {
					continue
				}
				sessionTok += tokens.Estimate(query.Format(result))
				fetched++
			}

			// 4. Small delta (2 changes since compile, not the full initial load).
			insertSmallDelta(b, st, ctx)
			diffs, _ := eng.Delta(ctx, 1)
			sessionTok += tokens.Estimate(renderDelta(diffs))

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				renderTree(ctx, st)
				_, _ = eng.Search(ctx, "authentication", 1000)
				_, _ = eng.Search(ctx, "configuration", 1000)
				if len(roots) > 0 {
					_, _ = eng.Fetch(ctx, roots[0].ID, 1000)
				}
				_, _ = eng.Delta(ctx, 1)
			}
			b.StopTimer()

			b.ReportMetric(float64(naiveTok), "naive-tok")
			b.ReportMetric(float64(sessionTok), "remindb-tok")
			b.ReportMetric(100*(1-float64(sessionTok)/float64(naiveTok)), "pct-saved")
		})
	}
}
