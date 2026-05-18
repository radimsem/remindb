package remindb_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/internal/mcptest"
	"github.com/radimsem/remindb/internal/testutil"
	remindb "github.com/radimsem/remindb/pkg/mcp"
	"github.com/radimsem/remindb/pkg/store"
	"github.com/radimsem/remindb/pkg/temperature"
)

// Simulates an OpenClaw agent session.
func TestMcp_OpenClawAgent(t *testing.T) {
	env := mcptest.NewEnv(t)
	dir, _ := filepath.Abs("testdata/openclaw")

	// 1. Agent compiles its identity files into the database.
	compileResult := env.CallTool(t, "MemoryCompile", map[string]any{
		"path":    dir,
		"message": "openclaw-init",
	})
	text := env.TextContent(t, compileResult)
	if !strings.Contains(text, "compiled") {
		t.Fatalf("unexpected compile result: %s", text)
	}

	// 2. Agent inspects its memory tree — should include all workspace files.
	treeResult := env.CallTool(t, "MemoryTree", map[string]any{})
	treeText := env.TextContent(t, treeResult)

	const limit = 200
	if !strings.Contains(treeText, "Soul") && !strings.Contains(treeText, "Identity") {
		t.Errorf("tree should contain Soul/Identity headings, got: %s", treeText[:min(limit, len(treeText))])
	}

	// 3. Agent searches for its capabilities to self-describe.
	searchResult := env.CallTool(t, "MemorySearch", map[string]any{
		"query":  "refactoring security vulnerabilities",
		"budget": 2000,
	})
	searchText := env.TextContent(t, searchResult)
	if searchText == "no results" {
		t.Fatal("expected search results for capabilities")
	}

	// 4. Agent checks user preferences before responding.
	userResult := env.CallTool(t, "MemorySearch", map[string]any{
		"query":  "terse responses error handling",
		"budget": 1000,
	})
	userText := env.TextContent(t, userResult)
	if userText == "no results" {
		t.Fatal("expected search results for user preferences from USER.md")
	}

	// 5. Agent checks daily memory logs for recent session context.
	dailyResult := env.CallTool(t, "MemorySearch", map[string]any{
		"query":  "rate limiting token bucket",
		"budget": 1000,
	})
	dailyText := env.TextContent(t, dailyResult)
	if dailyText == "no results" {
		t.Fatal("expected search results for daily memory log content")
	}

	// 6. Agent searches for JSON session data from memory/session_data.json.
	jsonResult := env.CallTool(t, "MemorySearch", map[string]any{
		"query":  "session tasks blocked WebSocket",
		"budget": 1000,
	})
	jsonText := env.TextContent(t, jsonResult)
	if jsonText == "no results" {
		t.Fatal("expected search results for JSON session data")
	}

	// 7. Agent writes a new memory from a conversation.
	writeResult := env.CallTool(t, "MemoryWrite", map[string]any{
		"payload": "User prefers verbose explanations when reviewing Go code. Confirmed after code review session.",
	})
	writeText := env.TextContent(t, writeResult)
	if !strings.Contains(writeText, "wrote node") {
		t.Fatalf("unexpected write result: %s", writeText)
	}

	// Extract the node ID from "wrote node XXXXXXXX (N tokens)".
	nodeID := extractNodeID(writeText)

	// 8. Agent fetches context around the new memory.
	fetchResult := env.CallTool(t, "MemoryFetch", map[string]any{
		"anchor": nodeID,
		"budget": 1000,
	})
	fetchText := env.TextContent(t, fetchResult)
	if !strings.Contains(fetchText, "verbose explanations") {
		t.Errorf("fetch should include the written content, got: %s", fetchText[:min(100, len(fetchText))])
	}

	// 9. Agent checks delta since the compile snapshot.
	deltaResult := env.CallTool(t, "MemoryDelta", map[string]any{
		"since_snapshot": 1,
	})
	deltaText := env.TextContent(t, deltaResult)
	if deltaText == "no changes" {
		t.Error("expected delta changes after write")
	}
}

// Simulates a Claude Code session.
func TestMcp_ClaudeCodeAgent(t *testing.T) {
	env := mcptest.NewEnv(t)
	dir, _ := filepath.Abs("testdata/claude-code")

	// 1. Compile the project instructions and memory files.
	env.CallTool(t, "MemoryCompile", map[string]any{
		"path":    dir,
		"message": "claude-code-init",
	})

	// 2. Agent starts a task and searches for relevant testing guidance.
	searchResult := env.CallTool(t, "MemorySearch", map[string]any{
		"query":  "snapshot",
		"budget": 2000,
	})
	searchText := env.TextContent(t, searchResult)
	if !strings.Contains(searchText, "snapshot") {
		t.Fatal("search should find the snapshot testing feedback")
	}

	// 3. Agent searches for user preferences before responding.
	prefResult := env.CallTool(t, "MemorySearch", map[string]any{
		"query":  "Zod",
		"budget": 1000,
	})
	prefText := env.TextContent(t, prefResult)
	if !strings.Contains(prefText, "user_preferences.md") {
		t.Fatal("search should find user Zod preference")
	}

	// 4. Agent writes a new feedback memory after user correction.
	env.CallTool(t, "MemoryWrite", map[string]any{
		"payload": "User prefers function components over class components. Always use hooks for state management.",
	})

	// 5. Agent searches for the newly written memory to verify persistence.
	hookResult := env.CallTool(t, "MemorySearch", map[string]any{
		"query":  "hooks",
		"budget": 1000,
	})
	hookText := env.TextContent(t, hookResult)
	if !strings.Contains(hookText, "function components") {
		t.Fatal("newly written memory should be searchable")
	}

	// 6. Agent finds a verbose node and summarizes it.
	treeResult := env.CallTool(t, "MemoryTree", map[string]any{})
	treeText := env.TextContent(t, treeResult)

	// Find a node ID from the tree output to summarize.
	nodeID := extractFirstNodeID(treeText)
	if nodeID == "" {
		t.Fatal("could not find a node ID in tree output")
	}

	env.CallTool(t, "MemorySummarize", map[string]any{
		"node_id": nodeID,
		"summary": "Summarized: webshop project uses Next.js 15 with App Router.",
	})

	// Verify the summarization took effect.
	fetchResult := env.CallTool(t, "MemoryFetch", map[string]any{
		"anchor": nodeID,
		"budget": 1000,
	})
	fetchText := env.TextContent(t, fetchResult)
	if !strings.Contains(fetchText, "Summarized") {
		t.Errorf("fetched content should reflect summary, got: %s", fetchText[:min(100, len(fetchText))])
	}
}

// Simulates a Gemini CLI session.
func TestMcp_GeminiCliAgent(t *testing.T) {
	env := mcptest.NewEnv(t)
	dir, _ := filepath.Abs("testdata/gemini-cli")

	// 1. Compile the infra-api project context.
	env.CallTool(t, "MemoryCompile", map[string]any{
		"path":    dir,
		"message": "gemini-cli-init",
	})

	// 2. Agent searches for architecture decisions before modifying code.
	archResult := env.CallTool(t, "MemorySearch", map[string]any{
		"query":  "idempotent",
		"budget": 2000,
	})
	archText := env.TextContent(t, archResult)
	if !strings.Contains(strings.ToLower(archText), "idempotent") {
		t.Fatal("search should find idempotent apply semantics decision")
	}

	// 3. Agent checks for incident history before touching namespace code.
	incidentResult := env.CallTool(t, "MemorySearch", map[string]any{
		"query":  "finalizer",
		"budget": 2000,
	})
	incidentText := env.TextContent(t, incidentResult)
	if !strings.Contains(incidentText, "context.yaml") {
		t.Fatal("search should find the finalizer incident")
	}

	// 4. Agent writes a new architecture decision.
	writeResult := env.CallTool(t, "MemoryWrite", map[string]any{
		"payload": "Decision: use structured logging with slog instead of log package. Rationale: better observability in Kubernetes, JSON output for log aggregation.",
	})
	writeText := env.TextContent(t, writeResult)
	nodeID := extractNodeID(writeText)

	// 5. Agent verifies the decision is searchable.
	slogResult := env.CallTool(t, "MemorySearch", map[string]any{
		"query":  "slog",
		"budget": 1000,
	})
	slogText := env.TextContent(t, slogResult)
	if !strings.Contains(slogText, "slog") {
		t.Fatal("newly written decision should be searchable")
	}

	// 6. Agent checks history of the newly written node.
	histResult := env.CallTool(t, "MemoryHistory", map[string]any{
		"anchor": nodeID,
	})
	histText := env.TextContent(t, histResult)
	if histText == "no history" {
		t.Error("expected history for the written node")
	}

	// 7. Agent updates the decision node.
	env.CallTool(t, "MemoryWrite", map[string]any{
		"anchor":  nodeID,
		"payload": "Decision: use structured logging with slog instead of log package. Rationale: better observability. Approved by team on 2026-04-16.",
	})

	// 8. Check delta to see both writes.
	deltaResult := env.CallTool(t, "MemoryDelta", map[string]any{
		"since_snapshot": 1,
	})
	deltaText := env.TextContent(t, deltaResult)
	if deltaText == "no changes" {
		t.Error("expected delta changes after writes")
	}
}

