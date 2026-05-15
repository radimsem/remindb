package tools

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/pkg/parser"
	"github.com/radimsem/remindb/pkg/store"
)

type RelateInput struct {
	SourceID     string  `json:"source_id"                 jsonschema:"Existing node ID (the link source)"`
	TargetID     string  `json:"target_id,omitempty"       jsonschema:"Explicit target node ID (highest priority)"`
	TargetLabel  string  `json:"target_label,omitempty"    jsonschema:"Heading label to resolve (used if target_id absent)"`
	TargetSource string  `json:"target_source,omitempty"   jsonschema:"Source-file qualifier for target_label (hard constraint)"`
	Weight       float64 `json:"weight,omitempty"          jsonschema:"Edge weight (default 1.0)"`
}

const defaultRelateWeight = 1.0

func (d *Deps) HandleRelate(ctx context.Context, _ *gomcp.CallToolRequest, input RelateInput) (_ *gomcp.CallToolResult, _ any, err error) {
	defer d.logCall("MemoryRelate", &err, time.Now(),
		"source_id", input.SourceID, "target_id", input.TargetID,
		"target_label", input.TargetLabel, "target_source", input.TargetSource,
		"weight", input.Weight)

	if input.SourceID == "" {
		return nil, nil, fmt.Errorf("source_id is required")
	}

	_, err = d.Store.GetNode(ctx, input.SourceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, fmt.Errorf("source_id not found: %s", input.SourceID)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch: source node %s: %w", input.SourceID, err)
	}

	weight := input.Weight
	if weight == 0 {
		weight = defaultRelateWeight
	}

	ref := parser.WikilinkRef{
		Label:      input.TargetLabel,
		SourceQual: input.TargetSource,
		IDHint:     input.TargetID,
		Weight:     weight,
	}

	d.Store.OpMu.Lock()
	defer d.Store.OpMu.Unlock()

	targetID, err := d.Resolver.Resolve(ctx, ref)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve: %w", err)
	}

	if targetID != "" {
		rel := &store.Relation{
			SourceNodeID: input.SourceID,
			TargetNodeID: targetID,
			Weight:       weight,
			Origin:       store.OriginManual,
		}
		if err := d.Store.UpsertRelation(ctx, rel); err != nil {
			return nil, nil, fmt.Errorf("failed to upsert: relation: %w", err)
		}

		return textResult(fmt.Sprintf("edge created (resolved): %s -> %s", input.SourceID, targetID)), nil, nil
	}

	if input.TargetLabel == "" && input.TargetID == "" {
		return nil, nil, fmt.Errorf("target_id or target_label is required")
	}

	pr := &store.PendingRelation{
		SourceNodeID: input.SourceID,
		TargetLabel:  input.TargetLabel,
		TargetSource: input.TargetSource,
		TargetIDHint: input.TargetID,
		Weight:       weight,
		Origin:       store.OriginManual,
	}
	if err := d.Store.InsertPendingRelation(ctx, pr); err != nil {
		return nil, nil, fmt.Errorf("failed to insert: pending relation: %w", err)
	}
	return textResult(fmt.Sprintf("edge created (pending): %s -> %q", input.SourceID, input.TargetLabel)), nil, nil
}

func textResult(msg string) *gomcp.CallToolResult {
	return &gomcp.CallToolResult{
		Content: []gomcp.Content{&gomcp.TextContent{Text: msg}},
	}
}
