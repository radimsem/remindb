package parser

import (
	"strings"

	"github.com/gomarkdown/markdown/ast"
	mdparser "github.com/gomarkdown/markdown/parser"
)

type frame struct {
	node  *ContextNode
	level int
}

// attachNode places n under the innermost open section on stack. If stack
// has only the sentinel root, n is returned as a new top-level entry of out
// with depth 1; otherwise n becomes a child of the top frame's node with
// depth one greater than its parent.
func attachNode(stack []frame, out []*ContextNode, n *ContextNode) []*ContextNode {
	top := stack[len(stack)-1]
	if top.node == nil {
		n.Depth = 1
		return append(out, n)
	}

	n.Depth = top.node.Depth + 1
	top.node.Children = append(top.node.Children, n)
	return out
}

// popSections trims stack back to the first frame whose heading level is
// less than level, so a new heading at that level attaches to the correct
// ancestor.
func popSections(stack []frame, level int) []frame {
	for len(stack) > 1 && stack[len(stack)-1].level >= level {
		stack = stack[:len(stack)-1]
	}
	return stack
}

// parseMarkdown turns Markdown source into a ContextNode tree. A preamble
// (YAML or TOML metadata block), when present, becomes the first top-level
// node; headings are then organized into sections by level — a heading of
// level N becomes the parent of all subsequent blocks until the next
// heading of level ≤ N.
func parseMarkdown(path string, data []byte) ([]*ContextNode, error) {
	front, body, kind := splitPreamble(data)

	var out []*ContextNode
	if kind != preambleNone {
		pn, err := preambleNode(path, front, kind)
		if err != nil {
			return nil, err
		}
		if pn != nil {
			out = append(out, pn)
		}
	}

	p := mdparser.NewWithExtensions(
		mdparser.CommonExtensions | mdparser.Tables | mdparser.FencedCode,
	)
	doc := p.Parse(body)

	stack := []frame{{level: 0}} // sentinel root

	for _, child := range doc.GetChildren() {
		switch block := child.(type) {
		case *ast.Heading:
			n := &ContextNode{
				SourceFile: path,
				NodeType:   NodeHeading,
				Content:    extractText(block),
			}

			stack = popSections(stack, block.Level)
			out = attachNode(stack, out, n)
			stack = append(stack, frame{node: n, level: block.Level})

		default:
			if n := nodeFromBlock(path, block); n != nil {
				out = attachNode(stack, out, n)
			}
		}
	}

	return out, nil
}

// nodeFromBlock maps a non-heading block-level AST node to a ContextNode.
// Returns nil when the block has no extractable content (e.g. a horizontal
// rule or a paragraph whose only children are whitespace).
func nodeFromBlock(path string, n ast.Node) *ContextNode {
	switch b := n.(type) {
	case *ast.CodeBlock:
		content := string(b.Literal)
		if lang := strings.TrimSpace(string(b.Info)); lang != "" {
			content = lang + "\n" + content
		}
		content = strings.TrimRight(content, "\n")
		if content == "" {
			return nil
		}
		return &ContextNode{SourceFile: path, NodeType: NodeCode, Content: content}

	case *ast.List:
		content := extractListText(b)
		if content == "" {
			return nil
		}
		return &ContextNode{SourceFile: path, NodeType: NodeList, Content: content}

	case *ast.Table:
		content := extractTableText(b)
		if content == "" {
			return nil
		}
		return &ContextNode{SourceFile: path, NodeType: NodeTable, Content: content}

	case *ast.HTMLBlock:
		content := strings.TrimSpace(string(b.Literal))
		if content == "" {
			return nil
		}
		return &ContextNode{SourceFile: path, NodeType: NodeText, Content: content}

	case *ast.HorizontalRule:
		return nil

	default:
		content := extractText(b)
		if content == "" {
			return nil
		}
		return &ContextNode{SourceFile: path, NodeType: NodeText, Content: content}
	}
}

// extractText concatenates all visible text inside n, converting inline
// breaks into spaces or newlines so adjacent text runs stay separated.
func extractText(n ast.Node) string {
	var sb strings.Builder
	ast.WalkFunc(n, func(node ast.Node, entering bool) ast.WalkStatus {
		if !entering {
			return ast.GoToNext
		}
		switch t := node.(type) {
		case *ast.Text:
			sb.Write(t.Literal)
		case *ast.Code:
			sb.Write(t.Literal)
		case *ast.Softbreak:
			sb.WriteByte(' ')
		case *ast.Hardbreak:
			sb.WriteByte('\n')
		}
		return ast.GoToNext
	})
	return strings.TrimSpace(sb.String())
}

// extractListText renders each top-level list item as one "- text" line.
// Nested list items are flattened into their parent item's text.
func extractListText(list *ast.List) string {
	var lines []string
	for _, item := range list.GetChildren() {
		if text := extractText(item); text != "" {
			lines = append(lines, "- "+text)
		}
	}
	return strings.Join(lines, "\n")
}

// extractTableText renders a table as tab-separated values, one row per line.
// The header row comes first, then body rows in source order.
func extractTableText(tbl *ast.Table) string {
	var rows []string
	ast.WalkFunc(tbl, func(node ast.Node, entering bool) ast.WalkStatus {
		if !entering {
			return ast.GoToNext
		}
		row, ok := node.(*ast.TableRow)
		if !ok {
			return ast.GoToNext
		}
		cells := make([]string, 0, len(row.GetChildren()))
		for _, cell := range row.GetChildren() {
			cells = append(cells, extractText(cell))
		}
		rows = append(rows, strings.Join(cells, "\t"))
		return ast.SkipChildren
	})
	return strings.Join(rows, "\n")
}