// Simulates a Codex agent session.
func TestMcp_CodexAgent(t *testing.T) {
	env := mcptest.NewEnv(t)
	dir, _ := filepath.Abs("testdata/codex")

	// 1. Compile the data pipeline project context.
	compileResult := env.CallTool(t, "MemoryCompile", map[string]any{
		"path":    dir,
		"message": "codex-init",
	})
	text := env.TextContent(t, compileResult)
	if !strings.Contains(text, "compiled") {
		t.Fatalf("unexpected compile result: %s", text)
	}

	// 2. Agent inspects the memory tree.
	treeResult := env.CallTool(t, "MemoryTree", map[string]any{})
	treeText := env.TextContent(t, treeResult)
	if !strings.Contains(treeText, "Codex") && !strings.Contains(treeText, "Project") {
		t.Errorf("tree should contain Codex/Project headings, got: %s", treeText[:min(200, len(treeText))])
	}

	// 3. Agent searches for typing feedback before writing code.
	typingResult := env.CallTool(t, "MemorySearch", map[string]any{
		"query":  "Pydantic Protocol typing",
		"budget": 2000,
	})
	typingText := env.TextContent(t, typingResult)
	if typingText == "no results" {
		t.Fatal("expected search results for typing feedback")
	}
	if !strings.Contains(typingText, "Pydantic") {
		t.Error("typing search should surface Pydantic feedback")
	}

	// 4. Agent checks migration state before starting a new pipeline.
	migrationResult := env.CallTool(t, "MemorySearch", map[string]any{
		"query":  "ETL migration remaining blocked",
		"budget": 2000,
	})
	migrationText := env.TextContent(t, migrationResult)
	if migrationText == "no results" {
		t.Fatal("expected search results for ETL migration state")
	}

	// 5. Agent searches YAML pipeline config for vendor details.
	vendorResult := env.CallTool(t, "MemorySearch", map[string]any{
		"query":  "vendor oauth2 websocket redis",
		"budget": 1000,
	})
	vendorText := env.TextContent(t, vendorResult)
	if vendorText == "no results" {
		t.Fatal("expected search results for YAML vendor config")
	}

	// 6. Agent writes a new architecture decision.
	writeResult := env.CallTool(t, "MemoryWrite", map[string]any{
		"payload": "Decision: use polars instead of pandas for new transforms. Rationale: 5x faster on large datasets, native lazy evaluation, better memory efficiency with Apache Arrow backend.",
	})
	writeText := env.TextContent(t, writeResult)
	if !strings.Contains(writeText, "wrote node") {
		t.Fatalf("unexpected write result: %s", writeText)
	}
	nodeID := extractNodeID(writeText)

	// 7. Agent verifies the decision is searchable.
	polarsResult := env.CallTool(t, "MemorySearch", map[string]any{
		"query":  "polars",
		"budget": 1000,
	})
	polarsText := env.TextContent(t, polarsResult)
	if !strings.Contains(polarsText, "polars") {
		t.Fatal("newly written decision should be searchable")
	}

	// 8. Agent fetches context around the new decision.
	fetchResult := env.CallTool(t, "MemoryFetch", map[string]any{
		"anchor": nodeID,
		"budget": 2000,
	})
	fetchText := env.TextContent(t, fetchResult)
	if !strings.Contains(fetchText, "polars") {
		t.Errorf("fetch should include the written content, got: %s", fetchText[:min(100, len(fetchText))])
	}

	// 9. Agent checks delta since compile.
	deltaResult := env.CallTool(t, "MemoryDelta", map[string]any{
		"since_snapshot": 1,
	})
	deltaText := env.TextContent(t, deltaResult)
	if deltaText == "no changes" {
		t.Error("expected delta changes after write")
	}
}

// Simulates an OpenCode agent session.
func TestMcp_OpenCodeAgent(t *testing.T) {
	env := mcptest.NewEnv(t)
	dir, _ := filepath.Abs("testdata/opencode")

	// 1. Compile the harbor project context.
	compileResult := env.CallTool(t, "MemoryCompile", map[string]any{
		"path":    dir,
		"message": "opencode-init",
	})
	text := env.TextContent(t, compileResult)
	if !strings.Contains(text, "compiled") {
		t.Fatalf("unexpected compile result: %s", text)
	}

	// 2. Agent inspects the memory tree.
	treeResult := env.CallTool(t, "MemoryTree", map[string]any{})
	treeText := env.TextContent(t, treeResult)
	if !strings.Contains(treeText, "Harbor") && !strings.Contains(treeText, "harbor") {
		t.Errorf("tree should contain Harbor/harbor headings, got: %s", treeText[:min(200, len(treeText))])
	}

	// 3. Agent checks project migration state before picking up tantivy work.
	migrationResult := env.CallTool(t, "MemorySearch", map[string]any{
		"query":  "tantivy migration remaining blocked",
		"budget": 2000,
	})
	migrationText := env.TextContent(t, migrationResult)
	if migrationText == "no results" {
		t.Fatal("expected search results for tantivy migration state")
	}
	if !strings.Contains(migrationText, "tantivy") {
		t.Error("migration search should surface tantivy content")
	}

	// 4. Agent checks testing feedback before writing new tests.
	testingResult := env.CallTool(t, "MemorySearch", map[string]any{
		"query":  "proptest property based",
		"budget": 1000,
	})
	testingText := env.TextContent(t, testingResult)
	if testingText == "no results" {
		t.Fatal("expected search results for testing feedback")
	}
	if !strings.Contains(testingText, "proptest") {
		t.Error("testing search should surface proptest feedback")
	}

	// 5. Agent checks user preferences before responding.
	userResult := env.CallTool(t, "MemorySearch", map[string]any{
		"query":  "unified diffs terse",
		"budget": 1000,
	})
	userText := env.TextContent(t, userResult)
	if userText == "no results" {
		t.Fatal("expected search results for user preferences")
	}
	if !strings.Contains(userText, "user_preferences.md") {
		t.Error("user pref search should surface user_preferences.md")
	}

	// 6. Agent writes a new architectural decision after a design spike.
	writeResult := env.CallTool(t, "MemoryWrite", map[string]any{
		"payload": "Decision: use tokio::select! over futures::select! for the TUI event loop. Rationale: tokio::select! integrates with our existing tokio runtime, supports biased polling for predictable keybinding latency, and avoids pulling futures-util as an extra dependency.",
	})
	writeText := env.TextContent(t, writeResult)
	if !strings.Contains(writeText, "wrote node") {
		t.Fatalf("unexpected write result: %s", writeText)
	}
	nodeID := extractNodeID(writeText)

	// 7. Agent verifies the decision is searchable.
	tokioResult := env.CallTool(t, "MemorySearch", map[string]any{
		"query":  "tokio select biased polling",
		"budget": 1000,
	})
	tokioText := env.TextContent(t, tokioResult)
	if !strings.Contains(tokioText, "tokio::select") {
		t.Fatal("newly written decision should be searchable")
	}

	// 8. Agent fetches context around the new decision.
	fetchResult := env.CallTool(t, "MemoryFetch", map[string]any{
		"anchor": nodeID,
		"budget": 2000,
	})
	fetchText := env.TextContent(t, fetchResult)
	if !strings.Contains(fetchText, "tokio::select") {
		t.Errorf("fetch should include the written content, got: %s", fetchText[:min(100, len(fetchText))])
	}

	// 9. Agent summarizes a verbose node to trim session budget.
	summaryNodeID := extractFirstNodeID(treeText)
	if summaryNodeID == "" {
		t.Fatal("could not find a node ID in tree output")
	}
	env.CallTool(t, "MemorySummarize", map[string]any{
		"node_id": summaryNodeID,
		"summary": "Summarized: harbor is a Rust TUI code search engine using tantivy and tree-sitter.",
	})

	// 10. Agent checks delta since compile.
	deltaResult := env.CallTool(t, "MemoryDelta", map[string]any{
		"since_snapshot": 1,
	})
	deltaText := env.TextContent(t, deltaResult)
	if deltaText == "no changes" {
		t.Error("expected delta changes after write and summarize")
	}
}

