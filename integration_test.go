package remindb_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/radimsem/remindb/internal/testutil"
	"github.com/radimsem/remindb/pkg/compiler"
	"github.com/radimsem/remindb/pkg/query"
	"github.com/radimsem/remindb/pkg/temperature"
)

// Simulates compiling an OpenClaw agent definition,
// then searching and fetching context as the agent would at runtime.
func TestOpenClawAgentWorkflow(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()

	result, err := compiler.CompileDir(ctx, st, "testdata/openclaw", "openclaw-agent")
	if err != nil {
		t.Fatalf("CompileDir: %v", err)
	}
	if result.Added == 0 {
		t.Fatal("expected nodes from OpenClaw fixtures")
	}
	t.Logf("compiled: +%d ~%d -%d (%d total)", result.Added, result.Modified, result.Removed, result.Total)

	// Verify tree structure: should have roots from 3 files.
	roots, err := st.GetRootNodes(ctx)
	if err != nil {
		t.Fatalf("GetRootNodes: %v", err)
	}
	if len(roots) < 3 {
		t.Errorf("roots = %d, want >= 3 (SOUL, IDENTITY, PROTOCOLS preambles + headings)", len(roots))
	}

	// Search for agent capabilities — should find nodes from IDENTITY.md.
	eng := query.NewEngine(st)
	searchResult, err := eng.Search(ctx, "code review refactoring security", 2000)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(searchResult.Nodes) == 0 {
		t.Fatal("expected search results for 'code review refactoring security'")
	}

	found := false
	for _, sn := range searchResult.Nodes {
		if strings.Contains(sn.Node.Content, "refactor") {
			found = true
			break
		}
	}
	if !found {
		t.Error("search results should include IDENTITY.md capabilities content")
	}

	// Search for memory protocol — should find nodes from PROTOCOLS.md.
	memResult, err := eng.Search(ctx, "memory protocol feedback recall", 2000)
	if err != nil {
		t.Fatalf("Search memory: %v", err)
	}
	if len(memResult.Nodes) == 0 {
		t.Fatal("expected search results for 'memory protocol feedback recall'")
	}

	// Fetch context around a specific node.
	fetchResult, err := eng.Fetch(ctx, searchResult.Nodes[0].Node.ID, 4000)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if fetchResult.TokensUsed == 0 {
		t.Error("expected non-zero token usage in fetch result")
	}
	t.Logf("fetch: %d nodes, %d tokens", len(fetchResult.Nodes), fetchResult.TokensUsed)
}

// TestClaudeCodeMemoryWorkflow simulates the memory workflow of a Claude Code
// session: compile project instructions and memory files, then search for
// feedback and project context as the agent would when starting a task.
func TestClaudeCodeMemoryWorkflow(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()

	result, err := compiler.CompileDir(ctx, st, "testdata/claude-code", "claude-code-memory")
	if err != nil {
		t.Fatalf("CompileDir: %v", err)
	}
	if result.Added == 0 {
		t.Fatal("expected nodes from Claude Code fixtures")
	}
	t.Logf("compiled: +%d ~%d -%d (%d total)", result.Added, result.Modified, result.Removed, result.Total)

	eng := query.NewEngine(st)

	// Agent starts a task: "add a new page to the webshop".
	// It should find the CLAUDE.md instructions about pages and components.
	pageResult, err := eng.Search(ctx, "adding new page server components", 2000)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(pageResult.Nodes) == 0 {
		t.Fatal("expected results for 'adding new page server components'")
	}

	// Agent checks for testing feedback before writing tests.
	testResult, err := eng.Search(ctx, "snapshot tests mock database", 2000)
	if err != nil {
		t.Fatalf("Search testing: %v", err)
	}
	if len(testResult.Nodes) == 0 {
		t.Fatal("expected results for testing feedback")
	}

	hasSnapshotWarning := false
	for _, sn := range testResult.Nodes {
		if strings.Contains(sn.Node.Content, "snapshot") {
			hasSnapshotWarning = true
			break
		}
	}
	if !hasSnapshotWarning {
		t.Error("testing search should surface the snapshot test feedback")
	}

	// Agent checks project state for current sprint context.
	sprintResult, err := eng.Search(ctx, "checkout Stripe sprint blockers", 2000)
	if err != nil {
		t.Fatalf("Search sprint: %v", err)
	}
	if len(sprintResult.Nodes) == 0 {
		t.Fatal("expected results for current sprint context")
	}

	// Verify user preference was compiled.
	userResult, err := eng.Search(ctx, "senior engineer Zod validation preferences", 2000)
	if err != nil {
		t.Fatalf("Search user: %v", err)
	}
	if len(userResult.Nodes) == 0 {
		t.Fatal("expected results for user preferences")
	}
}

