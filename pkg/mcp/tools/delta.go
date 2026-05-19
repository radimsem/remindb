package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type DeltaInput struct {
	SinceSnapshot int64 `json:"since_snapshot,omitempty" jsonschema:"Snapshot ID to diff from (0 for all changes)"`
}

func (d *Deps) HandleDelta(ctx context.Context, _ *gomcp.CallToolRequest, input DeltaInput) (_ *gomcp.CallToolResult, _ any, err error) {
	defer d.logCall(ctx, "MemoryDelta", &err, time.Now(), "since_snapshot", input.SinceSnapshot)

	diffs, err := d.Engine.Delta(ctx, input.SinceSnapshot)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get delta: %w", err)
	}

	if len(diffs) == 0 {
		return &gomcp.CallToolResult{
			Content: []gomcp.Content{&gomcp.TextContent{Text: "no changes"}},
		}, nil, nil
	}

	var b strings.Builder
	for _, dr := range diffs {
		fmt.Fprintf(&b, "[%s] %s (snapshot %d)\n", dr.Op, dr.NodeID, dr.SnapshotID)
	}
	return &gomcp.CallToolResult{
		Content: []gomcp.Content{&gomcp.TextContent{Text: b.String()}},
	}, nil, nil
}