func TestMcp_WikilinkRelationsWorkflow(t *testing.T) {
	env := mcptest.NewEnv(t)
	dir := t.TempDir()

	const (
		aSrc = "# Source\n\nSee [[Target; w=2.0]] for the design.\nUniqueMarkerSource is the anchor.\n"
		bSrc = "# Target\n\nThe target heading lives here.\n\n# Sibling\n\nA second heading.\n"
	)
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte(aSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.md"), []byte(bSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	// 1. Compile both files.
	compileResult := env.CallTool(t, "MemoryCompile", map[string]any{
		"path": dir, "message": "wikilink-init",
	})
	if !strings.Contains(env.TextContent(t, compileResult), "compiled") {
		t.Fatalf("unexpected compile result: %s", env.TextContent(t, compileResult))
	}

	// 2. Find the source paragraph in a.md via search.
	searchResult := env.CallTool(t, "MemorySearch", map[string]any{
		"query": "UniqueMarkerSource", "budget": 1000,
	})
	searchText := env.TextContent(t, searchResult)

	sourceID := extractFirstNodeID(searchText)
	if sourceID == "" {
		t.Fatalf("could not extract source node ID from search: %s", searchText)
	}

	// 3. MemoryRelated from that paragraph should surface b.md's Target heading with the authored weight.
	relatedResult := env.CallTool(t, "MemoryRelated", map[string]any{
		"anchor": sourceID, "direction": "out", "depth": 1,
	})

	relatedText := env.TextContent(t, relatedResult)
	if !strings.Contains(relatedText, "Target") {
		t.Errorf("MemoryRelated should surface Target heading, got: %s", relatedText)
	}
	if !strings.Contains(relatedText, "weight=2.00") {
		t.Errorf("MemoryRelated should report authored weight 2.0, got: %s", relatedText)
	}
	if !strings.Contains(relatedText, "hop=1") {
		t.Errorf("MemoryRelated should report hop=1 for direct edge, got: %s", relatedText)
	}

	// 4. Manually add an edge from the source paragraph to the second heading.
	relateResult := env.CallTool(t, "MemoryRelate", map[string]any{
		"source_id":    sourceID,
		"target_label": "Sibling",
		"weight":       3.0,
	})
	if !strings.Contains(env.TextContent(t, relateResult), "resolved") {
		t.Errorf("MemoryRelate should resolve to Sibling: %s", env.TextContent(t, relateResult))
	}

	// 5. Both edges should now surface via MemoryRelated.
	bothResult := env.CallTool(t, "MemoryRelated", map[string]any{
		"anchor": sourceID, "direction": "out", "depth": 1,
	})
	bothText := env.TextContent(t, bothResult)

	if !strings.Contains(bothText, "Target") {
		t.Errorf("parsed edge missing after manual add: %s", bothText)
	}
	if !strings.Contains(bothText, "Sibling") {
		t.Errorf("manual edge missing: %s", bothText)
	}

	// 6. MemoryRelate must not emit a snapshot.
	snaps, _ := env.Store.ListSnapshots(context.Background(), 10)
	if len(snaps) != 1 {
		t.Errorf("snapshot count = %d, want 1 (MemoryRelate must not snapshot)", len(snaps))
	}
}

func TestMcp_FetchBatchWorkflow(t *testing.T) {
	env := mcptest.NewEnv(t)
	dir, _ := filepath.Abs("testdata/openclaw")

	env.CallTool(t, "MemoryCompile", map[string]any{
		"path":    dir,
		"message": "fetch-batch-init",
	})

	searchResult := env.CallTool(t, "MemorySearch", map[string]any{
		"query":  "refactoring security vulnerabilities",
		"budget": 2000,
	})
	searchText := env.TextContent(t, searchResult)

	ids := extractAllNodeIDs(searchText)
	if len(ids) == 0 {
		t.Fatalf("no node IDs extracted from search output: %s", searchText)
	}

	withGhost := append([]string{}, ids...)
	withGhost = append(withGhost, "ghostid1")

	batchResult := env.CallTool(t, "MemoryFetchBatch", map[string]any{
		"node_ids": withGhost,
		"budget":   4000,
	})
	batchText := env.TextContent(t, batchResult)

	if !strings.Contains(batchText, "not found: ghostid1") {
		t.Errorf("expected `not found: ghostid1` marker, got: %s", batchText)
	}
	if strings.Count(batchText, "[heading]")+strings.Count(batchText, "[text]")+strings.Count(batchText, "[code]") == 0 {
		t.Errorf("expected at least one rendered node block, got: %s", batchText[:min(300, len(batchText))])
	}

	// Tight budget forces some IDs out — verify the over-budget marker fires.
	overResult := env.CallTool(t, "MemoryFetchBatch", map[string]any{
		"node_ids": ids,
		"budget":   1,
	})

	overText := env.TextContent(t, overResult)
	if !strings.Contains(overText, "over budget:") {
		t.Errorf("expected `over budget:` marker with budget=1, got: %s", overText[:min(300, len(overText))])
	}
}

func TestMcp_StatsWorkflow(t *testing.T) {
	env := mcptest.NewEnv(t)
	dir, _ := filepath.Abs("testdata/openclaw")

	env.CallTool(t, "MemoryCompile", map[string]any{
		"path":    dir,
		"message": "stats-init",
	})

	statsResult := env.CallTool(t, "MemoryStats", map[string]any{})
	statsText := env.TextContent(t, statsResult)

	for _, want := range []string{
		"Database:",
		"Nodes:",
		"Snapshots:",
		"Temperature:",
		"Relations:",
		"FTS rows:",
		"pinned:",
	} {
		if !strings.Contains(statsText, want) {
			t.Errorf("MemoryStats missing %q in:\n%s", want, statsText)
		}
	}

	if strings.Contains(statsText, "Nodes:              0") {
		t.Errorf("MemoryStats reports zero nodes after compile:\n%s", statsText)
	}
	if !strings.Contains(statsText, "├─") && !strings.Contains(statsText, "└─") {
		t.Errorf("MemoryStats missing tree branch glyphs:\n%s", statsText)
	}
}

func TestMcp_DiffWorkflow(t *testing.T) {
	env := mcptest.NewEnv(t)
	ctx := context.Background()

	// snap1: create nodeA at "v1".
	writeA1 := env.CallTool(t, "MemoryWrite", map[string]any{
		"payload": "Title: alpha\nbody line 1\nbody line 2\n",
	})
	nodeAID := extractNodeID(env.TextContent(t, writeA1))
	if nodeAID == "" {
		t.Fatalf("could not parse nodeA ID from write result")
	}

	// snap2: modify nodeA to v2 (changes the middle line).
	env.CallTool(t, "MemoryWrite", map[string]any{
		"anchor":  nodeAID,
		"payload": "Title: alpha\nbody line 1 EDITED\nbody line 2\n",
	})

	// snap3: modify nodeA again to v3 (also changes line 2).
	env.CallTool(t, "MemoryWrite", map[string]any{
		"anchor":  nodeAID,
		"payload": "Title: alpha\nbody line 1 EDITED\nbody line 2 EDITED TOO\n",
	})

	// snap4: create an unrelated nodeB.
	writeB := env.CallTool(t, "MemoryWrite", map[string]any{
		"payload": "Title: beta\nbeta first\nbeta second\n",
	})
	nodeBID := extractNodeID(env.TextContent(t, writeB))
	if nodeBID == "" {
		t.Fatalf("could not parse nodeB ID from write result")
	}

	snaps, err := env.Store.ListSnapshots(ctx, 10)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snaps) != 4 {
		t.Fatalf("expected 4 snapshots, got %d", len(snaps))
	}
	// snaps comes back DESC.
	snap1, snap2, snap3, snap4 := snaps[3].ID, snaps[2].ID, snaps[1].ID, snaps[0].ID

	// 1. Full history: (0, snap4]. Should consolidate to one [add] per node, showing only final state.
	fullResult := env.CallTool(t, "MemoryDiff", map[string]any{
		"from_snapshot_id": 0,
		"to_snapshot_id":   snap4,
	})
	fullText := env.TextContent(t, fullResult)

	if got := strings.Count(fullText, "[add] "+nodeAID); got != 1 {
		t.Errorf("nodeA [add] header count = %d, want 1\n%s", got, fullText)
	}
	if got := strings.Count(fullText, "[add] "+nodeBID); got != 1 {
		t.Errorf("nodeB [add] header count = %d, want 1\n%s", got, fullText)
	}
	if !strings.Contains(fullText, "+body line 1 EDITED") {
		t.Errorf("expected nodeA final state body in full diff:\n%s", fullText)
	}
	if !strings.Contains(fullText, "+beta first") {
		t.Errorf("expected nodeB body in full diff:\n%s", fullText)
	}

	// 2. Consolidation: (snap1, snap3] captures snap2 + snap3 mods on nodeA — should collapse to one [mod] showing v1 → v3.
	consolResult := env.CallTool(t, "MemoryDiff", map[string]any{
		"from_snapshot_id": snap1,
		"to_snapshot_id":   snap3,
	})
	consolText := env.TextContent(t, consolResult)

	if got := strings.Count(consolText, "[mod] "+nodeAID); got != 1 {
		t.Errorf("expected exactly 1 consolidated [mod] for nodeA, got %d\n%s", got, consolText)
	}
	for _, want := range []string{"-body line 1", "-body line 2", "+body line 1 EDITED", "+body line 2 EDITED TOO"} {
		if !strings.Contains(consolText, want) {
			t.Errorf("missing %q in consolidated mod\n%s", want, consolText)
		}
	}

	// Intermediate state (post-snap2 line 2 unedited form) must NOT survive consolidation as a + line.
	if strings.Contains(consolText, "+body line 2\n") {
		t.Errorf("intermediate snap2 state leaked into consolidated output\n%s", consolText)
	}
	// NodeB wasn't touched in this range.
	if strings.Contains(consolText, nodeBID) {
		t.Errorf("nodeB appeared in a range it was not modified in\n%s", consolText)
	}

	// 3. Tight range: (snap3, snap4] catches only nodeB's add.
	tightResult := env.CallTool(t, "MemoryDiff", map[string]any{
		"from_snapshot_id": snap3,
		"to_snapshot_id":   snap4,
	})
	tightText := env.TextContent(t, tightResult)

	if !strings.Contains(tightText, "[add] "+nodeBID) {
		t.Errorf("expected [add] for nodeB in tight range\n%s", tightText)
	}
	if strings.Contains(tightText, nodeAID) {
		t.Errorf("nodeA leaked into tight range it wasn't touched in\n%s", tightText)
	}

	// 4. Equal bounds: empty range → "no changes".
	equalResult := env.CallTool(t, "MemoryDiff", map[string]any{
		"from_snapshot_id": snap2,
		"to_snapshot_id":   snap2,
	})
	equalText := env.TextContent(t, equalResult)
	if equalText != "no changes" {
		t.Errorf("equal bounds got %q, want %q", equalText, "no changes")
	}

	// 5. Header-only line should remain as context (` ` prefix) since it didn't change.
	if !strings.Contains(consolText, " Title: alpha") {
		t.Errorf("expected unchanged header to appear as context line in consolidated mod\n%s", consolText)
	}

	// 6. Validation: from > to surfaces as an MCP error.
	badResult, err := env.Session.CallTool(ctx, &gomcp.CallToolParams{
		Name: "MemoryDiff",
		Arguments: map[string]any{
			"from_snapshot_id": snap4,
			"to_snapshot_id":   snap1,
		},
	})
	if err != nil {
		t.Fatalf("validation call failed unexpectedly: %v", err)
	}

	if !badResult.IsError {
		t.Errorf("expected IsError=true for from > to, got %#v", badResult)
	}
	if badResult.IsError && len(badResult.Content) > 0 {
		errText := env.TextContent(t, badResult)
		if !strings.Contains(errText, "must be <=") {
			t.Errorf("error text missing validation phrase: %q", errText)
		}
	}
}

