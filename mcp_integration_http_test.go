package remindb_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/radimsem/remindb/internal/mcptest"
)

func TestMcp_HttpTransport(t *testing.T) {
	env := mcptest.NewHttpEnv(t)
	fixture, _ := filepath.Abs("testdata/sample.md")

	compileResult := env.CallTool(t, "MemoryCompile", map[string]any{
		"path":    fixture,
		"message": "http-init",
	})
	if text := env.TextContent(t, compileResult); !strings.Contains(text, "compiled") {
		t.Fatalf("unexpected compile result: %s", text)
	}

	treeResult := env.CallTool(t, "MemoryTree", map[string]any{})
	if text := env.TextContent(t, treeResult); !strings.Contains(text, "Top Heading") {
		t.Errorf("tree missing expected heading; got: %s", text)
	}

	searchResult := env.CallTool(t, "MemorySearch", map[string]any{
		"query":  "paragraph",
		"budget": 1000,
	})
	if text := env.TextContent(t, searchResult); text == "no results" {
		t.Fatal("expected search results for 'paragraph' in sample.md")
	}

	writeResult := env.CallTool(t, "MemoryWrite", map[string]any{
		"payload": "HTTP transport smoke test write",
	})
	if text := env.TextContent(t, writeResult); !strings.Contains(text, "wrote") {
		t.Errorf("unexpected write result: %s", text)
	}
}
