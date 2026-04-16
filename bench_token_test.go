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
	fmt.Fprintf(b, "%s[%s] %s (id=%s file=%s temp=%.2f tok=%d)\n", indent, n.NodeType, n.Label, n.ID, n.SourceFile, n.Temperature, n.TokenCount)

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

// Simulate realistic file modifications: a section rewrite and an appended paragraph.
// Returns the source file that was "modified" so callers can compute naive read cost.
func insertDelta(b *testing.B, st *store.Store, ctx context.Context) string {
	b.Helper()

	// Pick a real source file from the compiled corpus.
	roots, err := st.GetRootNodes(ctx)
	if err != nil || len(roots) == 0 {
		b.Fatal("no root nodes for delta")
	}
	sourceFile := roots[0].SourceFile

	modifiedContent := "The rate limiting subsystem has been migrated from a fixed token bucket " +
		"to a sliding window algorithm backed by Redis sorted sets. Each client request is " +
		"logged with a millisecond timestamp, and the window slides forward continuously " +
		"rather than resetting at fixed intervals. This eliminates the burst-at-boundary " +
		"problem where clients could send double their allowed rate by timing requests at " +
		"the edge of two consecutive windows. The migration required updating the Lua " +
		"scripts that enforce atomicity in Redis, as the sorted set operations have " +
		"different memory characteristics than the simple INCR/EXPIRE pattern used by " +
		"the token bucket. Rollback plan: revert the Lua scripts and restart the gateway " +
		"pods, which will re-initialize with the old token bucket counters."

	appendedContent := "Post-migration analysis shows a 15% reduction in false-positive rate " +
		"limit rejections during traffic spikes. The sliding window approach handles bursty " +
		"traffic more gracefully because it considers the actual distribution of requests " +
		"within the window rather than a simple count. Latency overhead is negligible at " +
		"0.3ms per request at p99, compared to 0.1ms for the previous approach. Teams " +
		"should update their Grafana dashboards to include the new " +
		"gateway_ratelimit_window_utilization metric, which shows what percentage of each " +
		"client's window is consumed at any given moment. The metric is already exported " +
		"by the gateway and available in the Prometheus scrape targets."

	err = st.Tx(ctx, func(tx *sql.Tx) error {
		_ = st.UpsertNodeTx(ctx, tx, &store.Node{
			ID: "delta_mod1", SourceFile: sourceFile,
			NodeType: string(parser.NodeText), Depth: 1,
			Label:   "Rate limiting migration to sliding window algorithm.",
			Content: modifiedContent, Format: parser.FormatPlain,
			TokenCount:  tokens.Estimate(modifiedContent),
			ContentHash: "delta_mod1_hash",
		})
		_ = st.UpsertNodeTx(ctx, tx, &store.Node{
			ID: "delta_add1", SourceFile: sourceFile,
			NodeType: string(parser.NodeText), Depth: 1,
			Label:   "Post-migration analysis shows a 15% reduction in false-positive rejections.",
			Content: appendedContent, Format: parser.FormatPlain,
			TokenCount:  tokens.Estimate(appendedContent),
			ContentHash: "delta_add1_hash",
		})

		snapID, err := st.CreateSnapshotTx(ctx, tx, "bench-delta", "bench-write")
		if err != nil {
			return err
		}

		for _, d := range []store.DiffRecord{
			{SnapshotID: snapID, NodeID: "delta_mod1", Op: "modify", NewHash: "delta_mod1_hash", NewContent: modifiedContent},
			{SnapshotID: snapID, NodeID: "delta_add1", Op: "add", NewHash: "delta_add1_hash", NewContent: appendedContent},
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
	return sourceFile
}

// Simulate an agent's Grep tool: find lines matching any term, return output and matching file paths.
func grepDir(dir string, terms []string) (string, []string) {
	var b strings.Builder
	seen := make(map[string]bool)
	var matched []string

	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !fileext.Supported(path) {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		rel, _ := filepath.Rel(dir, path)
		for i, line := range strings.Split(string(data), "\n") {
			lower := strings.ToLower(line)
			for _, term := range terms {
				if strings.Contains(lower, strings.ToLower(term)) {
					fmt.Fprintf(&b, "%s:%d:%s\n", rel, i+1, line)
					if !seen[path] {
						seen[path] = true
						matched = append(matched, path)
					}
					break
				}
			}
		}
		return nil
	})

	return b.String(), matched
}

func sumFileTokens(files []string) int {
	total := 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		total += tokens.Estimate(string(data))
	}
	return total
}

// Simulate `find` output: one line per file with relative path.
func listDirFiles(dir string) string {
	var b strings.Builder
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !fileext.Supported(path) {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		fmt.Fprintf(&b, "./%s\n", rel)
		return nil
	})
	return b.String()
}

// Naive: list files + read all contents. remindb: compact tree with metadata.
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

			naiveTok := tokens.Estimate(listDirFiles(dir)) + countDirTokens(b, dir)
			treeOut := renderTree(ctx, st)
			treeTok := tokens.Estimate(treeOut)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				renderTree(ctx, st)
			}
			b.StopTimer()

			b.ReportMetric(float64(naiveTok), "naive-tok")
			b.ReportMetric(float64(treeTok), "tree-tok")
			b.ReportMetric(100*(1-float64(treeTok)/float64(naiveTok)), "pct-saved")
		})
	}
}

