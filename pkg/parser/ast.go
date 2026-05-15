// Package parser turns source files (Markdown, HTML, YAML, JSON, TOON) into
// a unified ContextNode tree.
package parser

type NodeType string

const (
	NodeHeading  NodeType = "heading"
	NodeList     NodeType = "list"
	NodeTable    NodeType = "table"
	NodeCode     NodeType = "code"
	NodeText     NodeType = "text"
	NodeKV       NodeType = "kv"
	NodePreamble NodeType = "preamble"
	NodeEmbed    NodeType = "embed"
)

const (
	FormatPlain  = "plain"
	FormatToon   = "toon"
	FormatMathml = "mathml"
	FormatLatex  = "latex"
)

// Promote a nested map or array to its own ContextNode at or above this many entries; smaller structures stay inlined.
const MaxInlineFields = 5

type ContextNode struct {
	Children     []*ContextNode
	WikilinkRefs []WikilinkRef
	ID           string
	ParentID     string
	SourceFile   string
	Label        string
	Content      string
	ContentHash  string
	Format       string
	Temperature  *float64
	Depth        int
	TokenCount   int
	NodeType
}

func Flatten(roots []*ContextNode) []*ContextNode {
	return collectNodes(nil, roots)
}

func FlattenMap(roots []*ContextNode) map[string]*ContextNode {
	flat := Flatten(roots)
	m := make(map[string]*ContextNode, len(flat))
	for _, n := range flat {
		m[n.ID] = n
	}
	return m
}

func collectNodes(out []*ContextNode, nodes []*ContextNode) []*ContextNode {
	for _, n := range nodes {
		out = append(out, n)
		out = collectNodes(out, n.Children)
	}
	return out
}
