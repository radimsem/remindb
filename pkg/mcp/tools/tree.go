package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/internal/treewalk"
	"github.com/radimsem/remindb/pkg/store"
)

type TreeInput struct {
	Root  string `json:"root,omitempty" jsonschema:"Root node ID (empty for full tree)"`
	Depth int    `json:"depth,omitempty" jsonschema:"Maximum depth to traverse (default 5)"`
}

func (d *Deps) HandleTree(ctx context.Context, _ *gomcp.CallToolRequest, input TreeInput) (_ *gomcp.CallToolResult, _ any, err error) {
	defer d.logCall(ctx, "MemoryTree", &err, time.Now(), "root", input.Root, "depth", input.Depth)

	maxDepth := treewalk.ClampDepth(input.Depth, 5, treewalk.MaxDepth)

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

	compileRoot, _ := d.Store.GetLatestCompileRoot(ctx)

	var b strings.Builder
	for _, root := range roots {
		writeTree(&b, childMap, root, compileRoot, maxDepth)
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

func writeTree(b *strings.Builder, children map[string][]*store.Node, root *store.Node, compileRoot string, maxDepth int) {
	treewalk.Walk[struct{}](children, root, maxDepth, func(n, parent *store.Node, depth int, descend func() []struct{}) struct{} {
		indent := strings.Repeat("  ", depth)
		fmt.Fprintf(b, "%s[%s] %s (id=%s", indent, n.NodeType, n.Label, n.ID)

		parentSource := ""
		if parent != nil {
			parentSource = parent.SourceFile
		}
		if n.SourceFile != parentSource {
			fmt.Fprintf(b, " file=%s", treewalk.RelativeTo(n.SourceFile, compileRoot))
		}
		fmt.Fprintf(b, " temp=%.2f tok=%d)\n", n.Temperature, n.TokenCount)

		descend()
		return struct{}{}
	})
}
