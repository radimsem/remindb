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

// renderTree produces the same output as the MemoryTree MCP tool.
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

// renderDelta produces the same output as the MemoryDelta MCP tool.
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

var deltaContents = []struct {
	label   string
	content string
}{
	{
		"Rate limiting migration to sliding window algorithm.",
		"The rate limiting subsystem has been migrated from a fixed token bucket " +
			"to a sliding window algorithm backed by Redis sorted sets. Each client request is " +
			"logged with a millisecond timestamp, and the window slides forward continuously " +
			"rather than resetting at fixed intervals. This eliminates the burst-at-boundary " +
			"problem where clients could send double their allowed rate by timing requests at " +
			"the edge of two consecutive windows. The migration required updating the Lua " +
			"scripts that enforce atomicity in Redis, as the sorted set operations have " +
			"different memory characteristics than the simple INCR/EXPIRE pattern used by " +
			"the token bucket. Rollback plan: revert the Lua scripts and restart the gateway " +
			"pods, which will re-initialize with the old token bucket counters.",
	},
	{
		"Post-migration analysis shows a 15% reduction in false-positive rejections.",
		"Post-migration analysis shows a 15% reduction in false-positive rate " +
			"limit rejections during traffic spikes. The sliding window approach handles bursty " +
			"traffic more gracefully because it considers the actual distribution of requests " +
			"within the window rather than a simple count. Latency overhead is negligible at " +
			"0.3ms per request at p99, compared to 0.1ms for the previous approach. Teams " +
			"should update their Grafana dashboards to include the new " +
			"gateway_ratelimit_window_utilization metric, which shows what percentage of each " +
			"client's window is consumed at any given moment. The metric is already exported " +
			"by the gateway and available in the Prometheus scrape targets.",
	},
	{
		"Circuit breaker thresholds tuned for downstream latency spikes.",
		"The circuit breaker configuration for the payment service has been adjusted after " +
			"repeated timeout cascades during peak checkout hours. The failure threshold moves " +
			"from 5 consecutive failures to a sliding error-rate window of 20% over 30 seconds, " +
			"which avoids tripping on isolated slow responses while still catching sustained " +
			"degradation. The half-open state now probes with a single health-check request " +
			"rather than allowing a full traffic burst, preventing the downstream service from " +
			"being overwhelmed during recovery. Dashboards updated to track breaker state " +
			"transitions and the error-rate window value in real time.",
	},
	{
		"Retry budget enforced per service to prevent cascading amplification.",
		"A global retry budget has been introduced at the service mesh layer to cap the " +
			"total number of retries any single service can issue within a 10-second window. " +
			"Previously each caller retried independently, which during partial outages could " +
			"amplify traffic by 3-4x and turn a minor degradation into a full cascade. The " +
			"budget is set to 20% of the baseline request rate, meaning at most one in five " +
			"requests can be a retry. Callers that exhaust the budget receive a local fast-fail " +
			"rather than queueing, which keeps latency predictable for end users even when " +
			"the backend is struggling.",
	},
}

