package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/pkg/store"
)

const (
	TreeURI            = "remindb://tree"
	TreeByRootTemplate = "remindb://tree/{rootId}{?depth}"
)

type treeNode struct {
	ID          string      `json:"id"`
	Type        string      `json:"type"`
	Label       string      `json:"label"`
	Depth       int         `json:"depth"`
	Tokens      int         `json:"tokens"`
	Temperature float64     `json:"temperature"`
	Source      string      `json:"source"`
	Children    []*treeNode `json:"children"`
}

type treeEnvelope struct {
	Roots []*treeNode `json:"roots"`
}

// Make an absolute source path relative to the compile root, mirroring MemoryTree.
func relSource(source, compileRoot string) string {
	if compileRoot == "" || !filepath.IsAbs(source) {
		return source
	}

	rel, err := filepath.Rel(compileRoot, source)
	if err != nil || strings.HasPrefix(rel, "..") {
		return source
	}

	return rel
}

func buildTreeNode(children map[string][]*store.Node, n *store.Node, compileRoot string, level, maxDepth int) *treeNode {
	tn := &treeNode{
		ID:          n.ID,
		Type:        n.NodeType,
		Label:       n.Label,
		Depth:       n.Depth,
		Tokens:      n.TokenCount,
		Temperature: n.Temperature,
		Source:      relSource(n.SourceFile, compileRoot),
		Children:    []*treeNode{},
	}

	if maxDepth > 0 && level >= maxDepth {
		return tn
	}

	for _, c := range children[n.ID] {
		tn.Children = append(tn.Children, buildTreeNode(children, c, compileRoot, level+1, maxDepth))
	}
	return tn
}

func (d *Deps) treeBody(ctx context.Context, rootID string, maxDepth int) ([]byte, error) {
	all, err := d.Store.GetAllNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get: tree nodes: %w", err)
	}

	roots, children := store.BuildTree(all)

	if rootID != "" {
		var root *store.Node
		for _, n := range all {
			if n.ID == rootID {
				root = n
				break
			}
		}

		if root == nil {
			return nil, fmt.Errorf("root node %s not found", rootID)
		}
		roots = []*store.Node{root}
	}

	compileRoot, _ := d.Store.GetLatestCompileRoot(ctx)

	env := treeEnvelope{Roots: make([]*treeNode, 0, len(roots))}
	for _, r := range roots {
		env.Roots = append(env.Roots, buildTreeNode(children, r, compileRoot, 0, maxDepth))
	}

	body, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal: tree: %w", err)
	}
	return body, nil
}

func (d *Deps) HandleTree(ctx context.Context, _ *gomcp.ReadResourceRequest) (*gomcp.ReadResourceResult, error) {
	body, err := d.treeBody(ctx, "", 0)
	if err != nil {
		return nil, err
	}

	return &gomcp.ReadResourceResult{
		Contents: []*gomcp.ResourceContents{{
			URI:      TreeURI,
			MIMEType: mimeJSON,
			Text:     string(body),
		}},
	}, nil
}

func (d *Deps) HandleTreeByRoot(ctx context.Context, req *gomcp.ReadResourceRequest) (*gomcp.ReadResourceResult, error) {
	u, err := url.Parse(req.Params.URI)
	if err != nil {
		return nil, fmt.Errorf("failed to parse: tree uri: %w", err)
	}

	rootID := strings.TrimPrefix(u.Path, "/")
	if rootID == "" {
		return nil, fmt.Errorf("tree uri missing root id")
	}

	maxDepth := 0
	if ds := u.Query().Get("depth"); ds != "" {
		maxDepth, err = strconv.Atoi(ds)
		if err != nil || maxDepth < 0 {
			return nil, fmt.Errorf("invalid depth %q", ds)
		}
	}

	body, err := d.treeBody(ctx, rootID, maxDepth)
	if err != nil {
		return nil, err
	}

	return &gomcp.ReadResourceResult{
		Contents: []*gomcp.ResourceContents{{
			URI:      req.Params.URI,
			MIMEType: mimeJSON,
			Text:     string(body),
		}},
	}, nil
}