// TestGeminiCliMemoryWorkflow simulates a Gemini CLI session working on the
// infra-api project: compile mixed markdown+yaml fixtures, search for
// architecture decisions and incident history.
func TestGeminiCliMemoryWorkflow(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()

	result, err := compiler.CompileDir(ctx, st, "testdata/gemini-cli", "gemini-cli-memory")
	if err != nil {
		t.Fatalf("CompileDir: %v", err)
	}
	if result.Added == 0 {
		t.Fatal("expected nodes from Gemini CLI fixtures")
	}
	t.Logf("compiled: +%d ~%d -%d (%d total)", result.Added, result.Modified, result.Removed, result.Total)

	eng := query.NewEngine(st)

	// Agent searches for architecture decisions before modifying the k8s layer.
	archResult, err := eng.Search(ctx, "service layer kubernetes handler idempotent", 2000)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(archResult.Nodes) == 0 {
		t.Fatal("expected results for architecture decisions")
	}

	// Agent checks incident history before touching namespace operations.
	incidentResult, err := eng.Search(ctx, "namespace deletion cascade finalizer", 2000)
	if err != nil {
		t.Fatalf("Search incidents: %v", err)
	}
	if len(incidentResult.Nodes) == 0 {
		t.Fatal("expected results for incident history")
	}

	// Verify YAML context was parsed — should find service dependencies.
	depResult, err := eng.Search(ctx, "postgresql vault kubernetes dependencies", 2000)
	if err != nil {
		t.Fatalf("Search deps: %v", err)
	}
	if len(depResult.Nodes) == 0 {
		t.Fatal("expected results for YAML service dependencies")
	}
}

// TestRecompileWorkflow tests incremental recompilation: compile, modify a
// file, recompile, verify snapshots and diffs accumulate.
func TestRecompileWorkflow(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()
	dir := t.TempDir()

	// Copy a fixture to a temp dir so we can modify it.
	src, err := os.ReadFile("testdata/claude-code/memory/project_state.md")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	p := filepath.Join(dir, "project_state.md")
	if err := os.WriteFile(p, src, 0o644); err != nil {
		t.Fatal(err)
	}

	// First compile.
	r1, err := compiler.Compile(ctx, st, []string{p}, "v1")
	if err != nil {
		t.Fatalf("Compile v1: %v", err)
	}
	t.Logf("v1: +%d ~%d -%d (%d total)", r1.Added, r1.Modified, r1.Removed, r1.Total)

	snaps, _ := st.ListSnapshots(ctx, 10)
	if len(snaps) != 1 {
		t.Fatalf("snapshots = %d, want 1", len(snaps))
	}

	// Modify the file — simulate sprint progress update.
	updated := strings.ReplaceAll(string(src),
		"Implement checkout flow with Stripe integration",
		"Checkout flow shipped, now in QA")
	if err := os.WriteFile(p, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}

	// Recompile.
	r2, err := compiler.Compile(ctx, st, []string{p}, "v2")
	if err != nil {
		t.Fatalf("Compile v2: %v", err)
	}
	t.Logf("v2: +%d ~%d -%d (%d total)", r2.Added, r2.Modified, r2.Removed, r2.Total)

	snaps, _ = st.ListSnapshots(ctx, 10)
	if len(snaps) != 2 {
		t.Fatalf("snapshots = %d, want 2", len(snaps))
	}

	// Delta query should show changes between v1 and v2.
	eng := query.NewEngine(st)
	diffs, err := eng.Delta(ctx, snaps[1].ID)
	if err != nil {
		t.Fatalf("Delta: %v", err)
	}
	if len(diffs) == 0 {
		t.Error("expected diffs between v1 and v2")
	}
	t.Logf("delta: %d changes", len(diffs))
}

