package tools

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type PinInput struct {
	NodeID      string   `json:"node_id"               jsonschema:"Node ID to pin against temperature decay and cold-set selection"`
	Temperature *float64 `json:"temperature,omitempty" jsonschema:"Optional override temperature in [0, 1]; if omitted, the node is pinned at its current temperature"`
}

type UnpinInput struct {
	NodeID string `json:"node_id" jsonschema:"Node ID to unpin"`
}

func (d *Deps) HandlePin(ctx context.Context, _ *gomcp.CallToolRequest, input PinInput) (_ *gomcp.CallToolResult, _ any, err error) {
	defer d.logCall(ctx, "MemoryPin", &err, time.Now(), "node_id", input.NodeID)

	if input.Temperature != nil && (*input.Temperature < 0 || *input.Temperature > 1) {
		return nil, nil, fmt.Errorf("temperature must be in [0, 1], got %g", *input.Temperature)
	}
	return d.setPinned(ctx, input.NodeID, true, input.Temperature, "pinned")
}

func (d *Deps) HandleUnpin(ctx context.Context, _ *gomcp.CallToolRequest, input UnpinInput) (_ *gomcp.CallToolResult, _ any, err error) {
	defer d.logCall(ctx, "MemoryUnpin", &err, time.Now(), "node_id", input.NodeID)
	return d.setPinned(ctx, input.NodeID, false, nil, "unpinned")
}

func (d *Deps) setPinned(ctx context.Context, nodeID string, pinned bool, temp *float64, verb string) (*gomcp.CallToolResult, any, error) {
	if nodeID == "" {
		return nil, nil, fmt.Errorf("node_id is required")
	}

	d.Store.OpMu.Lock()
	defer d.Store.OpMu.Unlock()

	if _, err := d.Store.GetNode(ctx, nodeID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, fmt.Errorf("node_id not found: %s", nodeID)
		}
		return nil, nil, fmt.Errorf("failed to fetch: node %s: %w", nodeID, err)
	}

	if err := d.Store.SetPinned(ctx, nodeID, pinned, temp); err != nil {
		return nil, nil, fmt.Errorf("failed to set: pinned %s: %w", nodeID, err)
	}

	msg := fmt.Sprintf("%s node %s", verb, nodeID)
	if temp != nil {
		msg = fmt.Sprintf("%s node %s at temperature %g", verb, nodeID, *temp)
	}
	return textResult(msg), nil, nil
}
