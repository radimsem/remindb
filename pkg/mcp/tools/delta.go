package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/radimsem/remindb/pkg/store"
)

const (
	defaultDeltaLimit = 500
	maxDeltaLimit     = 5000
)

type DeltaInput struct {
	SinceSnapshot int64 `json:"since_snapshot,omitempty" jsonschema:"Snapshot ID to diff from (0 for all changes)"`
	Limit         int   `json:"limit,omitempty" jsonschema:"Max diff rows to return (1-5000, default 500); past the cap the result is truncated with a since_snapshot continuation note"`
}

func (d *Deps) HandleDelta(ctx context.Context, _ *gomcp.CallToolRequest, input DeltaInput) (_ *gomcp.CallToolResult, _ any, err error) {
	limit := resolveDeltaLimit(input.Limit)
	defer d.logCall(ctx, "MemoryDelta", &err, time.Now(), "since_snapshot", input.SinceSnapshot, "limit", limit)

	diffs, truncated, overflow, err := d.Engine.Delta(ctx, input.SinceSnapshot, limit)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get delta: %w", err)
	}

	if len(diffs) == 0 {
		return textResult("no changes"), nil, nil
	}

	var b strings.Builder
	for _, dr := range diffs {
		fmt.Fprintf(&b, "[%s] %s (snapshot %d)\n", dr.Op, dr.NodeID, dr.SnapshotID)
	}
	if truncated {
		fmt.Fprintf(&b, "\n%s\n", deltaTruncationNote(diffs, input.SinceSnapshot, limit, overflow))
	}
	return textResult(b.String()), nil, nil
}

func resolveDeltaLimit(limit int) int64 {
	if limit <= 0 {
		return defaultDeltaLimit
	}
	if limit > maxDeltaLimit {
		return maxDeltaLimit
	}
	return int64(limit)
}

// Continuation note for a truncated delta. A snapshot that overflows the limit can't be resumed
// past by advancing since_snapshot; steer to a higher limit while one helps, and once the page is
// already maxed to MemoryDiff scoped to that snapshot — which is exactly the immediate-next
// snapshot after since, so it remains reachable in full.
func deltaTruncationNote(diffs []*store.DiffRecord, since, limit int64, overflow bool) string {
	last := diffs[len(diffs)-1].SnapshotID
	if !overflow {
		return fmt.Sprintf("note: truncated at limit %d; more changes exist. Re-call MemoryDelta(since_snapshot=%d) to continue.", limit, last)
	}
	if limit < maxDeltaLimit {
		return fmt.Sprintf("note: snapshot %d alone has more than %d diffs; raise limit (max %d) to see the rest.", last, limit, maxDeltaLimit)
	}
	return fmt.Sprintf("note: snapshot %d exceeds the max page of %d diffs and can't be paged by MemoryDelta; read it in full with MemoryDiff(from_snapshot_id=%d, to_snapshot_id=%d).", last, maxDeltaLimit, since, last)
}
