package tools

import (
	"context"
	"fmt"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/internal/treewalk"
	"github.com/radimsem/remindb/pkg/query"
	"github.com/radimsem/remindb/pkg/store"
)

type RelatedInput struct {
	Anchor    string  `json:"anchor"                jsonschema:"Node ID to traverse from"`
	Direction string  `json:"direction,omitempty"   jsonschema:"'out' | 'in' | 'both' (default 'both')"`
	Depth     int     `json:"depth,omitempty"       jsonschema:"Hop count 1-5 (default 1)"`
	Budget    int     `json:"budget,omitempty"      jsonschema:"Token budget for the response (default 1000)"`
	WeightMin float64 `json:"weight_min,omitempty"  jsonschema:"Filter edges with weight >= this (default 0)"`
}

const (
	defaultRelatedDepth  = 1
	defaultRelatedBudget = 1000
	maxRelatedDepth      = 5
	relatedQueryLimit    = 100
)

func (d *Deps) HandleRelated(ctx context.Context, _ *gomcp.CallToolRequest, input RelatedInput) (_ *gomcp.CallToolResult, _ any, err error) {
	budget := resolveBudget(input.Budget, d.WorkspaceConfig.Budgets.Related, defaultRelatedBudget)
	defer d.logCall(ctx, "MemoryRelated", &err, time.Now(),
		"anchor", input.Anchor, "direction", input.Direction,
		"depth", input.Depth, "budget", budget, "weight_min", input.WeightMin)

	if input.Anchor == "" {
		return nil, nil, fmt.Errorf("anchor is required")
	}

	direction := input.Direction
	if direction == "" {
		direction = store.DirectionBoth
	}

	depth := treewalk.ClampDepth(input.Depth, defaultRelatedDepth, maxRelatedDepth)

	related, err := d.Store.GetRelatedNodes(ctx, input.Anchor,
		store.WithDirection(direction),
		store.WithMaxDepth(depth),
		store.WithWeightMin(input.WeightMin),
		store.WithLimit(relatedQueryLimit),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch: related nodes: %w", err)
	}

	d.boostRelatedNodes(ctx, related)

	return &gomcp.CallToolResult{
		Content: []gomcp.Content{&gomcp.TextContent{Text: query.FormatRelated(related, budget)}},
	}, nil, nil
}

func (d *Deps) boostRelatedNodes(ctx context.Context, related []*store.RelatedNode) {
	if d.Tracker == nil || len(related) == 0 {
		return
	}

	ids := make([]string, len(related))
	for i, r := range related {
		ids[i] = r.Node.ID
	}

	if err := d.Tracker.RecordAccess(ctx, ids); err != nil && d.Logger != nil {
		d.Logger.WarnContext(ctx, "failed to boost: related access", "err", err, "count", len(ids))
	}
}