func TestMcp_RollbackWorkflow_DropAfterFalse(t *testing.T) {
	env := mcptest.NewEnv(t)
	ctx := context.Background()

	writeA := env.CallTool(t, "MemoryWrite", map[string]any{
		"payload": "Title alpha\nfirst version\n",
	})
	nodeAID := extractNodeID(env.TextContent(t, writeA))
	if nodeAID == "" {
		t.Fatalf("could not parse node id from first write")
	}

	env.CallTool(t, "MemoryWrite", map[string]any{
		"payload": "Title beta\nsecond version\n",
	})

	snapsBefore, err := env.Store.ListSnapshots(ctx, 10)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snapsBefore) != 2 {
		t.Fatalf("expected 2 snapshots before rollback, got %d", len(snapsBefore))
	}

	snap1 := snapsBefore[1].ID
	snap2 := snapsBefore[0].ID

	rollbackResult := env.CallTool(t, "MemoryRollback", map[string]any{
		"snapshot_id": snap1,
	})
	if !strings.Contains(env.TextContent(t, rollbackResult), "rolled back to snapshot") {
		t.Errorf("unexpected rollback result: %s", env.TextContent(t, rollbackResult))
	}

	snapsAfter, err := env.Store.ListSnapshots(ctx, 10)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snapsAfter) != 3 {
		t.Fatalf("expected 3 snapshots after non-pruning rollback, got %d", len(snapsAfter))
	}

	rollbackSnap := snapsAfter[0]
	if !strings.HasPrefix(rollbackSnap.Message, "rollback to ") {
		t.Errorf("HEAD message = %q, want rollback prefix", rollbackSnap.Message)
	}
	if !rollbackSnap.ParentID.Valid || rollbackSnap.ParentID.Int64 != snap2 {
		t.Errorf("rollback snap parent = %v, want %d (prev HEAD)", rollbackSnap.ParentID, snap2)
	}

	// nodeA still alive, nodeB gone.
	if _, err := env.Store.GetNode(ctx, nodeAID); err != nil {
		t.Errorf("nodeA missing after rollback: %v", err)
	}

	// Next write chains on the rollback snap.
	env.CallTool(t, "MemoryWrite", map[string]any{
		"payload": "Title delta\npost-rollback note\n",
	})

	snapsPost, err := env.Store.ListSnapshots(ctx, 1)
	if err != nil || len(snapsPost) == 0 {
		t.Fatalf("ListSnapshots after post-rollback write: %v", err)
	}

	if !snapsPost[0].ParentID.Valid || snapsPost[0].ParentID.Int64 != rollbackSnap.ID {
		t.Errorf("post-rollback write parent = %v, want %d (rollback snap)", snapsPost[0].ParentID, rollbackSnap.ID)
	}
}

func TestMcp_RollbackWorkflow_DropAfterTrue(t *testing.T) {
	env := mcptest.NewEnv(t)
	ctx := context.Background()

	env.CallTool(t, "MemoryWrite", map[string]any{"payload": "Title one\nv1\n"})
	env.CallTool(t, "MemoryWrite", map[string]any{"payload": "Title two\nv2\n"})
	env.CallTool(t, "MemoryWrite", map[string]any{"payload": "Title three\nv3\n"})

	snapsBefore, err := env.Store.ListSnapshots(ctx, 10)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}

	if len(snapsBefore) != 3 {
		t.Fatalf("expected 3 snapshots before rollback, got %d", len(snapsBefore))
	}
	snap1 := snapsBefore[2].ID

	env.CallTool(t, "MemoryRollback", map[string]any{
		"snapshot_id": snap1,
		"drop_after":  true,
	})

	snapsAfter, err := env.Store.ListSnapshots(ctx, 10)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snapsAfter) != 2 {
		t.Errorf("expected 2 snapshots after pruning rollback (target + rollback), got %d", len(snapsAfter))
	}

	rollbackSnap := snapsAfter[0]
	if !rollbackSnap.ParentID.Valid || rollbackSnap.ParentID.Int64 != snap1 {
		t.Errorf("rollback snap parent = %v, want %d (drop_after=true linearizes to target)", rollbackSnap.ParentID, snap1)
	}
}

// Verifies that the client can list all available tools.
func TestMcp_ToolDiscovery(t *testing.T) {
	env := mcptest.NewEnv(t)

	tools, err := env.Session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	expected := map[string]bool{
		"MemoryFetch":      false,
		"MemoryFetchBatch": false,
		"MemorySearch":     false,
		"MemoryWrite":      false,
		"MemoryCompile":    false,
		"MemoryDelta":      false,
		"MemorySummarize":  false,
		"MemoryHistory":    false,
		"MemoryTree":       false,
		"MemoryRelated":    false,
		"MemoryRelate":     false,
		"MemoryPin":        false,
		"MemoryUnpin":      false,
		"MemoryStats":      false,
		"MemoryRollback":   false,
	}

	for _, tool := range tools.Tools {
		if _, ok := expected[tool.Name]; ok {
			expected[tool.Name] = true
		}
	}

	for name, found := range expected {
		if !found {
			t.Errorf("tool %q not found in ListTools response", name)
		}
	}
}

func TestMcp_OverviewResource(t *testing.T) {
	env := mcptest.NewEnv(t)
	ctx := context.Background()

	// Seed a node + snapshot so the envelope carries non-zero counts.
	writeResult := env.CallTool(t, "MemoryWrite", map[string]any{
		"payload": "Overview resource smoke content.",
	})
	if !strings.Contains(env.TextContent(t, writeResult), "wrote node") {
		t.Fatalf("seed write failed: %s", env.TextContent(t, writeResult))
	}

	listed, err := env.Session.ListResources(ctx, &gomcp.ListResourcesParams{})
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}

	var overview *gomcp.Resource
	for _, r := range listed.Resources {
		if r.URI == "remindb://overview" {
			overview = r
		}
	}
	if overview == nil {
		t.Fatalf("resources/list missing remindb://overview, got %d resources", len(listed.Resources))
	}
	if overview.MIMEType != "application/json" {
		t.Errorf("overview MIME type = %q, want application/json", overview.MIMEType)
	}

	read, err := env.Session.ReadResource(ctx, &gomcp.ReadResourceParams{URI: "remindb://overview"})
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if len(read.Contents) != 1 {
		t.Fatalf("ReadResource returned %d contents, want 1", len(read.Contents))
	}

	content := read.Contents[0]
	if content.MIMEType != "application/json" {
		t.Errorf("content MIME type = %q, want application/json", content.MIMEType)
	}
	if content.URI != "remindb://overview" {
		t.Errorf("content URI = %q, want remindb://overview", content.URI)
	}

	var env2 struct {
		DBPath string `json:"db_path"`
		Nodes  struct {
			Total  int            `json:"total"`
			ByType map[string]int `json:"by_type"`
			Tokens int64          `json:"tokens"`
		} `json:"nodes"`
		Snapshots struct {
			Count  int   `json:"count"`
			HeadID int64 `json:"head_id"`
		} `json:"snapshots"`
		Temperature struct {
			Avg float64 `json:"avg"`
		} `json:"temperature"`
		Relations struct {
			ByOrigin map[string]int `json:"by_origin"`
			Pending  int            `json:"pending"`
		} `json:"relations"`
		FTSRows int `json:"fts_rows"`
	}
	if err := json.Unmarshal([]byte(content.Text), &env2); err != nil {
		t.Fatalf("overview JSON not parseable: %v\nbody: %s", err, content.Text)
	}

	if env2.Nodes.Total < 1 {
		t.Errorf("nodes.total = %d, want >= 1 after a seeded write", env2.Nodes.Total)
	}
	if env2.Snapshots.Count < 1 || env2.Snapshots.HeadID < 1 {
		t.Errorf("snapshots = %+v, want count>=1 and head_id>=1", env2.Snapshots)
	}
}

