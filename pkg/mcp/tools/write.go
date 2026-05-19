package tools

import (
	"context"
	"fmt"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/internal/contentid"
	"github.com/radimsem/remindb/internal/tokens"
	"github.com/radimsem/remindb/pkg/diff"
	"github.com/radimsem/remindb/pkg/parser"
)

type WriteInput struct {
	Anchor  string `json:"anchor,omitempty" jsonschema:"Existing node ID to update (empty to create new)"`
	Payload string `json:"payload" jsonschema:"Content to write"`
}

func (d *Deps) HandleWrite(ctx context.Context, _ *gomcp.CallToolRequest, input WriteInput) (_ *gomcp.CallToolResult, _ any, err error) {
	defer d.logCall(ctx, "MemoryWrite", &err, time.Now(), "anchor", input.Anchor, "payload_bytes", len(input.Payload))

	d.Store.OpMu.Lock()
	defer d.Store.OpMu.Unlock()

	payload, hits := d.Redactor.Scrub(input.Payload)
	d.logRedaction("MemoryWrite", hits)

	contentHash := contentid.ContentHash(payload)
	nodeID := input.Anchor
	if nodeID == "" {
		nodeID = contentid.IdentifyPayload("mcp:write", payload)
	}

	tokenCount := tokens.Estimate(payload)
	label := firstLine(payload, 80)

	prev := make(map[string]diff.NodeState)
	existing, _ := d.Store.GetNode(ctx, nodeID)

	var node *parser.ContextNode
	if existing != nil {
		// Update: preserve original metadata, only change content fields.
		prev[nodeID] = diff.NodeState{Hash: existing.ContentHash, Content: existing.Content}
		node = &parser.ContextNode{
			ID:          existing.ID,
			ParentID:    existing.ParentID,
			SourceFile:  existing.SourceFile,
			NodeType:    parser.NodeType(existing.NodeType),
			Depth:       existing.Depth,
			Label:       label,
			Content:     payload,
			Format:      existing.Format,
			TokenCount:  tokenCount,
			ContentHash: contentHash,
		}
	} else {
		// Create: new text node with defaults.
		node = &parser.ContextNode{
			ID:          nodeID,
			SourceFile:  "mcp:write",
			NodeType:    parser.NodeText,
			Depth:       1,
			Label:       label,
			Content:     payload,
			Format:      parser.FormatPlain,
			TokenCount:  tokenCount,
			ContentHash: contentHash,
		}
	}

	if err := d.emitNodeChange(ctx, node, prev, "write:"+nodeID); err != nil {
		return nil, nil, fmt.Errorf("failed to write: %w", err)
	}

	msg := fmt.Sprintf("wrote node %s (%d tokens)", nodeID, tokenCount)
	return &gomcp.CallToolResult{
		Content: []gomcp.Content{&gomcp.TextContent{Text: msg}},
	}, nil, nil
}
