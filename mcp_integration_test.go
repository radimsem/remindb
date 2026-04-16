package remindb_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/radimsem/remindb/internal/mcptest"
)

// TestMcp_OpenClawAgent simulates an OpenClaw agent session:
// compile identity files, search for capabilities, fetch context
// around a node, write a new memory, then check the tree.
func TestMcp_OpenClawAgent(t *testing.T) {
	env := mcptest.NewEnv(t)
	dir, _ := filepath.Abs("testdata/openclaw")

	// 1. Agent compiles its identity files into the database.
	compileResult := env.CallTool(t, "memory_compile", map[string]any{
		"path":    dir,
		"message": "openclaw-init",
	})
	text := env.TextContent(t, compileResult)
	if !strings.Contains(text, "compiled") {
		t.Fatalf("unexpected compile result: %s", text)
	}

	// 2. Agent inspects its memory tree — should include all workspace files.
	treeResult := env.CallTool(t, "memory_tree", map[string]any{})
	treeText := env.TextContent(t, treeResult)
	if !strings.Contains(treeText, "Soul") && !strings.Contains(treeText, "Identity") {
		t.Errorf("tree should contain Soul/Identity headings, got: %s", treeText[:min(200, len(treeText))])
	}

	// 3. Agent searches for its capabilities to self-describe.
	searchResult := env.CallTool(t, "memory_search", map[string]any{
		"query":  "refactoring security vulnerabilities",
		"budget": 2000,
	})
	searchText := env.TextContent(t, searchResult)
	if searchText == "no results" {
		t.Fatal("expected search results for capabilities")
	}

	// 4. Agent checks user preferences before responding.
	userResult := env.CallTool(t, "memory_search", map[string]any{
		"query":  "terse responses error handling",
		"budget": 1000,
	})
	userText := env.TextContent(t, userResult)
	if userText == "no results" {
		t.Fatal("expected search results for user preferences from USER.md")
	}

	// 5. Agent checks daily memory logs for recent session context.
	dailyResult := env.CallTool(t, "memory_search", map[string]any{
		"query":  "rate limiting token bucket",
		"budget": 1000,
	})
	dailyText := env.TextContent(t, dailyResult)
	if dailyText == "no results" {
		t.Fatal("expected search results for daily memory log content")
	}

	// 6. Agent searches for JSON session data from memory/session_data.json.
	jsonResult := env.CallTool(t, "memory_search", map[string]any{
		"query":  "session tasks blocked WebSocket",
		"budget": 1000,
	})
	jsonText := env.TextContent(t, jsonResult)
	if jsonText == "no results" {
		t.Fatal("expected search results for JSON session data")
	}

	// 7. Agent writes a new memory from a conversation.
	writeResult := env.CallTool(t, "memory_write", map[string]any{
		"payload": "User prefers verbose explanations when reviewing Go code. Confirmed after code review session.",
	})
	writeText := env.TextContent(t, writeResult)
	if !strings.Contains(writeText, "wrote node") {
		t.Fatalf("unexpected write result: %s", writeText)
	}

	// Extract the node ID from "wrote node XXXXXXXX (N tokens)".
	nodeID := extractNodeID(writeText)

	// 8. Agent fetches context around the new memory.
	fetchResult := env.CallTool(t, "memory_fetch", map[string]any{
		"anchor": nodeID,
		"budget": 1000,
	})
	fetchText := env.TextContent(t, fetchResult)
	if !strings.Contains(fetchText, "verbose explanations") {
		t.Errorf("fetch should include the written content, got: %s", fetchText[:min(100, len(fetchText))])
	}

	// 9. Agent checks delta since the compile snapshot.
	deltaResult := env.CallTool(t, "memory_delta", map[string]any{
		"since_snapshot": 1,
	})
	deltaText := env.TextContent(t, deltaResult)
	if deltaText == "no changes" {
		t.Error("expected delta changes after write")
	}
}

