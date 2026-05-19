package tools

import (
	"context"
	"fmt"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/internal/contentid"
	"github.com/radimsem/remindb/internal/tokens"
	"github.com/radimsem/remindb/pkg/diff"
	"github.com/radimsem/remindb/pkg/parser"
)

type SummarizeInput struct {
	NodeID      string   `json:"node_id" jsonschema:"Node ID to summarize"`
	Summary     string   `json:"summary" jsonschema:"Summary text to replace the node content"`
	Temperature *float64 `json:"temperature,omitempty" jsonschema:"Optional post-summarize temperature in [0, 1]; defaults to Config.SummarizeRebound"`
}

func (d *Deps) HandleSummarize(ctx context.Context, _ *gomcp.CallToolRequest, input SummarizeInput) (_ *gomcp.CallToolResult, _ any, err error) {
	defer d.logCall(ctx, "MemorySummarize", &err, time.Now(), "node_id", input.NodeID, "summary_bytes", len(input.Summary))

	if input.Temperature != nil && (*input.Temperature < 0 || *input.Temperature > 1) {
		return nil, nil, fmt.Errorf("temperature must be in [0, 1], got %g", *input.Temperature)
	}

	d.Store.OpMu.Lock()
	defer d.Store.OpMu.Unlock()

	existing, err := d.Store.GetNode(ctx, input.NodeID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get node: %s: %w", input.NodeID, err)
	}

	summary, hits := d.Redactor.Scrub(input.Summary)
	d.logRedaction("MemorySummarize", hits)

	oldTokens := existing.TokenCount
	tokenCount := tokens.Estimate(summary)

	prev := map[string]diff.NodeState{
		input.NodeID: {Hash: existing.ContentHash, Content: existing.Content},
	}

	rebound := d.SummarizeRebound
	if input.Temperature != nil {
		rebound = *input.Temperature
	}

	node := &parser.ContextNode{
		ID:          existing.ID,
		ParentID:    existing.ParentID,
		SourceFile:  existing.SourceFile,
		NodeType:    parser.NodeType(existing.NodeType),
		Depth:       existing.Depth,
		Label:       "Summary: " + firstLine(summary, 70),
		Content:     summary,
		Format:      existing.Format,
		TokenCount:  tokenCount,
		ContentHash: contentid.ContentHash(summary),
		Temperature: &rebound,
	}

	if err := d.emitNodeChange(ctx, node, prev, "summarize:"+input.NodeID); err != nil {
		return nil, nil, fmt.Errorf("failed to summarize: %w", err)
	}

	msg := fmt.Sprintf("summarized node %s (%d → %d tokens)", input.NodeID, oldTokens, tokenCount)
	return &gomcp.CallToolResult{
		Content: []gomcp.Content{&gomcp.TextContent{Text: msg}},
	}, nil, nil
}
