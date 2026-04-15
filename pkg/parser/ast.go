// Package parser turns source files (Markdown, YAML, JSON) into a unified
// ContextNode tree.
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
)

const (
	FormatPlain = "plain"
	FormatToon  = "toon"
)

// MaxInlineFields is the threshold for promoting a nested map or array to its
// own ContextNode. Smaller structures stay inlined in the parent's Content.
const MaxInlineFields = 5

type ContextNode struct {
	Children    []*ContextNode
	ID          string
	ParentID    string
	SourceFile  string
	Label       string
	Content     string
	ContentHash string
	Format      string
	Depth       int
	TokenCount  int
	NodeType
}
