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

type ContextNode struct {
	Children    []*ContextNode
	ID          string
	ParentID    string
	SourceFile  string
	Label       string
	Content     string
	ContentHash string
	Depth       int
	TokenCount  int
	NodeType
}
