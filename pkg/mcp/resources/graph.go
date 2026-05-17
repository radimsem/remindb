package resources

import (
	"context"
	"encoding/json"
	"fmt"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

const GraphURI = "remindb://graph"

type graphNode struct {
	ID          string  `json:"id"`
	Label       string  `json:"label"`
	Type        string  `json:"type"`
	Temperature float64 `json:"temperature"`
}

type graphEdge struct {
	Source string  `json:"source"`
	Target string  `json:"target"`
	Weight float64 `json:"weight"`
	Origin string  `json:"origin"`
}

type graphPending struct {
	Source       string  `json:"source"`
	TargetLabel  string  `json:"target_label"`
	TargetSource string  `json:"target_source"`
	TargetIDHint string  `json:"target_id_hint"`
	Weight       float64 `json:"weight"`
	Origin       string  `json:"origin"`
}

type graphEnvelope struct {
	Nodes   []graphNode    `json:"nodes"`
	Edges   []graphEdge    `json:"edges"`
	Pending []graphPending `json:"pending"`
}

func (d *Deps) graphBody(ctx context.Context) ([]byte, error) {
	relations, err := d.Store.GetAllRelations(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get: relations: %w", err)
	}

	pending, err := d.Store.GetAllPendingRelations(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get: pending relations: %w", err)
	}

	all, err := d.Store.GetAllNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get: graph nodes: %w", err)
	}

	referenced := make(map[string]struct{}, len(relations)*2+len(pending))
	for _, r := range relations {
		referenced[r.SourceNodeID] = struct{}{}
		referenced[r.TargetNodeID] = struct{}{}
	}

	for _, p := range pending {
		referenced[p.SourceNodeID] = struct{}{}
	}

	env := graphEnvelope{
		Nodes:   make([]graphNode, 0, len(referenced)),
		Edges:   make([]graphEdge, 0, len(relations)),
		Pending: make([]graphPending, 0, len(pending)),
	}

	for _, n := range all {
		if _, ok := referenced[n.ID]; !ok {
			continue
		}
		env.Nodes = append(env.Nodes, graphNode{
			ID:          n.ID,
			Label:       n.Label,
			Type:        n.NodeType,
			Temperature: n.Temperature,
		})
	}

	for _, r := range relations {
		env.Edges = append(env.Edges, graphEdge{
			Source: r.SourceNodeID,
			Target: r.TargetNodeID,
			Weight: r.Weight,
			Origin: r.Origin,
		})
	}

	for _, p := range pending {
		env.Pending = append(env.Pending, graphPending{
			Source:       p.SourceNodeID,
			TargetLabel:  p.TargetLabel,
			TargetSource: p.TargetSource,
			TargetIDHint: p.TargetIDHint,
			Weight:       p.Weight,
			Origin:       p.Origin,
		})
	}

	body, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal: graph: %w", err)
	}
	return body, nil
}

func (d *Deps) HandleGraph(ctx context.Context, _ *gomcp.ReadResourceRequest) (*gomcp.ReadResourceResult, error) {
	body, err := d.graphBody(ctx)
	if err != nil {
		return nil, err
	}

	return &gomcp.ReadResourceResult{
		Contents: []*gomcp.ResourceContents{{
			URI:      GraphURI,
			MIMEType: mimeJSON,
			Text:     string(body),
		}},
	}, nil
}
