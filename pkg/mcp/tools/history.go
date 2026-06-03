package tools

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/internal/treewalk"
)

type HistoryInput struct {
	Anchor string `json:"anchor" jsonschema:"Node ID to view history for"`
	Depth  int    `json:"depth,omitempty" jsonschema:"Maximum number of history entries (default 10)"`
}

func (d *Deps) HandleHistory(ctx context.Context, _ *gomcp.CallToolRequest, input HistoryInput) (_ *gomcp.CallToolResult, _ any, err error) {
	defer d.logCall(ctx, "MemoryHistory", &err, time.Now(), "anchor", input.Anchor, "depth", input.Depth)

	if input.Anchor == "" {
		return nil, nil, fmt.Errorf("anchor is required")
	}

	diffs, err := d.Store.GetDiffsForNode(ctx, input.Anchor)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get history: %w", err)
	}

	// An empty trail is ambiguous: a live node that never changed, or an
	// unknown ID. Probe the nodes table to tell them apart. A non-empty
	// trail renders as-is — diffs outlive the node, so history stays
	// available for forgotten nodes.
	if len(diffs) == 0 {
		_, err = d.Store.GetNode(ctx, input.Anchor)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, nil, fmt.Errorf("node_id not found: %s", input.Anchor)
			}
			return nil, nil, fmt.Errorf("failed to fetch: node %s: %w", input.Anchor, err)
		}
		return textResult("no history for " + input.Anchor), nil, nil
	}

	limit := treewalk.ClampDepth(input.Depth, 10, treewalk.MaxDepth)
	if limit > len(diffs) {
		limit = len(diffs)
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
	return textResult(b.String()), nil, nil
}
