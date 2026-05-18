package tools

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/pkg/diff"
	"github.com/radimsem/remindb/pkg/emitter"
	"github.com/radimsem/remindb/pkg/parser"
	"github.com/radimsem/remindb/pkg/store"
)

type ForgetInput struct {
	NodeID string `json:"node_id" jsonschema:"ID of the node to delete"`
	Mode   string `json:"mode,omitempty" jsonschema:"Removal mode: 'strict' (default; refuses to delete a node with children), 'cascade' (also removes all descendants), or 'reparent' (promotes children to the target's parent, or to roots if the target has no parent)"`
}

func (d *Deps) HandleForget(ctx context.Context, _ *gomcp.CallToolRequest, input ForgetInput) (_ *gomcp.CallToolResult, _ any, err error) {
	defer d.logCall("MemoryForget", &err, time.Now(), "node_id", input.NodeID, "mode", input.Mode)

	mode, err := store.ParseDeleteMode(input.Mode)
	if err != nil {
		return nil, nil, err
	}

	d.Store.OpMu.Lock()
	defer d.Store.OpMu.Unlock()

	target, err := d.Store.GetNode(ctx, input.NodeID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, fmt.Errorf("node_id not found: %s", input.NodeID)
		}
		return nil, nil, fmt.Errorf("failed to fetch: node %s: %w", input.NodeID, err)
	}

	children, err := d.Store.GetChildren(ctx, input.NodeID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load: children of %s: %w", input.NodeID, err)
	}

	roots, deltas, err := d.buildForgetDeltas(ctx, target, children, mode)
	if err != nil {
		return nil, nil, err
	}

	if err := emitter.Emit(ctx, d.Store,
		emitter.WithRoots(roots),
		emitter.WithDeltas(deltas),
		emitter.WithCursorHash(diff.CursorHashForDeltas(deltas)),
		emitter.WithMessage("forget:"+mode.String()+":"+input.NodeID),
	); err != nil {
		return nil, nil, fmt.Errorf("failed to forget: %w", err)
	}

	d.touchSnapshot()

	msg := fmt.Sprintf("forgot node %s (mode=%s, %d affected)", input.NodeID, mode, len(deltas))
	return textResult(msg), nil, nil
}

func (d *Deps) buildForgetDeltas(ctx context.Context, target *store.Node, children []*store.Node, mode store.DeleteMode) ([]*parser.ContextNode, []diff.Delta, error) {
	switch mode {
	case store.DeleteStrict:
		if len(children) > 0 {
			return nil, nil, fmt.Errorf("node %s has %d children; pass mode=cascade or mode=reparent", target.ID, len(children))
		}
		return nil, []diff.Delta{remDelta(target)}, nil

	case store.DeleteCascade:
		descendants, err := d.Store.GetDescendants(ctx, target.ID, math.MaxInt32)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load: descendants of %s: %w", target.ID, err)
		}

		deltas := make([]diff.Delta, 0, 1+len(descendants))
		deltas = append(deltas, remDelta(target))

		for _, desc := range descendants {
			deltas = append(deltas, remDelta(desc))
		}
		return nil, deltas, nil

	case store.DeleteReparent:
		roots := make([]*parser.ContextNode, 0, len(children))
		deltas := make([]diff.Delta, 0, 1+len(children))

		for _, c := range children {
			updated := contextFromStoreNode(c)
			updated.ParentID = target.ParentID
			roots = append(roots, updated)
			deltas = append(deltas, diff.Delta{
				NodeID:     c.ID,
				Op:         diff.OpMod,
				OldHash:    c.ContentHash,
				NewHash:    c.ContentHash,
				OldContent: c.Content,
				NewContent: c.Content,
			})
		}

		deltas = append(deltas, remDelta(target))
		return roots, deltas, nil
	}

	return nil, nil, fmt.Errorf("unknown delete mode: %d", mode)
}

func remDelta(n *store.Node) diff.Delta {
	return diff.Delta{
		NodeID:     n.ID,
		Op:         diff.OpRem,
		OldHash:    n.ContentHash,
		OldContent: n.Content,
	}
}

func contextFromStoreNode(n *store.Node) *parser.ContextNode {
	return &parser.ContextNode{
		ID:          n.ID,
		ParentID:    n.ParentID,
		SourceFile:  n.SourceFile,
		NodeType:    parser.NodeType(n.NodeType),
		Depth:       n.Depth,
		Label:       n.Label,
		Content:     n.Content,
		Format:      n.Format,
		TokenCount:  n.TokenCount,
		ContentHash: n.ContentHash,
	}
}
