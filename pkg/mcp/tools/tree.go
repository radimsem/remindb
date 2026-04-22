package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/pkg/store"
)

type TreeInput struct {
	Root  string `json:"root,omitempty" jsonschema:"Root node ID (empty for full tree)"`
	Depth int    `json:"depth,omitempty" jsonschema:"Maximum depth to traverse (default 5)"`
}

func (d *Deps) HandleTree(ctx context.Context, _ *gomcp.CallToolRequest, input TreeInput) (_ *gomcp.CallToolResult, _ any, err error) {
	defer d.logCall("MemoryTree", &err, time.Now(), "root", input.Root, "depth", input.Depth)

	maxDepth := input.Depth
	if maxDepth <= 0 {
		maxDepth = 5
	}

	all, err := d.Store.GetAllNodes(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get tree: %w", err)
	}

	roots, childMap := store.BuildTree(all)

	if input.Root != "" {
		var root *store.Node
		for _, n := range all {
			if n.ID == input.Root {
				root = n
				break
			}
		}
		if root == nil {
			return nil, nil, fmt.Errorf("failed to get root: node %s not found", input.Root)
		}
		roots = []*store.Node{root}
	}

	var b strings.Builder
	for _, root := range roots {
		writeTreeNode(&b, childMap, root, 0, maxDepth)
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

func writeTreeNode(b *strings.Builder, children map[string][]*store.Node, n *store.Node, depth, maxDepth int) {
	indent := strings.Repeat("  ", depth)
	fmt.Fprintf(b, "%s[%s] %s (id=%s file=%s temp=%.2f tok=%d)\n", indent, n.NodeType, n.Label, n.ID, n.SourceFile, n.Temperature, n.TokenCount)

	if depth >= maxDepth {
		return
	}

	for _, child := range children[n.ID] {
		writeTreeNode(b, children, child, depth+1, maxDepth)
	}
}