func TestMcp_FilesResource(t *testing.T) {
	env := mcptest.NewEnv(t)
	ctx := context.Background()

	// A compiled dir → files grouped under a non-empty compile root.
	dir, _ := filepath.Abs("testdata/openclaw")
	compileResult := env.CallTool(t, "MemoryCompile", map[string]any{
		"path":    dir,
		"message": "files-resource-init",
	})
	if !strings.Contains(env.TextContent(t, compileResult), "compiled") {
		t.Fatalf("seed compile failed: %s", env.TextContent(t, compileResult))
	}

	// A freeform write → a file with no compile root (ungrouped bucket).
	writeResult := env.CallTool(t, "MemoryWrite", map[string]any{
		"payload": "Files resource ungrouped smoke content.",
	})
	if !strings.Contains(env.TextContent(t, writeResult), "wrote node") {
		t.Fatalf("seed write failed: %s", env.TextContent(t, writeResult))
	}

	listed, err := env.Session.ListResources(ctx, &gomcp.ListResourcesParams{})
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}

	var files *gomcp.Resource
	for _, r := range listed.Resources {
		if r.URI == "remindb://files" {
			files = r
		}
	}
	if files == nil {
		t.Fatalf("resources/list missing remindb://files, got %d resources", len(listed.Resources))
	}
	if files.MIMEType != "application/json" {
		t.Errorf("files MIME type = %q, want application/json", files.MIMEType)
	}

	read, err := env.Session.ReadResource(ctx, &gomcp.ReadResourceParams{URI: "remindb://files"})
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if len(read.Contents) != 1 {
		t.Fatalf("ReadResource returned %d contents, want 1", len(read.Contents))
	}

	content := read.Contents[0]
	if content.MIMEType != "application/json" {
		t.Errorf("content MIME type = %q, want application/json", content.MIMEType)
	}
	if content.URI != "remindb://files" {
		t.Errorf("content URI = %q, want remindb://files", content.URI)
	}

	var env2 struct {
		Roots []struct {
			Root  string `json:"root"`
			Files []struct {
				Path   string `json:"path"`
				Nodes  int    `json:"nodes"`
				Tokens int    `json:"tokens"`
			} `json:"files"`
		} `json:"roots"`
	}
	if err := json.Unmarshal([]byte(content.Text), &env2); err != nil {
		t.Fatalf("files JSON not parseable: %v\nbody: %s", err, content.Text)
	}

	if len(env2.Roots) < 2 {
		t.Fatalf("roots = %d, want >= 2 (one compiled root + ungrouped)", len(env2.Roots))
	}

	// Roots sort ascending; the empty-string ("ungrouped") root sorts last.
	if got := env2.Roots[len(env2.Roots)-1].Root; got != "" {
		t.Errorf("last root = %q, want %q (ungrouped sorts last)", got, "")
	}
	for i := 1; i < len(env2.Roots)-1; i++ {
		if env2.Roots[i-1].Root > env2.Roots[i].Root {
			t.Errorf("roots not sorted ascending: %q before %q", env2.Roots[i-1].Root, env2.Roots[i].Root)
		}
	}

	sawCompiled := false
	for _, rg := range env2.Roots {
		for _, f := range rg.Files {
			if f.Path == "" {
				t.Errorf("file with empty path in root %q", rg.Root)
			}
			if f.Nodes < 1 || f.Tokens < 1 {
				t.Errorf("file %q: nodes=%d tokens=%d, want both >= 1", f.Path, f.Nodes, f.Tokens)
			}
		}
		if rg.Root == dir && len(rg.Files) > 0 {
			sawCompiled = true
		}
	}
	if !sawCompiled {
		t.Errorf("no file group under compiled root %q; roots=%+v", dir, env2.Roots)
	}
}

type treeNodeJSON struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	Label       string          `json:"label"`
	Depth       int             `json:"depth"`
	Tokens      int             `json:"tokens"`
	Temperature float64         `json:"temperature"`
	Source      string          `json:"source"`
	Children    []*treeNodeJSON `json:"children"`
}

type treeEnvJSON struct {
	Roots []*treeNodeJSON `json:"roots"`
}

func TestMcp_TreeResource(t *testing.T) {
	env := mcptest.NewEnv(t)
	ctx := context.Background()

	dir, _ := filepath.Abs("testdata/openclaw")
	compileResult := env.CallTool(t, "MemoryCompile", map[string]any{
		"path":    dir,
		"message": "tree-resource-init",
	})
	if !strings.Contains(env.TextContent(t, compileResult), "compiled") {
		t.Fatalf("seed compile failed: %s", env.TextContent(t, compileResult))
	}

	listed, err := env.Session.ListResources(ctx, &gomcp.ListResourcesParams{})
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	var tree *gomcp.Resource
	for _, r := range listed.Resources {
		if r.URI == "remindb://tree" {
			tree = r
		}
	}
	if tree == nil {
		t.Fatalf("resources/list missing remindb://tree, got %d resources", len(listed.Resources))
	}
	if tree.MIMEType != "application/json" {
		t.Errorf("tree MIME type = %q, want application/json", tree.MIMEType)
	}

	read, err := env.Session.ReadResource(ctx, &gomcp.ReadResourceParams{URI: "remindb://tree"})
	if err != nil {
		t.Fatalf("ReadResource(tree): %v", err)
	}
	if len(read.Contents) != 1 {
		t.Fatalf("ReadResource returned %d contents, want 1", len(read.Contents))
	}
	if read.Contents[0].URI != "remindb://tree" {
		t.Errorf("content URI = %q, want remindb://tree", read.Contents[0].URI)
	}

	var full treeEnvJSON
	if err := json.Unmarshal([]byte(read.Contents[0].Text), &full); err != nil {
		t.Fatalf("tree JSON not parseable: %v\nbody: %s", err, read.Contents[0].Text)
	}
	if len(full.Roots) == 0 {
		t.Fatalf("full tree has no roots")
	}

	// Find a node two levels deep so depth-bounding is observable: it must have a child that itself has children.
	var pivot *treeNodeJSON
	var find func(n *treeNodeJSON)

	find = func(n *treeNodeJSON) {
		if pivot != nil {
			return
		}

		for _, c := range n.Children {
			if len(c.Children) > 0 {
				pivot = n
				return
			}
		}

		for _, c := range n.Children {
			find(c)
		}
	}
	for _, r := range full.Roots {
		find(r)
	}
	if pivot == nil {
		t.Fatalf("no node with a grandchild found; cannot assert depth bounding")
	}

	// Shape: every node carries the full field set.
	if pivot.ID == "" || pivot.Type == "" || pivot.Source == "" {
		t.Errorf("pivot missing required fields: %+v", pivot)
	}

	uri := "remindb://tree/" + pivot.ID + "?depth=1"
	bounded, err := env.Session.ReadResource(ctx, &gomcp.ReadResourceParams{URI: uri})
	if err != nil {
		t.Fatalf("ReadResource(%s): %v", uri, err)
	}
	if bounded.Contents[0].URI != uri {
		t.Errorf("content URI = %q, want %q", bounded.Contents[0].URI, uri)
	}

	var sub treeEnvJSON
	if err := json.Unmarshal([]byte(bounded.Contents[0].Text), &sub); err != nil {
		t.Fatalf("bounded tree JSON not parseable: %v\nbody: %s", err, bounded.Contents[0].Text)
	}
	if len(sub.Roots) != 1 {
		t.Fatalf("bounded read roots = %d, want 1", len(sub.Roots))
	}

	root := sub.Roots[0]
	if root.ID != pivot.ID {
		t.Errorf("bounded root id = %q, want %q", root.ID, pivot.ID)
	}
	if len(root.Children) == 0 {
		t.Fatalf("depth=1 should include the first child level; got none")
	}

	for _, c := range root.Children {
		if len(c.Children) != 0 {
			t.Errorf("depth=1 not bounded: child %q has %d grandchildren", c.ID, len(c.Children))
		}
	}

	// Unknown root → error.
	if _, err := env.Session.ReadResource(ctx, &gomcp.ReadResourceParams{URI: "remindb://tree/does-not-exist"}); err == nil {
		t.Errorf("ReadResource for unknown root: want error, got nil")
	}
}

type snapshotEntryJSON struct {
	ID          int64  `json:"id"`
	ParentID    *int64 `json:"parent_id"`
	Message     string `json:"message"`
	CompileRoot string `json:"compile_root"`
	CreatedAt   int64  `json:"created_at"`
	IsHead      bool   `json:"is_head"`
}

type snapshotsEnvJSON struct {
	Snapshots []snapshotEntryJSON `json:"snapshots"`
}

type snapshotDiffsEnvJSON struct {
	SnapshotID int64 `json:"snapshot_id"`
	Diffs      []struct {
		Op         string `json:"op"`
		NodeID     string `json:"node_id"`
		OldHash    string `json:"old_hash"`
		NewHash    string `json:"new_hash"`
		OldContent string `json:"old_content"`
		NewContent string `json:"new_content"`
	} `json:"diffs"`
}

