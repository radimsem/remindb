package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type HistoryInput struct {
	Anchor string `json:"anchor" jsonschema:"Node ID to view history for"`
	Depth  int    `json:"depth,omitempty" jsonschema:"Maximum number of history entries (default 10)"`
}

func (d *Deps) HandleHistory(ctx context.Context, _ *gomcp.CallToolRequest, input HistoryInput) (_ *gomcp.CallToolResult, _ any, err error) {
	defer d.logCall("MemoryHistory", &err, time.Now(), "anchor", input.Anchor, "depth", input.Depth)

	diffs, err := d.Store.GetDiffsForNode(ctx, input.Anchor)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get history: %w", err)
	}

	limit := input.Depth
	if limit <= 0 {
		limit = 10
	}
	if limit > len(diffs) {
		limit = len(diffs)
	}

	if len(diffs) == 0 {
		return &gomcp.CallToolResult{
			Content: []gomcp.Content{&gomcp.TextContent{Text: "no history for " + input.Anchor}},
		}, nil, nil
	}

	var b strings.Builder
	for _, dr := range diffs[:limit] {
		fmt.Fprintf(&b, "snapshot %d: %s\n", dr.SnapshotID, dr.Op)
		if dr.OldContent != "" {
			fmt.Fprintf(&b, "  old: %s\n", truncate(dr.OldContent, 100))
		}
		if dr.NewContent != "" {
			fmt.Fprintf(&b, "  new: %s\n", truncate(dr.NewContent, 100))
		}
	}
	return &gomcp.CallToolResult{
		Content: []gomcp.Content{&gomcp.TextContent{Text: b.String()}},
	}, nil, nil
}
