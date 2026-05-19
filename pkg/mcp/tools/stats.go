package tools

import (
	"context"
	"fmt"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/pkg/inspect"
)

type StatsInput struct{}

func (d *Deps) HandleStats(ctx context.Context, _ *gomcp.CallToolRequest, _ StatsInput) (_ *gomcp.CallToolResult, _ any, err error) {
	defer d.logCall(ctx, "MemoryStats", &err, time.Now())

	stats, err := inspect.Collect(ctx, d.Store)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to collect: stats: %w", err)
	}

	return textResult(inspect.Format(stats)), nil, nil
}
