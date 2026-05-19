package tools

import (
	"context"
	"fmt"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/pkg/query"
)

type DiffInput struct {
	FromSnapshotID int64 `json:"from_snapshot_id" jsonschema:"Lower-bound snapshot ID (exclusive); use the older of the two snapshots"`
	ToSnapshotID   int64 `json:"to_snapshot_id"   jsonschema:"Upper-bound snapshot ID (inclusive); use the newer of the two snapshots"`
}

func (d *Deps) HandleDiff(ctx context.Context, _ *gomcp.CallToolRequest, input DiffInput) (_ *gomcp.CallToolResult, _ any, err error) {
	defer d.logCall(ctx, "MemoryDiff", &err, time.Now(), "from_snapshot_id", input.FromSnapshotID, "to_snapshot_id", input.ToSnapshotID)

	if input.FromSnapshotID > input.ToSnapshotID {
		return nil, nil, fmt.Errorf("from_snapshot_id (%d) must be <= to_snapshot_id (%d)", input.FromSnapshotID, input.ToSnapshotID)
	}

	diffs, err := d.Engine.Diff(ctx, input.FromSnapshotID, input.ToSnapshotID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get diff: %w", err)
	}

	return &gomcp.CallToolResult{
		Content: []gomcp.Content{&gomcp.TextContent{Text: query.FormatDiffs(diffs)}},
	}, nil, nil
}