// TestMcp_ClaudeCodeAgent simulates a Claude Code session:
// compile project instructions + memory files, search for testing
// feedback, write a new feedback memory, summarize a verbose node.
func TestMcp_ClaudeCodeAgent(t *testing.T) {
	env := mcptest.NewEnv(t)
	dir, _ := filepath.Abs("testdata/claude-code")

	// 1. Compile the project instructions and memory files.
	env.CallTool(t, "memory_compile", map[string]any{
		"path":    dir,
		"message": "claude-code-init",
	})

	// 2. Agent starts a task and searches for relevant testing guidance.
	searchResult := env.CallTool(t, "memory_search", map[string]any{
		"query":  "snapshot",
		"budget": 2000,
	})
	searchText := env.TextContent(t, searchResult)
	if !strings.Contains(searchText, "snapshot") {
		t.Fatal("search should find the snapshot testing feedback")
	}

	// 3. Agent searches for user preferences before responding.
	prefResult := env.CallTool(t, "memory_search", map[string]any{
		"query":  "Zod",
		"budget": 1000,
	})
	prefText := env.TextContent(t, prefResult)
	if !strings.Contains(prefText, "user_preferences.md") {
		t.Fatal("search should find user Zod preference")
	}

	// 4. Agent writes a new feedback memory after user correction.
	env.CallTool(t, "memory_write", map[string]any{
		"payload": "User prefers function components over class components. Always use hooks for state management.",
	})

	// 5. Agent searches for the newly written memory to verify persistence.
	hookResult := env.CallTool(t, "memory_search", map[string]any{
		"query":  "hooks",
		"budget": 1000,
	})
	hookText := env.TextContent(t, hookResult)
	if !strings.Contains(hookText, "function components") {
		t.Fatal("newly written memory should be searchable")
	}

	// 6. Agent finds a verbose node and summarizes it.
	treeResult := env.CallTool(t, "memory_tree", map[string]any{})
	treeText := env.TextContent(t, treeResult)

	// Find a node ID from the tree output to summarize.
	nodeID := extractFirstNodeID(treeText)
	if nodeID == "" {
		t.Fatal("could not find a node ID in tree output")
	}

	env.CallTool(t, "memory_summarize", map[string]any{
		"node_id": nodeID,
		"summary": "Summarized: webshop project uses Next.js 15 with App Router.",
	})

	// Verify the summarization took effect.
	fetchResult := env.CallTool(t, "memory_fetch", map[string]any{
		"anchor": nodeID,
		"budget": 1000,
	})
	fetchText := env.TextContent(t, fetchResult)
	if !strings.Contains(fetchText, "Summarized") {
		t.Errorf("fetched content should reflect summary, got: %s", fetchText[:min(100, len(fetchText))])
	}
}

// TestMcp_GeminiCliAgent simulates a Gemini CLI session:
// compile mixed markdown+YAML fixtures, search architecture decisions,
// write a decision, check history.
func TestMcp_GeminiCliAgent(t *testing.T) {
	env := mcptest.NewEnv(t)
	dir, _ := filepath.Abs("testdata/gemini-cli")

	// 1. Compile the infra-api project context.
	env.CallTool(t, "memory_compile", map[string]any{
		"path":    dir,
		"message": "gemini-cli-init",
	})

	// 2. Agent searches for architecture decisions before modifying code.
	archResult := env.CallTool(t, "memory_search", map[string]any{
		"query":  "idempotent",
		"budget": 2000,
	})
	archText := env.TextContent(t, archResult)
	if !strings.Contains(strings.ToLower(archText), "idempotent") {
		t.Fatal("search should find idempotent apply semantics decision")
	}

	// 3. Agent checks for incident history before touching namespace code.
	incidentResult := env.CallTool(t, "memory_search", map[string]any{
		"query":  "finalizer",
		"budget": 2000,
	})
	incidentText := env.TextContent(t, incidentResult)
	if !strings.Contains(incidentText, "context.yaml") {
		t.Fatal("search should find the finalizer incident")
	}

	// 4. Agent writes a new architecture decision.
	writeResult := env.CallTool(t, "memory_write", map[string]any{
		"payload": "Decision: use structured logging with slog instead of log package. Rationale: better observability in Kubernetes, JSON output for log aggregation.",
	})
	writeText := env.TextContent(t, writeResult)
	nodeID := extractNodeID(writeText)

	// 5. Agent verifies the decision is searchable.
	slogResult := env.CallTool(t, "memory_search", map[string]any{
		"query":  "slog",
		"budget": 1000,
	})
	slogText := env.TextContent(t, slogResult)
	if !strings.Contains(slogText, "slog") {
		t.Fatal("newly written decision should be searchable")
	}

	// 6. Agent checks history of the newly written node.
	histResult := env.CallTool(t, "memory_history", map[string]any{
		"anchor": nodeID,
	})
	histText := env.TextContent(t, histResult)
	if histText == "no history" {
		t.Error("expected history for the written node")
	}

	// 7. Agent updates the decision node.
	env.CallTool(t, "memory_write", map[string]any{
		"anchor":  nodeID,
		"payload": "Decision: use structured logging with slog instead of log package. Rationale: better observability. Approved by team on 2026-04-16.",
	})

	// 8. Check delta to see both writes.
	deltaResult := env.CallTool(t, "memory_delta", map[string]any{
		"since_snapshot": 1,
	})
	deltaText := env.TextContent(t, deltaResult)
	if deltaText == "no changes" {
		t.Error("expected delta changes after writes")
	}
}