func TestMcp_SnapshotsResource(t *testing.T) {
	env := mcptest.NewEnv(t)
	ctx := context.Background()

	// Seed a chain: compile (snap1) → write (snap2) → write (snap3, HEAD).
	dir, _ := filepath.Abs("testdata/openclaw")
	compileResult := env.CallTool(t, "MemoryCompile", map[string]any{
		"path":    dir,
		"message": "snapshots-resource-init",
	})
	if !strings.Contains(env.TextContent(t, compileResult), "compiled") {
		t.Fatalf("seed compile failed: %s", env.TextContent(t, compileResult))
	}

	env.CallTool(t, "MemoryWrite", map[string]any{"payload": "Snapshots resource note one."})
	env.CallTool(t, "MemoryWrite", map[string]any{"payload": "Snapshots resource note two."})

	listed, err := env.Session.ListResources(ctx, &gomcp.ListResourcesParams{})
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}

	var snapshots *gomcp.Resource
	for _, r := range listed.Resources {
		if r.URI == "remindb://snapshots" {
			snapshots = r
		}
	}
	if snapshots == nil {
		t.Fatalf("resources/list missing remindb://snapshots, got %d resources", len(listed.Resources))
	}
	if snapshots.MIMEType != "application/json" {
		t.Errorf("snapshots MIME type = %q, want application/json", snapshots.MIMEType)
	}

	// Full history: ordered newest-first, parent chain intact, exactly one HEAD.
	read, err := env.Session.ReadResource(ctx, &gomcp.ReadResourceParams{URI: "remindb://snapshots"})
	if err != nil {
		t.Fatalf("ReadResource(snapshots): %v", err)
	}
	if len(read.Contents) != 1 {
		t.Fatalf("ReadResource returned %d contents, want 1", len(read.Contents))
	}
	if read.Contents[0].URI != "remindb://snapshots" {
		t.Errorf("content URI = %q, want remindb://snapshots", read.Contents[0].URI)
	}

	var full snapshotsEnvJSON
	if err := json.Unmarshal([]byte(read.Contents[0].Text), &full); err != nil {
		t.Fatalf("snapshots JSON not parseable: %v\nbody: %s", err, read.Contents[0].Text)
	}
	if len(full.Snapshots) < 3 {
		t.Fatalf("snapshots = %d, want >= 3 (compile + 2 writes)", len(full.Snapshots))
	}

	for i := 1; i < len(full.Snapshots); i++ {
		if full.Snapshots[i-1].ID <= full.Snapshots[i].ID {
			t.Errorf("snapshots not newest-first: id %d before %d", full.Snapshots[i-1].ID, full.Snapshots[i].ID)
		}
	}

	oldest := full.Snapshots[len(full.Snapshots)-1]
	if oldest.ParentID != nil {
		t.Errorf("oldest snapshot parent_id = %d, want null (root has no parent)", *oldest.ParentID)
	}

	for i := 0; i < len(full.Snapshots)-1; i++ {
		s := full.Snapshots[i]
		if s.ParentID == nil || *s.ParentID != full.Snapshots[i+1].ID {
			t.Errorf("snapshot %d parent_id = %v, want %d (chains to previous)", s.ID, s.ParentID, full.Snapshots[i+1].ID)
		}
	}

	headCount := 0
	for _, s := range full.Snapshots {
		if s.IsHead {
			headCount++
		}
	}
	if headCount != 1 {
		t.Errorf("is_head count = %d, want exactly 1", headCount)
	}
	if !full.Snapshots[0].IsHead {
		t.Errorf("newest snapshot %d is_head=false, want true (HEAD is the latest write)", full.Snapshots[0].ID)
	}

	// Templated ?limit bounds to the newest N.
	limited, err := env.Session.ReadResource(ctx, &gomcp.ReadResourceParams{URI: "remindb://snapshots?limit=1"})
	if err != nil {
		t.Fatalf("ReadResource(snapshots?limit=1): %v", err)
	}
	if limited.Contents[0].URI != "remindb://snapshots?limit=1" {
		t.Errorf("content URI = %q, want remindb://snapshots?limit=1", limited.Contents[0].URI)
	}

	var bounded snapshotsEnvJSON
	if err := json.Unmarshal([]byte(limited.Contents[0].Text), &bounded); err != nil {
		t.Fatalf("bounded snapshots JSON not parseable: %v\nbody: %s", err, limited.Contents[0].Text)
	}
	if len(bounded.Snapshots) != 1 {
		t.Fatalf("limit=1 returned %d snapshots, want 1", len(bounded.Snapshots))
	}
	if bounded.Snapshots[0].ID != full.Snapshots[0].ID || !bounded.Snapshots[0].IsHead {
		t.Errorf("limit=1 snapshot = %+v, want the HEAD (%d)", bounded.Snapshots[0], full.Snapshots[0].ID)
	}

	// Templated per-snapshot diffs for the HEAD write.
	headID := full.Snapshots[0].ID
	diffURI := "remindb://snapshots/" + strconv.FormatInt(headID, 10) + "/diffs"
	diffRead, err := env.Session.ReadResource(ctx, &gomcp.ReadResourceParams{URI: diffURI})
	if err != nil {
		t.Fatalf("ReadResource(%s): %v", diffURI, err)
	}
	if diffRead.Contents[0].URI != diffURI {
		t.Errorf("content URI = %q, want %q", diffRead.Contents[0].URI, diffURI)
	}

	var diffs snapshotDiffsEnvJSON
	if err := json.Unmarshal([]byte(diffRead.Contents[0].Text), &diffs); err != nil {
		t.Fatalf("snapshot diffs JSON not parseable: %v\nbody: %s", err, diffRead.Contents[0].Text)
	}

	if diffs.SnapshotID != headID {
		t.Errorf("snapshot_id = %d, want %d", diffs.SnapshotID, headID)
	}
	if len(diffs.Diffs) == 0 {
		t.Fatalf("HEAD write produced no diff records")
	}
	for _, d := range diffs.Diffs {
		if d.Op == "" || d.NodeID == "" {
			t.Errorf("diff missing op/node_id: %+v", d)
		}
	}

	// A non-numeric snapshot id is an error, not an empty body.
	if _, err := env.Session.ReadResource(ctx, &gomcp.ReadResourceParams{URI: "remindb://snapshots/not-an-int/diffs"}); err == nil {
		t.Errorf("ReadResource for bad snapshot id: want error, got nil")
	}
}

// Pulls every "id=XXXXXXXXXXX" occurrence out of a Format/FormatCompact output.
func extractAllNodeIDs(s string) []string {
	var out []string
	rest := s
	for {
		_, after, ok := strings.Cut(rest, "id=")
		if !ok {
			return out
		}

		end := strings.IndexAny(after, " )\n")
		if end <= 0 {
			return out
		}

		out = append(out, after[:end])
		rest = after[end:]
	}
}

// Parses "wrote node XXXXXXXX (N tokens)" to get the node ID.
func extractNodeID(s string) string {
	_, rest, ok := strings.Cut(s, "wrote node ")
	if !ok {
		return ""
	}

	if j := strings.IndexByte(rest, ' '); j > 0 {
		return rest[:j]
	}
	return rest
}

// Finds the first node ID in tree output like "(id=XXXXXXXX".
func extractFirstNodeID(tree string) string {
	_, rest, ok := strings.Cut(tree, "(id=")
	if !ok {
		// Try the other format: "(XXXXXXXX)"
		return extractFirstParenID(tree)
	}

	if j := strings.IndexAny(rest, " )"); j > 0 {
		return rest[:j]
	}
	return ""
}

func extractFirstParenID(tree string) string {
	// Tree format: "[type] label (XXXXXXXX)"
	_, rest, ok := strings.Cut(tree, "(")
	if !ok {
		return ""
	}

	if j := strings.IndexByte(rest, ')'); j > 0 {
		id := rest[:j]

		// Validate it looks like an ID (8 alphanumeric chars).
		if len(id) >= 6 && !strings.Contains(id, " ") {
			return id
		}
	}
	return ""
}

