package tools

import (
	"context"
	"fmt"
	"strings"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/pkg/store"
)

type TreeInput struct {
	Root  string `json:"root,omitempty" jsonschema:"Root node ID (empty for full tree)"`
	Depth int    `json:"depth,omitempty" jsonschema:"Maximum depth to traverse (default 5)"`
}

func (d *Deps) HandleTree(ctx context.Context, _ *gomcp.CallToolRequest, input TreeInput) (*gomcp.CallToolResult, any, error) {
	maxDepth := input.Depth
	if maxDepth <= 0 {
		maxDepth = 5
	}

	var roots []*store.Node
	var err error

	if input.Root == "" {
		roots, err = d.Store.GetRootNodes(ctx)
	} else {
		node, e := d.Store.GetNode(ctx, input.Root)
		if e != nil {
			return nil, nil, fmt.Errorf("failed to get root: %w", e)
		}

		roots = []*store.Node{node}
	}
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get tree: %w", err)
	}

	var b strings.Builder
	for _, root := range roots {
		if err := d.writeTreeNode(&b, ctx, root, 0, maxDepth); err != nil {
			return nil, nil, fmt.Errorf("failed to traverse tree: %w", err)
		}
	}

	if b.Len() == 0 {
		return &gomcp.CallToolResult{
			Content: []gomcp.Content{&gomcp.TextContent{Text: "empty tree"}},
		}, nil, nil
	}
	return &gomcp.CallToolResult{
		Content: []gomcp.Content{&gomcp.TextContent{Text: b.String()}},
	}, nil, nil
}

func (d *Deps) writeTreeNode(b *strings.Builder, ctx context.Context, n *store.Node, depth, maxDepth int) error {
	indent := strings.Repeat("  ", depth)
	fmt.Fprintf(b, "%s[%s] %s (%s)\n", indent, n.NodeType, n.Label, n.ID)

	if depth >= maxDepth {
		return nil
	}

	children, err := d.Store.GetChildren(ctx, n.ID)
	if err != nil {
		return err
	}

	for _, child := range children {
		if err := d.writeTreeNode(b, ctx, child, depth+1, maxDepth); err != nil {
			return err
		}
	}
	return nil
}
