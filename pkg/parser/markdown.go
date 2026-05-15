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

// Place n under the innermost open section and set its Depth from the parent.
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

// Pop frames whose heading level >= level so a new heading attaches to the correct ancestor.
func popSections(stack []frame, level int) []frame {
	for len(stack) > 1 && stack[len(stack)-1].level >= level {
		stack = stack[:len(stack)-1]
	}
	return stack
}

// Parse Markdown source into a ContextNode tree: preamble first (if any), then heading-organized body.
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

	exts := mdparser.CommonExtensions &^ (mdparser.MathJax | mdparser.Strikethrough)
	p := mdparser.NewWithExtensions(exts)
	doc := p.Parse(body)

	stack := []frame{{level: 0}} // sentinel root

	for _, child := range doc.GetChildren() {
		switch block := child.(type) {
		case *ast.Heading:
			content, refs := extractText(block)
			n := &ContextNode{
				SourceFile:   path,
				NodeType:     NodeHeading,
				Content:      content,
				Format:       FormatPlain,
				WikilinkRefs: refs,
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

// Map a non-heading block-level AST node to a ContextNode, or nil if no extractable content.
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
		return &ContextNode{SourceFile: path, NodeType: NodeCode, Content: content, Format: FormatPlain}

	case *ast.List:
		content, refs := extractListText(b)
		if content == "" {
			return nil
		}
		return &ContextNode{SourceFile: path, NodeType: NodeList, Content: content, Format: FormatPlain, WikilinkRefs: refs}

	case *ast.Table:
		content, refs := extractTableText(b)
		if content == "" {
			return nil
		}
		return &ContextNode{SourceFile: path, NodeType: NodeTable, Content: content, Format: FormatPlain, WikilinkRefs: refs}

	case *ast.HTMLBlock:
		content := strings.TrimSpace(string(b.Literal))
		if content == "" {
			return nil
		}
		return &ContextNode{SourceFile: path, NodeType: NodeText, Content: content, Format: FormatPlain}

	case *ast.HorizontalRule:
		return nil

	default:
		content, refs := extractText(b)
		if content == "" {
			return nil
		}
		return &ContextNode{SourceFile: path, NodeType: NodeText, Content: content, Format: FormatPlain, WikilinkRefs: refs}
	}
}

func extractText(n ast.Node) (string, []WikilinkRef) {
	var sb strings.Builder
	var refs []WikilinkRef

	ast.WalkFunc(n, func(node ast.Node, entering bool) ast.WalkStatus {
		switch t := node.(type) {
		case *ast.Text:
			if entering {
				rewritten, r := ExtractWikilinks(string(t.Literal))
				sb.WriteString(rewritten)
				refs = append(refs, r...)
			}
		case *ast.Code:
			if entering {
				sb.Write(t.Literal)
			}
		case *ast.Softbreak:
			if entering {
				sb.WriteByte(' ')
			}
		case *ast.Hardbreak:
			if entering {
				sb.WriteByte('\n')
			}
		case *ast.Link:
			if entering {
				sb.WriteByte('[')
			} else {
				writeLinkTail(&sb, t.Destination, t.Title)
			}
		case *ast.Image:
			if entering {
				sb.WriteString("![")
			} else {
				writeLinkTail(&sb, t.Destination, t.Title)
			}
		}
		return ast.GoToNext
	})
	return strings.TrimSpace(sb.String()), refs
}

func writeLinkTail(sb *strings.Builder, dest, title []byte) {
	sb.WriteString("](")
	sb.Write(dest)

	if len(title) > 0 {
		sb.WriteString(` "`)
		sb.Write(title)
		sb.WriteByte('"')
	}
	sb.WriteByte(')')
}

// Render each top-level list item as one "- text" line, flattening nested items.
func extractListText(list *ast.List) (string, []WikilinkRef) {
	var lines []string
	var refs []WikilinkRef

	for _, item := range list.GetChildren() {
		text, r := extractText(item)
		if text == "" {
			continue
		}

		lines = append(lines, "- "+text)
		refs = append(refs, r...)
	}
	return strings.Join(lines, "\n"), refs
}

// Render a table as tab-separated rows, header first.
func extractTableText(tbl *ast.Table) (string, []WikilinkRef) {
	var rows []string
	var refs []WikilinkRef

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
			text, r := extractText(cell)
			cells = append(cells, text)
			refs = append(refs, r...)
		}

		rows = append(rows, strings.Join(cells, "\t"))
		return ast.SkipChildren
	})
	return strings.Join(rows, "\n"), refs
}
