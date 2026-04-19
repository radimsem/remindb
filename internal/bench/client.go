package bench

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Connect MCP client to `remindb serve --db <dbPath>` served server over stdio.
func spawnServerClient(ctx context.Context, dbPath string, serverStderr io.Writer) (*gomcp.ClientSession, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve: bench binary path: %w", err)
	}

	cmd := exec.Command(self, "serve", "--db", dbPath)
	cmd.Stderr = serverStderr

	transport := &gomcp.CommandTransport{Command: cmd}
	client := gomcp.NewClient(&gomcp.Implementation{Name: "remindb-bench", Version: "0.1.0"}, nil)

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: MCP client: %w", err)
	}
	return session, nil
}

func callTool(ctx context.Context, s *gomcp.ClientSession, name string, args map[string]any) (string, error) {
	result, err := s.CallTool(ctx, &gomcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		return "", fmt.Errorf("failed to call: %s: %w", name, err)
	}

	if result.IsError {
		return "", fmt.Errorf("tool %s returned error: %s", name, firstText(result))
	}
	return firstText(result), nil
}

func firstText(r *gomcp.CallToolResult) string {
	if r == nil || len(r.Content) == 0 {
		return ""
	}

	tc, ok := r.Content[0].(*gomcp.TextContent)
	if !ok {
		return ""
	}
	return tc.Text
}
