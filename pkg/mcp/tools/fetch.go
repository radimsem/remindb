package tools

import (
	"context"
	"fmt"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/pkg/query"
)

type FetchInput struct {
	Anchor string `json:"anchor" jsonschema:"Node ID to fetch context around"`
	Budget int    `json:"budget" jsonschema:"Maximum token budget for the response"`
}

func (d *Deps) HandleFetch(ctx context.Context, _ *gomcp.CallToolRequest, input FetchInput) (*gomcp.CallToolResult, any, error) {
	result, err := d.Engine.Fetch(ctx, input.Anchor, input.Budget)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch: %w", err)
	}

	d.boostResultNodes(ctx, result)
	text := query.Format(result)
	return &gomcp.CallToolResult{
		Content: []gomcp.Content{&gomcp.TextContent{Text: text}},
	}, nil, nil
}