func TestMcp_TemperatureResource(t *testing.T) {
	env := mcptest.NewEnv(t)
	ctx := context.Background()

	// Seed nodes so the heatmap carries a non-empty unified array.
	writeResult := env.CallTool(t, "MemoryWrite", map[string]any{
		"payload": "Temperature resource smoke content.",
	})
	if !strings.Contains(env.TextContent(t, writeResult), "wrote node") {
		t.Fatalf("seed write failed: %s", env.TextContent(t, writeResult))
	}

	listed, err := env.Session.ListResources(ctx, &gomcp.ListResourcesParams{})
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}

	var heat *gomcp.Resource
	for _, r := range listed.Resources {
		if r.URI == "remindb://temperature" {
			heat = r
		}
	}
	if heat == nil {
		t.Fatalf("resources/list missing remindb://temperature, got %d resources", len(listed.Resources))
	}
	if heat.MIMEType != "application/json" {
		t.Errorf("temperature MIME type = %q, want application/json", heat.MIMEType)
	}

	read, err := env.Session.ReadResource(ctx, &gomcp.ReadResourceParams{URI: "remindb://temperature"})
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if len(read.Contents) != 1 {
		t.Fatalf("ReadResource returned %d contents, want 1", len(read.Contents))
	}

	content := read.Contents[0]
	if content.URI != "remindb://temperature" {
		t.Errorf("content URI = %q, want remindb://temperature", content.URI)
	}

	var env2 struct {
		Summary struct {
			Hot           int     `json:"hot"`
			Cold          int     `json:"cold"`
			Pinned        int     `json:"pinned"`
			ColdThreshold float64 `json:"cold_threshold"`
			HotThreshold  float64 `json:"hot_threshold"`
		} `json:"summary"`
		Nodes []struct {
			ID          string  `json:"id"`
			Temperature float64 `json:"temperature"`
			Pinned      bool    `json:"pinned"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal([]byte(content.Text), &env2); err != nil {
		t.Fatalf("temperature JSON not parseable: %v\nbody: %s", err, content.Text)
	}

	if len(env2.Nodes) < 1 {
		t.Fatalf("nodes = %d, want >= 1 after a seeded write (unified array)", len(env2.Nodes))
	}
	// Default config (mcptest) → cold 0.1, fixed hot 0.5; both echoed.
	if env2.Summary.ColdThreshold != 0.1 || env2.Summary.HotThreshold != 0.5 {
		t.Errorf("thresholds = cold %v hot %v, want 0.1 / 0.5", env2.Summary.ColdThreshold, env2.Summary.HotThreshold)
	}

	// Summary must be derivable from the same nodes array — one fetch, no drift.
	var wantHot, wantCold, wantPinned int
	for _, n := range env2.Nodes {
		if n.Temperature >= env2.Summary.HotThreshold {
			wantHot++
		}
		if n.Temperature < env2.Summary.ColdThreshold {
			wantCold++
		}
		if n.Pinned {
			wantPinned++
		}
	}
	if env2.Summary.Hot != wantHot || env2.Summary.Cold != wantCold || env2.Summary.Pinned != wantPinned {
		t.Errorf("summary {hot:%d cold:%d pinned:%d} disagrees with nodes {hot:%d cold:%d pinned:%d}",
			env2.Summary.Hot, env2.Summary.Cold, env2.Summary.Pinned, wantHot, wantCold, wantPinned)
	}
}

func TestMcp_DoctorResource(t *testing.T) {
	env := mcptest.NewEnv(t)
	ctx := context.Background()

	// A seeded write keeps the DB healthy → every check passes.
	writeResult := env.CallTool(t, "MemoryWrite", map[string]any{
		"payload": "Doctor resource smoke content.",
	})
	if !strings.Contains(env.TextContent(t, writeResult), "wrote node") {
		t.Fatalf("seed write failed: %s", env.TextContent(t, writeResult))
	}

	listed, err := env.Session.ListResources(ctx, &gomcp.ListResourcesParams{})
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}

	var doctor *gomcp.Resource
	for _, r := range listed.Resources {
		if r.URI == "remindb://doctor" {
			doctor = r
		}
	}
	if doctor == nil {
		t.Fatalf("resources/list missing remindb://doctor, got %d resources", len(listed.Resources))
	}
	if doctor.MIMEType != "application/json" {
		t.Errorf("doctor MIME type = %q, want application/json", doctor.MIMEType)
	}

	read, err := env.Session.ReadResource(ctx, &gomcp.ReadResourceParams{URI: "remindb://doctor"})
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if len(read.Contents) != 1 {
		t.Fatalf("ReadResource returned %d contents, want 1", len(read.Contents))
	}

	content := read.Contents[0]
	if content.MIMEType != "application/json" {
		t.Errorf("content MIME type = %q, want application/json", content.MIMEType)
	}
	if content.URI != "remindb://doctor" {
		t.Errorf("content URI = %q, want remindb://doctor", content.URI)
	}

	var env2 struct {
		Status string `json:"status"`
		Checks []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Detail string `json:"detail"`
		} `json:"checks"`
	}
	if err := json.Unmarshal([]byte(content.Text), &env2); err != nil {
		t.Fatalf("doctor JSON not parseable: %v\nbody: %s", err, content.Text)
	}

	if env2.Status != "pass" {
		t.Errorf("status = %q, want pass on a healthy seeded DB", env2.Status)
	}
	if len(env2.Checks) == 0 {
		t.Fatalf("checks is empty; want the full doctor check set")
	}
	for _, c := range env2.Checks {
		if c.Name == "" || c.Status == "" {
			t.Errorf("check missing name/status: %+v", c)
		}
	}
}

func TestMcp_LogsResource(t *testing.T) {
	env := mcptest.NewEnvWithLog(t)
	ctx := context.Background()

	writeResult := env.CallTool(t, "MemoryWrite", map[string]any{
		"payload": "Logs resource smoke content.",
	})
	if !strings.Contains(env.TextContent(t, writeResult), "wrote node") {
		t.Fatalf("seed write failed: %s", env.TextContent(t, writeResult))
	}

	listed, err := env.Session.ListResources(ctx, &gomcp.ListResourcesParams{})
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}

	var logs *gomcp.Resource
	for _, r := range listed.Resources {
		if r.URI == "remindb://logs" {
			logs = r
		}
	}
	if logs == nil {
		t.Fatalf("resources/list missing remindb://logs, got %d resources", len(listed.Resources))
	}
	if logs.MIMEType != "application/json" {
		t.Errorf("logs MIME type = %q, want application/json", logs.MIMEType)
	}

	read, err := env.Session.ReadResource(ctx, &gomcp.ReadResourceParams{URI: "remindb://logs"})
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if len(read.Contents) != 1 {
		t.Fatalf("ReadResource returned %d contents, want 1", len(read.Contents))
	}

	content := read.Contents[0]
	if content.MIMEType != "application/json" {
		t.Errorf("content MIME type = %q, want application/json", content.MIMEType)
	}
	if content.URI != "remindb://logs" {
		t.Errorf("content URI = %q, want remindb://logs", content.URI)
	}

	var envelope struct {
		Records []struct {
			Time  int64          `json:"time"`
			Level string         `json:"level"`
			Msg   string         `json:"msg"`
			Attrs map[string]any `json:"attrs"`
		} `json:"records"`
		Dropped int64 `json:"dropped"`
	}
	if err := json.Unmarshal([]byte(content.Text), &envelope); err != nil {
		t.Fatalf("logs JSON not parseable: %v\nbody: %s", err, content.Text)
	}

	if len(envelope.Records) == 0 {
		t.Fatalf("records is empty; want the MemoryWrite tool-call trace captured")
	}
	if envelope.Dropped < 0 {
		t.Errorf("dropped = %d, want >= 0", envelope.Dropped)
	}

	var sawToolCall bool
	for _, r := range envelope.Records {
		if r.Time <= 0 || r.Level == "" || r.Attrs == nil {
			t.Errorf("malformed record: %+v", r)
		}

		if r.Msg == "mcp call" && r.Attrs["tool"] == "MemoryWrite" {
			sawToolCall = true
		}
	}

	if !sawToolCall {
		t.Errorf("no \"mcp call\" record with tool=MemoryWrite; tool-call logs not reaching the resource")
	}
}

func TestMcp_MemoryForget(t *testing.T) {
	t.Run("Strict_Leaf", func(t *testing.T) {
		env := mcptest.NewEnv(t)
		seedForgetNode(t, env, "rootroor", "")
		seedForgetNode(t, env, "leaf0001", "rootroor")

		result := env.CallTool(t, "MemoryForget", map[string]any{
			"node_id": "leaf0001",
		})
		text := env.TextContent(t, result)
		if !strings.Contains(text, "forgot node leaf0001") {
			t.Fatalf("unexpected result: %s", text)
		}
		if !strings.Contains(text, "mode=strict") {
			t.Errorf("result should mention mode=strict: %s", text)
		}

		treeText := env.TextContent(t, env.CallTool(t, "MemoryTree", map[string]any{}))
		if strings.Contains(treeText, "id=leaf0001") {
			t.Errorf("tree still contains leaf0001: %s", treeText)
		}
		if !strings.Contains(treeText, "id=rootroor") {
			t.Errorf("tree should still contain rootroor: %s", treeText)
		}

		deltaText := env.TextContent(t, env.CallTool(t, "MemoryDelta", map[string]any{
			"since_snapshot": 0,
		}))
		if !strings.Contains(deltaText, "[rem] leaf0001") {
			t.Errorf("delta should show [rem] leaf0001: %s", deltaText)
		}
	})

	t.Run("Strict_RejectsParent", func(t *testing.T) {
		env := mcptest.NewEnv(t)
		seedForgetNode(t, env, "rootroor", "")
		seedForgetNode(t, env, "child001", "rootroor")

		result, err := env.Session.CallTool(context.Background(), &gomcp.CallToolParams{
			Name:      "MemoryForget",
			Arguments: map[string]any{"node_id": "rootroor", "mode": "strict"},
		})
		if err != nil {
			t.Fatalf("CallTool transport: %v", err)
		}
		if !result.IsError {
			t.Fatalf("expected IsError=true, got success: %v", result)
		}

		msg := env.TextContent(t, result)
		if !strings.Contains(msg, "has 1 children") {
			t.Errorf("error should mention children count: %s", msg)
		}

		treeText := env.TextContent(t, env.CallTool(t, "MemoryTree", map[string]any{}))
		for _, id := range []string{"rootroor", "child001"} {
			if !strings.Contains(treeText, "id="+id) {
				t.Errorf("tree should still contain %s after rejected forget: %s", id, treeText)
			}
		}
	})

	t.Run("Cascade_Subtree", func(t *testing.T) {
		env := mcptest.NewEnv(t)
		seedForgetNode(t, env, "rootroor", "")
		seedForgetNode(t, env, "mid00001", "rootroor")
		seedForgetNode(t, env, "leaf0001", "mid00001")
		seedForgetNode(t, env, "leaf0002", "mid00001")

		result := env.CallTool(t, "MemoryForget", map[string]any{
			"node_id": "mid00001",
			"mode":    "cascade",
		})
		text := env.TextContent(t, result)
		if !strings.Contains(text, "3 affected") {
			t.Errorf("expected 3 affected (mid + 2 leaves): %s", text)
		}

		treeText := env.TextContent(t, env.CallTool(t, "MemoryTree", map[string]any{}))
		for _, id := range []string{"mid00001", "leaf0001", "leaf0002"} {
			if strings.Contains(treeText, "id="+id) {
				t.Errorf("tree still contains %s after cascade: %s", id, treeText)
			}
		}
		if !strings.Contains(treeText, "id=rootroor") {
			t.Errorf("tree should still contain rootroor: %s", treeText)
		}

		deltaText := env.TextContent(t, env.CallTool(t, "MemoryDelta", map[string]any{
			"since_snapshot": 0,
		}))
		if got := strings.Count(deltaText, "[rem]"); got != 3 {
			t.Errorf("rem count = %d, want 3: %s", got, deltaText)
		}

		// FTS5 sync: cascaded labels must not surface in search.
		searchText := env.TextContent(t, env.CallTool(t, "MemorySearch", map[string]any{
			"query":  "leaf0001",
			"budget": 500,
		}))
		if !strings.Contains(searchText, "no results") {
			t.Errorf("search for cascaded label should return no results: %s", searchText)
		}
	})

	t.Run("Reparent_WithGrandparent", func(t *testing.T) {
		env := mcptest.NewEnv(t)
		ctx := context.Background()
		seedForgetNode(t, env, "rootroor", "")
		seedForgetNode(t, env, "mid00001", "rootroor")
		seedForgetNode(t, env, "leaf0001", "mid00001")
		seedForgetNode(t, env, "leaf0002", "mid00001")

		env.CallTool(t, "MemoryForget", map[string]any{
			"node_id": "mid00001",
			"mode":    "reparent",
		})

		if _, err := env.Store.GetNode(ctx, "mid00001"); err == nil {
			t.Error("mid00001 still present after reparent")
		}

		for _, id := range []string{"leaf0001", "leaf0002"} {
			n, err := env.Store.GetNode(ctx, id)
			if err != nil {
				t.Fatalf("get %s: %v", id, err)
			}

			if n.ParentID != "rootroor" {
				t.Errorf("%s parent = %q, want rootroor", id, n.ParentID)
			}
		}

		deltaText := env.TextContent(t, env.CallTool(t, "MemoryDelta", map[string]any{
			"since_snapshot": 0,
		}))
		if got := strings.Count(deltaText, "[rem]"); got != 1 {
			t.Errorf("rem count = %d, want 1: %s", got, deltaText)
		}
		if got := strings.Count(deltaText, "[mod]"); got != 2 {
			t.Errorf("mod count = %d, want 2 (structural reparent): %s", got, deltaText)
		}
	})

	t.Run("Reparent_AtRoot", func(t *testing.T) {
		env := mcptest.NewEnv(t)
		ctx := context.Background()
		seedForgetNode(t, env, "rootroor", "")
		seedForgetNode(t, env, "child001", "rootroor")
		seedForgetNode(t, env, "child002", "rootroor")

		env.CallTool(t, "MemoryForget", map[string]any{
			"node_id": "rootroor",
			"mode":    "reparent",
		})

		for _, id := range []string{"child001", "child002"} {
			n, err := env.Store.GetNode(ctx, id)
			if err != nil {
				t.Fatalf("get %s: %v", id, err)
			}

			if n.ParentID != "" {
				t.Errorf("%s parent = %q, want empty (promoted to root)", id, n.ParentID)
			}
		}
	})

	t.Run("InvalidMode", func(t *testing.T) {
		env := mcptest.NewEnv(t)
		seedForgetNode(t, env, "leaf0001", "")

		result, err := env.Session.CallTool(context.Background(), &gomcp.CallToolParams{
			Name:      "MemoryForget",
			Arguments: map[string]any{"node_id": "leaf0001", "mode": "bogus"},
		})
		if err != nil {
			t.Fatalf("CallTool transport: %v", err)
		}

		if !result.IsError {
			t.Fatalf("expected IsError=true for unknown mode")
		}
		if msg := env.TextContent(t, result); !strings.Contains(msg, "unknown delete mode") {
			t.Errorf("error should mention unknown mode: %s", msg)
		}
	})
}

func seedForgetNode(t *testing.T, env *mcptest.Env, id, parent string) {
	t.Helper()

	err := env.Store.UpsertNode(context.Background(), &store.Node{
		ID: id, ParentID: parent,
		SourceFile: "test.md", NodeType: "heading", Depth: 1,
		Label: "label " + id, Content: "content " + id,
		Format: "plain", TokenCount: 5, ContentHash: "hash" + id,
	})
	if err != nil {
		t.Fatalf("seedForgetNode %s: %v", id, err)
	}
}

type clientMetaJSON struct {
	Name     string `json:"name"`
	Title    string `json:"title"`
	Version  string `json:"version"`
	Protocol string `json:"protocol"`
}

type sessJSON struct {
	ID             string         `json:"id"`
	Client         clientMetaJSON `json:"client_meta"`
	Transport      string         `json:"transport"`
	Listen         string         `json:"listen,omitempty"`
	ConnectedAt    int64          `json:"connected_at"`
	LastActivity   int64          `json:"last_activity"`
	CountToolCalls int64          `json:"count_tool_calls"`
}

type sessEnvJSON struct {
	DBPath   string     `json:"db_path"`
	Sessions []sessJSON `json:"sessions"`
}

func connectSessionClient(t *testing.T, srv *remindb.Server, name string) *gomcp.ClientSession {
	t.Helper()

	serverT, clientT := gomcp.NewInMemoryTransports()
	if _, err := srv.Connect(context.Background(), serverT); err != nil {
		t.Fatalf("server connect %s: %v", name, err)
	}

	c := gomcp.NewClient(&gomcp.Implementation{Name: name, Version: "0.1.0"}, nil)
	cs, err := c.Connect(context.Background(), clientT, nil)
	if err != nil {
		t.Fatalf("client connect %s: %v", name, err)
	}

	return cs
}

func readSessionsEnv(t *testing.T, cs *gomcp.ClientSession) sessEnvJSON {
	t.Helper()

	res, err := cs.ReadResource(context.Background(), &gomcp.ReadResourceParams{URI: "remindb://sessions"})
	if err != nil {
		t.Fatalf("ReadResource(sessions): %v", err)
	}

	var env sessEnvJSON
	if err := json.Unmarshal([]byte(res.Contents[0].Text), &env); err != nil {
		t.Fatalf("sessions JSON not parseable: %v\nbody: %s", err, res.Contents[0].Text)
	}
	return env
}

// Exercises the remindb://sessions resource end-to-end.
func TestMcp_SessionsResource(t *testing.T) {
	st := testutil.OpenTestDB(t)
	cfg := temperature.DefaultConfig()

	tracker, err := temperature.NewTracker(st, "", cfg, nil)
	if err != nil {
		t.Fatalf("NewTracker: %v", err)
	}
	srv, err := remindb.NewServer(st, tracker, cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	ctx := context.Background()
	a := connectSessionClient(t, srv, "agent-A")

	env := readSessionsEnv(t, a)
	if env.DBPath != st.Path {
		t.Errorf("db_path: got %q, want %q", env.DBPath, st.Path)
	}
	if len(env.Sessions) != 1 {
		t.Fatalf("after A connect: got %d sessions, want 1", len(env.Sessions))
	}
	if s := env.Sessions[0]; s.Transport != "stdio" || s.Listen != "" || s.CountToolCalls != 0 {
		t.Errorf("session shape: %+v (want stdio, no listen, 0 tool calls)", s)
	}
	if c := env.Sessions[0].Client; c.Name != "agent-A" || c.Version != "0.1.0" || c.Protocol == "" {
		t.Errorf("client_meta: %+v (want name=agent-A, version=0.1.0, non-empty protocol)", c)
	}
	if env.Sessions[0].ConnectedAt == 0 {
		t.Error("connected_at not stamped on first-seen request")
	}

	if _, err := a.CallTool(ctx, &gomcp.CallToolParams{Name: "MemoryTree", Arguments: map[string]any{}}); err != nil {
		t.Fatalf("CallTool MemoryTree: %v", err)
	}
	env = readSessionsEnv(t, a)
	if env.Sessions[0].CountToolCalls != 1 {
		t.Errorf("count_tool_calls after one MemoryTree (resource reads excluded): got %d, want 1", env.Sessions[0].CountToolCalls)
	}

	b := connectSessionClient(t, srv, "agent-B")
	if got := len(readSessionsEnv(t, a).Sessions); got != 2 {
		t.Fatalf("after B connect: got %d sessions, want 2", got)
	}

	if err := b.Close(); err != nil {
		t.Fatalf("close B: %v", err)
	}

	var final sessEnvJSON
	for range 50 {
		final = readSessionsEnv(t, a)
		if len(final.Sessions) == 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(final.Sessions) != 1 {
		t.Fatalf("after B disconnect: got %d sessions, want 1 (lazy reconcile against SDK set)", len(final.Sessions))
	}

	_ = a.Close()
}
