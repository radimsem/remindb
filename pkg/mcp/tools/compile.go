package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/pkg/compiler"
)

type CompileInput struct {
	Path    string `json:"path" jsonschema:"File path or directory to compile (absolute or relative; anchored to the server's source root automatically)"`
	Message string `json:"message,omitempty" jsonschema:"Snapshot message"`
}

func (d *Deps) HandleCompile(ctx context.Context, _ *gomcp.CallToolRequest, input CompileInput) (_ *gomcp.CallToolResult, _ any, err error) {
	defer d.logCall("MemoryCompile", &err, time.Now(), "path", input.Path, "message", input.Message)

	// Pure path normalization — runs before OpMu so EvalSymlinks doesn't block other writers.
	path, err := canonicalizePath(input.Path, d.SourceDir)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to canonicalize: %w", err)
	}

	d.Store.OpMu.Lock()
	defer d.Store.OpMu.Unlock()

	msg := input.Message
	if msg == "" {
		msg = "compile:" + path
	}

	fi, err := os.Stat(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to compile: %w", err)
	}

	opts := []compiler.Option{compiler.WithLogger(d.Logger), compiler.WithRedactor(d.Redactor)}

	var result *compiler.Result
	if fi.IsDir() {
		result, err = compiler.CompileDir(ctx, d.Store, path, msg, opts...)
	} else {
		result, err = compiler.CompileFile(ctx, d.Store, path, msg, opts...)
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

func canonicalizePath(input, sourceDir string) (string, error) {
	if sourceDir == "" || input == "" {
		return input, nil
	}

	absInput, err := filepath.Abs(input)
	if err != nil {
		return "", fmt.Errorf("failed to resolve: %s: %w", input, err)
	}
	absSource, err := filepath.Abs(sourceDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve: %s: %w", sourceDir, err)
	}

	if resolved, err := filepath.EvalSymlinks(absInput); err == nil {
		absInput = resolved
	}
	if resolved, err := filepath.EvalSymlinks(absSource); err == nil {
		absSource = resolved
	}

	rel, err := filepath.Rel(absSource, absInput)
	outsideSource := rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
	if err != nil || outsideSource {
		return input, nil
	}

	return filepath.Join(sourceDir, rel), nil
}
