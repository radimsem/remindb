package tools

import (
	"context"
	"fmt"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/internal/contentid"
	"github.com/radimsem/remindb/internal/tokens"
	"github.com/radimsem/remindb/pkg/diff"
	"github.com/radimsem/remindb/pkg/emitter"
	"github.com/radimsem/remindb/pkg/parser"
)

type WriteInput struct {
	Anchor  string `json:"anchor,omitempty" jsonschema:"Existing node ID to update (empty to create new)"`
	Payload string `json:"payload" jsonschema:"Content to write"`
}

func (d *Deps) HandleWrite(ctx context.Context, _ *gomcp.CallToolRequest, input WriteInput) (*gomcp.CallToolResult, any, error) {
	contentHash, generatedID := contentid.Hash(input.Payload)
	nodeID := input.Anchor
	if nodeID == "" {
		nodeID = generatedID
	}

	tokenCount := tokens.Estimate(input.Payload)
	label := firstLine(input.Payload, 80)

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
			Content:     input.Payload,
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
			Content:     input.Payload,
			Format:      parser.FormatPlain,
			TokenCount:  tokenCount,
			ContentHash: contentHash,
		}
	}

	roots := []*parser.ContextNode{node}
	deltas := diff.Diff(roots, prev)
	cursorHash := diff.CursorHash(roots)

	if err := emitter.Emit(ctx, d.Store, roots, deltas, cursorHash, "write:"+nodeID); err != nil {
		return nil, nil, fmt.Errorf("failed to write: %w", err)
	}

	msg := fmt.Sprintf("wrote node %s (%d tokens)", nodeID, tokenCount)
	return &gomcp.CallToolResult{
		Content: []gomcp.Content{&gomcp.TextContent{Text: msg}},
	}, nil, nil
}
