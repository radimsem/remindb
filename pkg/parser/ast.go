// Package parser turns source files (Markdown, YAML, JSON) into a unified
// ContextNode tree. The tree shape is identical across formats so later
// pipeline stages (transformer, diff, emitter) never need to branch on
// input format.
package parser

// NodeType classifies a ContextNode by its semantic role in the source.
// Values match the node_type column in the SQLite schema (VARCHAR(16)).
type NodeType string

const (
	NodeHeading     NodeType = "heading"
	NodeList        NodeType = "list"
	NodeTable       NodeType = "table"
	NodeCode        NodeType = "code"
	NodeText        NodeType = "text"
	NodeKV          NodeType = "kv"
	NodeFrontmatter NodeType = "frontmatter"
)

// ContextNode is one unit of content extracted from a source file. Parsers
// populate SourceFile, NodeType, Depth, Content, and Children. The remaining
// fields (ID, ParentID, Label, ContentHash, TokenCount) are filled in by
// later pipeline stages.
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
