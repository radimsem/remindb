package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/internal/treewalk"
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

func buildTreeJSON(children map[string][]*store.Node, root *store.Node, compileRoot string, maxDepth int) *treeNode {
	return treewalk.Walk[*treeNode](children, root, maxDepth, func(n, _ *store.Node, _ int, descend func() []*treeNode) *treeNode {
		kids := descend()
		if kids == nil {
			kids = []*treeNode{}
		}

		return &treeNode{
			ID:          n.ID,
			Type:        n.NodeType,
			Label:       n.Label,
			Depth:       n.Depth,
			Tokens:      n.TokenCount,
			Temperature: n.Temperature,
			Source:      treewalk.RelativeTo(n.SourceFile, compileRoot),
			Children:    kids,
		}
	})
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
		env.Roots = append(env.Roots, buildTreeJSON(children, r, compileRoot, maxDepth))
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