// TestTemperatureBoostOnAccess verifies that querying nodes boosts their
// temperature, simulating the "frequently accessed = hotter" behavior.
func TestTemperatureBoostOnAccess(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()

	_, err := compiler.CompileDir(ctx, st, "testdata/openclaw", "temp-test")
	if err != nil {
		t.Fatalf("CompileDir: %v", err)
	}

	cfg := temperature.DefaultConfig()
	tracker := temperature.NewTracker(st, cfg)

	// Find a node via search.
	eng := query.NewEngine(st)
	result, err := eng.Search(ctx, "precision speed verify", 2000)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(result.Nodes) == 0 {
		t.Fatal("expected search results")
	}

	nodeID := result.Nodes[0].Node.ID

	// Read temperature before boost.
	before, err := st.GetNode(ctx, nodeID)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	tempBefore := before.Temperature

	// Simulate agent accessing this node (same as MCP tool handler does).
	if err := tracker.RecordAccess(ctx, []string{nodeID}); err != nil {
		t.Fatalf("RecordAccess: %v", err)
	}

	// Temperature should have increased.
	after, err := st.GetNode(ctx, nodeID)
	if err != nil {
		t.Fatalf("GetNode after: %v", err)
	}
	if after.Temperature <= tempBefore {
		t.Errorf("temperature did not increase: before=%.3f after=%.3f", tempBefore, after.Temperature)
	}
	if after.AccessCount != before.AccessCount+1 {
		t.Errorf("access_count = %d, want %d", after.AccessCount, before.AccessCount+1)
	}
	t.Logf("temperature: %.3f -> %.3f", tempBefore, after.Temperature)
}

// TestCrossFormatSearch verifies that search works across all fixture formats
// (markdown, YAML, JSON) after compiling the original testdata samples.
func TestCrossFormatSearch(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()

	mdPath := abs(t, "testdata/sample.md")
	yamlPath := abs(t, "testdata/sample.yaml")
	jsonPath := abs(t, "testdata/sample.json")

	_, err := compiler.Compile(ctx, st, []string{mdPath, yamlPath, jsonPath}, "cross-format")
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	eng := query.NewEngine(st)

	// Search for content that exists in markdown.
	mdResult, err := eng.Search(ctx, "paragraph", 2000)
	if err != nil {
		t.Fatalf("Search md: %v", err)
	}
	if len(mdResult.Nodes) == 0 {
		t.Error("expected markdown content in search results")
	}

	// Search for content across YAML and JSON (both have "remindb").
	nameResult, err := eng.Search(ctx, "remindb", 2000)
	if err != nil {
		t.Fatalf("Search name: %v", err)
	}
	if len(nameResult.Nodes) == 0 {
		t.Error("expected results matching 'remindb' from YAML/JSON fixtures")
	}

	// Verify stats reflect all three formats.
	stats, err := st.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.NodeCount < 3 {
		t.Errorf("NodeCount = %d, want >= 3 (nodes from 3 files)", stats.NodeCount)
	}
	t.Logf("cross-format: %d nodes, %d snapshots", stats.NodeCount, stats.SnapshotCount)
}

func abs(t *testing.T, path string) string {
	t.Helper()
	p, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
