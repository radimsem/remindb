package tools

import (
	"context"
	"fmt"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/pkg/query"
)

const maxFetchBatchIDs = 256

type FetchBatchInput struct {
	NodeIDs []string `json:"node_ids" jsonschema:"List of node IDs to fetch (max 256)"`
	Budget  int      `json:"budget,omitempty" jsonschema:"Token budget shared across the batch (0 or omitted = unlimited)"`
}

func (d *Deps) HandleFetchBatch(ctx context.Context, _ *gomcp.CallToolRequest, input FetchBatchInput) (_ *gomcp.CallToolResult, _ any, err error) {
	defer d.logCall("MemoryFetchBatch", &err, time.Now(), "ids", len(input.NodeIDs), "budget", input.Budget)

	if len(input.NodeIDs) == 0 {
		return nil, nil, fmt.Errorf("node_ids must be non-empty")
	}
	if len(input.NodeIDs) > maxFetchBatchIDs {
		return nil, nil, fmt.Errorf("node_ids length %d exceeds cap %d", len(input.NodeIDs), maxFetchBatchIDs)
	}

	result, missing, err := d.Engine.FetchBatch(ctx, input.NodeIDs, input.Budget)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch batch: %w", err)
	}

	d.boostResultNodes(ctx, result)
	text := query.FormatBatch(result, input.NodeIDs, missing)
	return &gomcp.CallToolResult{
		Content: []gomcp.Content{&gomcp.TextContent{Text: text}},
	}, nil, nil
}
