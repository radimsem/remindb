package tools

import (
	"context"
	"fmt"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/internal/contentid"
	"github.com/radimsem/remindb/internal/tokens"
)

type SummarizeInput struct {
	NodeID  string `json:"node_id" jsonschema:"Node ID to summarize"`
	Summary string `json:"summary" jsonschema:"Summary text to replace the node content"`
}

func (d *Deps) HandleSummarize(ctx context.Context, _ *gomcp.CallToolRequest, input SummarizeInput) (*gomcp.CallToolResult, any, error) {
	node, err := d.Store.GetNode(ctx, input.NodeID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get node: %s: %w", input.NodeID, err)
	}

	contentHash := contentid.ContentHash(input.Summary)

	oldTokens := node.TokenCount
	node.Content = input.Summary
	node.ContentHash = contentHash
	node.TokenCount = tokens.Estimate(input.Summary)
	node.Label = "Summary: " + firstLine(input.Summary, 70)

	if err := d.Store.UpsertNode(ctx, node); err != nil {
		return nil, nil, fmt.Errorf("failed to update node: %w", err)
	}

	msg := fmt.Sprintf("summarized node %s (%d → %d tokens)", input.NodeID, oldTokens, node.TokenCount)
	return &gomcp.CallToolResult{
		Content: []gomcp.Content{&gomcp.TextContent{Text: msg}},
	}, nil, nil
}
