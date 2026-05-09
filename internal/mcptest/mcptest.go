package mcptest

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/internal/testutil"
	remindb "github.com/radimsem/remindb/pkg/mcp"
	"github.com/radimsem/remindb/pkg/store"
	"github.com/radimsem/remindb/pkg/temperature"
)

type Env struct {
	Session *mcp.ClientSession
	Store   *store.Store
}

func NewEnv(t *testing.T) *Env {
	t.Helper()

	st := testutil.OpenTestDB(t)
	cfg := temperature.DefaultConfig()
	tracker, err := temperature.NewTracker(st, cfg, nil)
	if err != nil {
		t.Fatalf("NewTracker: %v", err)
	}

	srv := remindb.NewServer(st, tracker, cfg)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ctx := context.Background()

	_, err = srv.Connect(ctx, serverTransport)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-agent", Version: "0.1.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}

	t.Cleanup(func() { _ = session.Close() })

	return &Env{Session: session, Store: st}
}

func (e *Env) CallTool(t *testing.T, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()

	t.Logf("→ %s %v", name, args)

	result, err := e.Session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool %s: %v", name, err)
	}
	if result.IsError {
		msg := "unknown error"
		if len(result.Content) > 0 {
			if tc, ok := result.Content[0].(*mcp.TextContent); ok {
				msg = tc.Text
			}
		}
		t.Fatalf("CallTool %s returned error: %s", name, msg)
	}

	text := e.textPreview(result)
	t.Logf("← %s: %s", name, text)

	return result
}

func (e *Env) TextContent(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("empty content in result")
	}

	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}

	return tc.Text
}

const maxPreview = 120

func (e *Env) textPreview(result *mcp.CallToolResult) string {
	if len(result.Content) == 0 {
		return "(empty)"
	}

	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		return "(non-text)"
	}

	s := tc.Text
	if len(s) <= maxPreview {
		return s
	}

	return s[:maxPreview] + "..."
}
