package tools

import (
	"context"
	"fmt"
	"os"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/pkg/compiler"
)

type CompileInput struct {
	Path    string `json:"path" jsonschema:"File path or directory to compile"`
	Message string `json:"message,omitempty" jsonschema:"Snapshot message"`
}

func (d *Deps) HandleCompile(ctx context.Context, _ *gomcp.CallToolRequest, input CompileInput) (_ *gomcp.CallToolResult, _ any, err error) {
	defer d.logCall("MemoryCompile", &err, time.Now(), "path", input.Path, "message", input.Message)

	d.Store.OpMu.Lock()
	defer d.Store.OpMu.Unlock()

	msg := input.Message
	if msg == "" {
		msg = "compile:" + input.Path
	}

	fi, err := os.Stat(input.Path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to compile: %w", err)
	}

	var result *compiler.Result
	if fi.IsDir() {
		result, err = compiler.CompileDir(ctx, d.Store, input.Path, msg)
	} else {
		result, err = compiler.Compile(ctx, d.Store,
			compiler.WithPaths([]string{input.Path}),
			compiler.WithMessage(msg),
		)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("failed to compile: %w", err)
	}

	text := fmt.Sprintf("compiled: %d added, %d modified, %d removed (%d ops)",
		result.Added, result.Modified, result.Removed, result.Total)

	return &gomcp.CallToolResult{
		Content: []gomcp.Content{&gomcp.TextContent{Text: text}},
	}, nil, nil
}