// Naive: grep + read matching files. remindb: compact search + fetch top result.
func BenchmarkTokens_SearchVsGrep(b *testing.B) {
	st := openBenchStore(b)
	ctx := context.Background()
	dir, _ := filepath.Abs("testdata/bench")
	_, _ = compiler.CompileDir(ctx, st, dir, "init")
	eng := query.NewEngine(st)

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
			// Naive: grep for terms, then read matching files.
			grepOut, matchFiles := grepDir(dir, strings.Fields(tc.q))
			naiveTok := tokens.Estimate(grepOut) + sumFileTokens(matchFiles)

			// remindb: compact search results + fetch top result.
			searchResult, err := eng.Search(ctx, tc.q, tc.budget)
			if err != nil {
				b.Fatal(err)
			}
			remindbTok := tokens.Estimate(query.FormatCompact(searchResult))
			if len(searchResult.Nodes) > 0 {
				fetchResult, err := eng.Fetch(ctx, searchResult.Nodes[0].Node.ID, tc.budget)
				if err == nil {
					remindbTok += tokens.Estimate(query.Format(fetchResult))
				}
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = eng.Search(ctx, tc.q, tc.budget)
			}
			b.StopTimer()

			b.ReportMetric(float64(naiveTok), "naive-tok")
			b.ReportMetric(float64(remindbTok), "remindb-tok")
			b.ReportMetric(100*(1-float64(remindbTok)/float64(naiveTok)), "pct-saved")
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

// Naive: read the modified file. remindb: compact delta of changed nodes only.
func BenchmarkTokens_DeltaVsReadFile(b *testing.B) {
	st := openBenchStore(b)
	ctx := context.Background()
	dir, _ := filepath.Abs("testdata/bench")
	_, _ = compiler.CompileDir(ctx, st, dir, "init") // snapshot 1
	eng := query.NewEngine(st)

	modifiedFile := insertDelta(b, st, ctx) // snapshot 2

	// Naive: agent reads the full file that was modified.
	data, err := os.ReadFile(filepath.Join(dir, modifiedFile))
	if err != nil {
		b.Fatalf("read modified file %s: %v", modifiedFile, err)
	}
	naiveTok := tokens.Estimate(string(data))

	// remindb: delta since snapshot 1.
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

	b.ReportMetric(float64(naiveTok), "naive-tok")
	b.ReportMetric(float64(deltaTok), "delta-tok")
	b.ReportMetric(100*(1-float64(deltaTok)/float64(naiveTok)), "pct-saved")
}

// Phase-by-phase comparison: both naive and remindb costs computed from real operations.
func BenchmarkTokens_AgentSession(b *testing.B) {
	dirs := []struct {
		name string
		dir  string
	}{
		{"bench", "testdata/bench"},
		{"openclaw", "testdata/openclaw"},
		{"codex", "testdata/codex"},
	}

	searchQueries := []string{
		"authentication security",
		"configuration deployment",
		"error handling retry",
	}

	for _, tc := range dirs {
		dir, _ := filepath.Abs(tc.dir)
		b.Run(tc.name, func(b *testing.B) {
			st := openBenchStore(b)
			ctx := context.Background()
			_, _ = compiler.CompileDir(ctx, st, dir, "init")
			eng := query.NewEngine(st)

			var naiveTok, remindbTok int

			// Phase 1: Orientation.
			// Naive: read all files. remindb: tree.
			naiveTok += countDirTokens(b, dir)
			remindbTok += tokens.Estimate(renderTree(ctx, st))

			// Phase 2: Search (3 queries).
			// Naive: grep + read matching files. remindb: compact search + fetch top result.
			for _, q := range searchQueries {
				grepOut, matchFiles := grepDir(dir, strings.Fields(q))
				naiveTok += tokens.Estimate(grepOut) + sumFileTokens(matchFiles)

				searchResult, err := eng.Search(ctx, q, 1000)
				if err != nil {
					continue
				}
				remindbTok += tokens.Estimate(query.FormatCompact(searchResult))
				if len(searchResult.Nodes) > 0 {
					fetchResult, err := eng.Fetch(ctx, searchResult.Nodes[0].Node.ID, 1000)
					if err == nil {
						remindbTok += tokens.Estimate(query.Format(fetchResult))
					}
				}
			}

			// Phase 3: Deep read (2 fetches from different source files).
			// Naive: read the source file. remindb: fetch with budget.
			roots, _ := st.GetRootNodes(ctx)
			readFiles := make(map[string]bool)
			fetched := 0
			for _, root := range roots {
				if fetched >= 2 {
					break
				}

				sourceFile := filepath.Join(dir, root.SourceFile)
				if !readFiles[sourceFile] {
					data, err := os.ReadFile(sourceFile)
					if err != nil {
						continue
					}
					naiveTok += tokens.Estimate(string(data))
					readFiles[sourceFile] = true
				}

				result, err := eng.Fetch(ctx, root.ID, 1000)
				if err != nil {
					continue
				}
				remindbTok += tokens.Estimate(query.Format(result))
				fetched++
			}

			// Phase 4: Delta.
			// Naive: read the modified file. remindb: delta.
			modifiedFile := insertDelta(b, st, ctx)
			data, err := os.ReadFile(filepath.Join(dir, modifiedFile))
			if err == nil {
				naiveTok += tokens.Estimate(string(data))
			}
			diffs, _ := eng.Delta(ctx, 1)
			remindbTok += tokens.Estimate(renderDelta(diffs))

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
			b.ReportMetric(float64(remindbTok), "remindb-tok")
			b.ReportMetric(100*(1-float64(remindbTok)/float64(naiveTok)), "pct-saved")
		})
	}
}