// Simulate file modifications: a section rewrite and an appended paragraph.
func insertDelta(b *testing.B, st *store.Store, ctx context.Context, round int) string {
	b.Helper()

	roots, err := st.GetRootNodes(ctx)
	if err != nil || len(roots) == 0 {
		b.Fatal("no root nodes for delta")
	}
	sourceFile := roots[0].SourceFile

	idx := (round * 2) % len(deltaContents)
	mod := deltaContents[idx]
	add := deltaContents[(idx+1)%len(deltaContents)]

	modID := fmt.Sprintf("delta_mod_%d", round)
	addID := fmt.Sprintf("delta_add_%d", round)
	modHash := modID + "_hash"
	addHash := addID + "_hash"

	err = st.Tx(ctx, func(tx *sql.Tx) error {
		_ = st.UpsertNodeTx(ctx, tx, &store.Node{
			ID: modID, SourceFile: sourceFile,
			NodeType: string(parser.NodeText), Depth: 1,
			Label: mod.label, Content: mod.content, Format: parser.FormatPlain,
			TokenCount: tokens.Estimate(mod.content), ContentHash: modHash,
		})
		_ = st.UpsertNodeTx(ctx, tx, &store.Node{
			ID: addID, SourceFile: sourceFile,
			NodeType: string(parser.NodeText), Depth: 1,
			Label: add.label, Content: add.content, Format: parser.FormatPlain,
			TokenCount: tokens.Estimate(add.content), ContentHash: addHash,
		})

		cursorHash := fmt.Sprintf("bench-delta-%d", round)
		snapID, err := st.CreateSnapshotTx(ctx, tx, cursorHash, "bench-write")
		if err != nil {
			return err
		}

		for _, d := range []store.DiffRecord{
			{SnapshotID: snapID, NodeID: modID, Op: "modify", NewHash: modHash, NewContent: mod.content},
			{SnapshotID: snapID, NodeID: addID, Op: "add", NewHash: addHash, NewContent: add.content},
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
				fetchResult, err := eng.Fetch(ctx, searchResult.Nodes[0].Node.ID, tc.budget, 0)
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
			result, err := eng.Fetch(ctx, anchor.ID, budget, 0)
			if err != nil {
				b.Fatal(err)
			}
			fetchTok := tokens.Estimate(query.Format(result))

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = eng.Fetch(ctx, anchor.ID, budget, 0)
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

	modifiedFile := insertDelta(b, st, ctx, 0) // snapshot 2

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

// sessionCost tracks cumulative token costs for both sides of a session benchmark.
type sessionCost struct {
	naive   int
	remindb int
}

// Orientation: agent explores the memory structure.
// Naive: list files + read all contents. remindb: tree.
func (c *sessionCost) orientation(b *testing.B, ctx context.Context, st *store.Store, dir string) {
	b.Helper()
	c.naive += tokens.Estimate(listDirFiles(dir)) + countDirTokens(b, dir)
	c.remindb += tokens.Estimate(renderTree(ctx, st))
}

// Only fetches the top result when its token count exceeds minFetchTok —
// small nodes are already well-summarized by their label in compact output.
func (c *sessionCost) search(ctx context.Context, eng *query.Engine, dir string, queries []string, budget, minFetchTok int) {
	for _, q := range queries {
		grepOut, matchFiles := grepDir(dir, strings.Fields(q))
		c.naive += tokens.Estimate(grepOut) + sumFileTokens(matchFiles)

		result, err := eng.Search(ctx, q, budget)
		if err != nil {
			continue
		}
		c.remindb += tokens.Estimate(query.FormatCompact(result))

		if len(result.Nodes) > 0 && result.Nodes[0].Node.TokenCount >= minFetchTok {
			fr, err := eng.Fetch(ctx, result.Nodes[0].Node.ID, budget, 0)
			if err == nil {
				c.remindb += tokens.Estimate(query.Format(fr))
			}
		}
	}
}

// Deep read: agent reads specific nodes in detail.
// Naive: read the source file. remindb: fetch with budget.
func (c *sessionCost) deepRead(ctx context.Context, eng *query.Engine, st *store.Store, dir string, n, budget int) {
	roots, _ := st.GetRootNodes(ctx)
	seen := make(map[string]bool)
	fetched := 0

	for _, root := range roots {
		if fetched >= n {
			break
		}

		path := filepath.Join(dir, root.SourceFile)
		if !seen[path] {
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			c.naive += tokens.Estimate(string(data))
			seen[path] = true
		}

		result, err := eng.Fetch(ctx, root.ID, budget, 0)
		if err != nil {
			continue
		}
		c.remindb += tokens.Estimate(query.Format(result))
		fetched++
	}
}

// Delta: agent checks what changed since a snapshot.
// Naive: read the modified file. remindb: compact diff.
func (c *sessionCost) delta(b *testing.B, ctx context.Context, eng *query.Engine, st *store.Store, dir string, round int, sinceSnapshot int64) {
	b.Helper()
	modifiedFile := insertDelta(b, st, ctx, round)

	data, err := os.ReadFile(filepath.Join(dir, modifiedFile))
	if err == nil {
		c.naive += tokens.Estimate(string(data))
	}

	diffs, _ := eng.Delta(ctx, sinceSnapshot)
	c.remindb += tokens.Estimate(renderDelta(diffs))
}

// Simulates a realistic agent session: orient, search, deep-read, modify, verify, iterate.
func BenchmarkTokens_AgentSession(b *testing.B) {
	dirs := []struct {
		name string
		dir  string
	}{
		{"openclaw", "testdata/openclaw"},
		{"codex", "testdata/codex"},
		{"claude_code", "testdata/claude-code"},
		{"gemini_cli", "testdata/gemini-cli"},
	}

	for _, tc := range dirs {
		dir, _ := filepath.Abs(tc.dir)
		b.Run(tc.name, func(b *testing.B) {
			st := openBenchStore(b)
			ctx := context.Background()
			_, _ = compiler.CompileDir(ctx, st, dir, "init")
			eng := query.NewEngine(st)

			var c sessionCost

			// Phase 1: Orientation.
			c.orientation(b, ctx, st, dir)

			// Phase 2: Exploration — 5 search queries, fetch top hit if large enough.
			c.search(ctx, eng, dir, []string{
				"authentication security",
				"configuration deployment",
				"error handling retry",
				"testing validation",
				"monitoring observability",
			}, 1000, 200)

			// Phase 3: Deep read — 3 fetches from distinct source files.
			c.deepRead(ctx, eng, st, dir, 3, 1000)

			// Phase 4: First modification round + delta.
			c.delta(b, ctx, eng, st, dir, 0, 1)

			// Phase 5: Re-orient after changes.
			c.orientation(b, ctx, st, dir)

			// Phase 6: Follow-up searches — agent iterates after seeing delta.
			c.search(ctx, eng, dir, []string{
				"rate limiting migration",
				"deployment rollback",
			}, 1000, 200)

			// Phase 7: Second modification round + delta.
			c.delta(b, ctx, eng, st, dir, 1, 2)

			// Phase 8: Targeted deep reads — agent verifies specific nodes.
			deep := findDeepNode(ctx, st)
			if deep != nil {
				data, err := os.ReadFile(filepath.Join(dir, deep.SourceFile))
				if err == nil {
					c.naive += tokens.Estimate(string(data))
				}
				result, err := eng.Fetch(ctx, deep.ID, 1000, 0)
				if err == nil {
					c.remindb += tokens.Estimate(query.Format(result))
				}
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				renderTree(ctx, st)
				_, _ = eng.Search(ctx, "authentication", 1000)
				_, _ = eng.Search(ctx, "configuration", 1000)
				_, _ = eng.Search(ctx, "monitoring", 1000)
				roots, _ := st.GetRootNodes(ctx)
				if len(roots) > 0 {
					_, _ = eng.Fetch(ctx, roots[0].ID, 1000, 0)
				}
				_, _ = eng.Delta(ctx, 1)
			}
			b.StopTimer()

			b.ReportMetric(float64(c.naive), "naive-tok")
			b.ReportMetric(float64(c.remindb), "remindb-tok")
			b.ReportMetric(100*(1-float64(c.remindb)/float64(c.naive)), "pct-saved")
		})
	}
}