// TestMcp_CodexAgent simulates a Codex agent session: compile Python pipeline
// fixtures, search for typing feedback and migration state, write a new
// decision, verify cross-format search hits YAML config.
func TestMcp_CodexAgent(t *testing.T) {
	env := mcptest.NewEnv(t)
	dir, _ := filepath.Abs("testdata/codex")

	// 1. Compile the data pipeline project context.
	compileResult := env.CallTool(t, "memory_compile", map[string]any{
		"path":    dir,
		"message": "codex-init",
	})
	text := env.TextContent(t, compileResult)
	if !strings.Contains(text, "compiled") {
		t.Fatalf("unexpected compile result: %s", text)
	}

	// 2. Agent inspects the memory tree.
	treeResult := env.CallTool(t, "memory_tree", map[string]any{})
	treeText := env.TextContent(t, treeResult)
	if !strings.Contains(treeText, "Codex") && !strings.Contains(treeText, "Project") {
		t.Errorf("tree should contain Codex/Project headings, got: %s", treeText[:min(200, len(treeText))])
	}

	// 3. Agent searches for typing feedback before writing code.
	typingResult := env.CallTool(t, "memory_search", map[string]any{
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
	migrationResult := env.CallTool(t, "memory_search", map[string]any{
		"query":  "ETL migration remaining blocked",
		"budget": 2000,
	})
	migrationText := env.TextContent(t, migrationResult)
	if migrationText == "no results" {
		t.Fatal("expected search results for ETL migration state")
	}

	// 5. Agent searches YAML pipeline config for vendor details.
	vendorResult := env.CallTool(t, "memory_search", map[string]any{
		"query":  "vendor oauth2 websocket redis",
		"budget": 1000,
	})
	vendorText := env.TextContent(t, vendorResult)
	if vendorText == "no results" {
		t.Fatal("expected search results for YAML vendor config")
	}

	// 6. Agent writes a new architecture decision.
	writeResult := env.CallTool(t, "memory_write", map[string]any{
		"payload": "Decision: use polars instead of pandas for new transforms. Rationale: 5x faster on large datasets, native lazy evaluation, better memory efficiency with Apache Arrow backend.",
	})
	writeText := env.TextContent(t, writeResult)
	if !strings.Contains(writeText, "wrote node") {
		t.Fatalf("unexpected write result: %s", writeText)
	}
	nodeID := extractNodeID(writeText)

	// 7. Agent verifies the decision is searchable.
	polarsResult := env.CallTool(t, "memory_search", map[string]any{
		"query":  "polars",
		"budget": 1000,
	})
	polarsText := env.TextContent(t, polarsResult)
	if !strings.Contains(polarsText, "polars") {
		t.Fatal("newly written decision should be searchable")
	}

	// 8. Agent fetches context around the new decision.
	fetchResult := env.CallTool(t, "memory_fetch", map[string]any{
		"anchor": nodeID,
		"budget": 2000,
	})
	fetchText := env.TextContent(t, fetchResult)
	if !strings.Contains(fetchText, "polars") {
		t.Errorf("fetch should include the written content, got: %s", fetchText[:min(100, len(fetchText))])
	}

	// 9. Agent checks delta since compile.
	deltaResult := env.CallTool(t, "memory_delta", map[string]any{
		"since_snapshot": 1,
	})
	deltaText := env.TextContent(t, deltaResult)
	if deltaText == "no changes" {
		t.Error("expected delta changes after write")
	}
}

// TestMcp_ToolDiscovery verifies that the client can list all available tools.
func TestMcp_ToolDiscovery(t *testing.T) {
	env := mcptest.NewEnv(t)

	tools, err := env.Session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	expected := map[string]bool{
		"memory_fetch":     false,
		"memory_search":    false,
		"memory_write":     false,
		"memory_compile":   false,
		"memory_delta":     false,
		"memory_summarize": false,
		"memory_history":   false,
		"memory_tree":      false,
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

// extractNodeID parses "wrote node XXXXXXXX (N tokens)" to get the node ID.
func extractNodeID(s string) string {
	prefix := "wrote node "
	i := strings.Index(s, prefix)
	if i < 0 {
		return ""
	}
	rest := s[i+len(prefix):]
	if j := strings.IndexByte(rest, ' '); j > 0 {
		return rest[:j]
	}
	return rest
}

// extractFirstNodeID finds the first node ID in tree output like "(id=XXXXXXXX".
func extractFirstNodeID(tree string) string {
	prefix := "(id="
	i := strings.Index(tree, prefix)
	if i < 0 {
		// Try the other format: "(XXXXXXXX)"
		return extractFirstParenID(tree)
	}
	rest := tree[i+len(prefix):]
	if j := strings.IndexAny(rest, " )"); j > 0 {
		return rest[:j]
	}
	return ""
}

func extractFirstParenID(tree string) string {
	// Tree format: "[type] label (XXXXXXXX)"
	i := strings.Index(tree, "(")
	if i < 0 {
		return ""
	}
	rest := tree[i+1:]
	if j := strings.IndexByte(rest, ')'); j > 0 {
		id := rest[:j]
		// Validate it looks like an ID (8 alphanumeric chars).
		if len(id) >= 6 && !strings.Contains(id, " ") {
			return id
		}
	}
	return ""
}
