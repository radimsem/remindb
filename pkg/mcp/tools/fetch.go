package tools

import (
	"context"
	"fmt"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/pkg/query"
)

type FetchInput struct {
	Anchor string `json:"anchor" jsonschema:"Node ID to fetch context around"`
	Budget int    `json:"budget" jsonschema:"Maximum token budget for the response"`
	Depth  int    `json:"depth,omitempty" jsonschema:"Max descendant depth (1-128, default 32); 0 uses engine default"`
}

func (d *Deps) HandleFetch(ctx context.Context, _ *gomcp.CallToolRequest, input FetchInput) (_ *gomcp.CallToolResult, _ any, err error) {
	defer d.logCall("MemoryFetch", &err, time.Now(), "anchor", input.Anchor, "budget", input.Budget, "depth", input.Depth)

	result, err := d.Engine.Fetch(ctx, input.Anchor, input.Budget, input.Depth)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch: %w", err)
	}

	d.boostResultNodes(ctx, result)
	text := query.Format(result)
	return &gomcp.CallToolResult{
		Content: []gomcp.Content{&gomcp.TextContent{Text: text}},
	}, nil, nil
}
