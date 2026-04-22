package tools

import (
	"context"
	"fmt"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/pkg/query"
)

type SearchInput struct {
	Query  string `json:"query" jsonschema:"Full-text search query"`
	Budget int    `json:"budget" jsonschema:"Maximum token budget for the response"`
}

func (d *Deps) HandleSearch(ctx context.Context, _ *gomcp.CallToolRequest, input SearchInput) (_ *gomcp.CallToolResult, _ any, err error) {
	defer d.logCall("MemorySearch", &err, time.Now(), "query", input.Query, "budget", input.Budget)

	result, err := d.Engine.Search(ctx, input.Query, input.Budget)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to search: %w", err)
	}

	d.boostResultNodes(ctx, result)
	text := query.FormatCompact(result)
	return &gomcp.CallToolResult{
		Content: []gomcp.Content{&gomcp.TextContent{Text: text}},
	}, nil, nil
}
