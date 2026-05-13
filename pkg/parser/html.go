package parser

import (
	"bytes"
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type HtmlParser struct{}

// Internal placeholder for an explicit line break (<br>) emitted by writeInline.
const brSentinel = '\x01'

func parseHtml(path string, data []byte) ([]*ContextNode, error) {
	return HtmlParser{}.parse(path, data)
}

// Parse HTML source into a ContextNode tree.
func (p HtmlParser) parse(path string, data []byte) ([]*ContextNode, error) {
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to parse: html %s: %w", path, err)
	}

	_, out := p.walk(doc, path, []frame{{level: 0}}, nil)
	return out, nil
}

func (p HtmlParser) walk(n *html.Node, path string, stack []frame, out []*ContextNode) ([]frame, []*ContextNode) {
	if n.Type == html.CommentNode {
		return stack, out
	}

	if n.Type != html.ElementNode {
		return p.walkChildren(n, path, stack, out)
	}

	switch n.DataAtom {
	case atom.Script, atom.Style, atom.Head:
		return stack, out

	case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
		return p.attachHeading(n, path, stack, out)

	case atom.Ul, atom.Ol, atom.Dl:
		return stack, p.attachList(n, path, stack, out)

	case atom.Table:
		return stack, p.attachTable(n, path, stack, out)

	case atom.Pre:
		return stack, p.attachCode(n, path, stack, out)

	case atom.Math:
		return stack, p.attachMath(n, path, stack, out)

	case atom.P, atom.Blockquote, atom.Figcaption:
		return stack, p.attachText(n, path, stack, out)

	case atom.Img, atom.Video, atom.Audio, atom.Iframe, atom.Embed, atom.Object:
		return stack, p.attachEmbed(n, path, stack, out)

	case atom.Svg, atom.Canvas:
		return stack, p.attachInlineMarkup(n, path, stack, out)
	}

	return p.walkChildren(n, path, stack, out)
}

func (p HtmlParser) walkChildren(n *html.Node, path string, stack []frame, out []*ContextNode) ([]frame, []*ContextNode) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		stack, out = p.walk(c, path, stack, out)
	}
	return stack, out
}

// Emit a heading node and push a new section frame at its level.
func (p HtmlParser) attachHeading(n *html.Node, path string, stack []frame, out []*ContextNode) ([]frame, []*ContextNode) {
	level := int(n.Data[1] - '0')
	content := p.inlineText(n)
	if content == "" {
		return stack, out
	}

	cn := &ContextNode{SourceFile: path, NodeType: NodeHeading, Content: content, Format: FormatPlain}
	stack = popSections(stack, level)

	out = attachNode(stack, out, cn)
	stack = append(stack, frame{node: cn, level: level})

	return stack, out
}

func (p HtmlParser) attachList(n *html.Node, path string, stack []frame, out []*ContextNode) []*ContextNode {
	content := p.listItems(n)
	if content == "" {
		return out
	}

	cn := &ContextNode{SourceFile: path, NodeType: NodeList, Content: content, Format: FormatPlain}
	return attachNode(stack, out, cn)
}

func (p HtmlParser) attachTable(n *html.Node, path string, stack []frame, out []*ContextNode) []*ContextNode {
	content := p.tableRows(n)
	if content == "" {
		return out
	}

	cn := &ContextNode{SourceFile: path, NodeType: NodeTable, Content: content, Format: FormatPlain}
	return attachNode(stack, out, cn)
}

func (p HtmlParser) attachCode(n *html.Node, path string, stack []frame, out []*ContextNode) []*ContextNode {
	content := strings.TrimRight(p.literalText(n), "\n")
	if content == "" {
		return out
	}

	cn := &ContextNode{SourceFile: path, NodeType: NodeCode, Content: content, Format: FormatPlain}
	return attachNode(stack, out, cn)
}

func (p HtmlParser) attachMath(n *html.Node, path string, stack []frame, out []*ContextNode) []*ContextNode {
	xml := p.serializeNode(n)
	if xml == "" {
		return out
	}

	content, format := xml, FormatMathml
	if latex, ok := mathmlToLatex(n); ok && beatsXml(xml, latex) {
		content = latex
		format = FormatLatex
	}

	cn := &ContextNode{SourceFile: path, NodeType: NodeCode, Content: content, Format: format}
	return attachNode(stack, out, cn)
}

func (p HtmlParser) attachText(n *html.Node, path string, stack []frame, out []*ContextNode) []*ContextNode {
	content := p.inlineText(n)
	if content == "" {
		return out
	}

	cn := &ContextNode{SourceFile: path, NodeType: NodeText, Content: content, Format: FormatPlain}
	return attachNode(stack, out, cn)
}

func (p HtmlParser) attachEmbed(n *html.Node, path string, stack []frame, out []*ContextNode) []*ContextNode {
	cn := p.embedNode(path, n)
	if cn == nil {
		return out
	}
	return attachNode(stack, out, cn)
}

// Preserve <svg>/<canvas> as "attrs\n<inner markup>".
func (p HtmlParser) attachInlineMarkup(n *html.Node, path string, stack []frame, out []*ContextNode) []*ContextNode {
	content := p.inlineMarkupContent(n)
	if content == "" {
		return out
	}

	cn := &ContextNode{SourceFile: path, NodeType: NodeCode, Content: content, Format: n.Data}
	return attachNode(stack, out, cn)
}

// Render n as `attr1="v1" attr2="v2"\n<inner-markup>`; either side may be empty.
func (p HtmlParser) inlineMarkupContent(n *html.Node) string {
	var attrs strings.Builder

	for i, a := range n.Attr {
		if i > 0 {
			attrs.WriteByte(' ')
		}

		attrs.WriteString(a.Key)
		attrs.WriteString(`="`)
		attrs.WriteString(a.Val)
		attrs.WriteByte('"')
	}

	var inner strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if err := html.Render(&inner, c); err != nil {
			return ""
		}
	}
	innerStr := strings.TrimSpace(inner.String())

	switch {
	case attrs.Len() > 0 && innerStr != "":
		return attrs.String() + "\n" + innerStr
	case attrs.Len() > 0:
		return attrs.String()
	default:
		return innerStr
	}
}

// Concatenate visible text in n, rendering <a>/<img> as [text](dest) / ![alt](dest).
func (p HtmlParser) inlineText(n *html.Node) string {
	var sb strings.Builder
	p.writeInline(&sb, n)

	return normalizeWhitespace(sb.String())
}

func (p HtmlParser) writeInline(sb *strings.Builder, n *html.Node) {
	switch n.Type {
	case html.TextNode:
		sb.WriteString(n.Data)
		return

	case html.ElementNode:
		switch n.DataAtom {
		case atom.Script, atom.Style:
			return
		case atom.Br:
			sb.WriteByte(brSentinel)
			return
		case atom.A:
			p.writeLink(sb, n)
			return
		case atom.Img:
			p.writeImage(sb, n)
			return
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		p.writeInline(sb, c)
	}
}

func (p HtmlParser) writeLink(sb *strings.Builder, n *html.Node) {
	sb.WriteByte('[')
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		p.writeInline(sb, c)
	}

	sb.WriteString("](")
	sb.WriteString(getAttr(n, "href"))
	sb.WriteByte(')')
}

func (p HtmlParser) writeImage(sb *strings.Builder, n *html.Node) {
	sb.WriteString("![")
	sb.WriteString(getAttr(n, "alt"))
	sb.WriteString("](")
	sb.WriteString(getAttr(n, "src"))
	sb.WriteByte(')')
}

// Render each <li>/<dt>/<dd> as one "- text" line; nested lists indent by one tab per level.
func (p HtmlParser) listItems(n *html.Node) string {
	return strings.Join(p.collectListItems(n, nil, ""), "\n")
}

func (p HtmlParser) collectListItems(n *html.Node, lines []string, indent string) []string {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode {
			continue
		}

		switch c.DataAtom {
		case atom.Li, atom.Dt, atom.Dd:
			text, nested := p.splitItemContent(c)
			if text != "" {
				lines = append(lines, indent+"- "+text)
			}
			for _, list := range nested {
				lines = p.collectListItems(list, lines, indent+"\t")
			}
		case atom.Ul, atom.Ol, atom.Dl:
			lines = p.collectListItems(c, lines, indent)
		}
	}
	return lines
}

// Split an item's text from nested-list children so nesting renders at a deeper indent.
func (p HtmlParser) splitItemContent(li *html.Node) (string, []*html.Node) {
	var sb strings.Builder
	var nested []*html.Node

	for c := li.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode {
			switch c.DataAtom {
			case atom.Ul, atom.Ol, atom.Dl:
				nested = append(nested, c)
				continue
			}
		}
		p.writeInline(&sb, c)
	}

	return normalizeWhitespace(sb.String()), nested
}

// Render a table as tab-separated rows, header first.
func (p HtmlParser) tableRows(n *html.Node) string {
	return strings.Join(p.collectRows(n, nil), "\n")
}

func (p HtmlParser) collectRows(n *html.Node, rows []string) []string {
	if n.Type == html.ElementNode && n.DataAtom == atom.Tr {
		return append(rows, p.rowCells(n))
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		rows = p.collectRows(c, rows)
	}
	return rows
}

func (p HtmlParser) rowCells(tr *html.Node) string {
	var cells []string

	for c := tr.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode {
			continue
		}

		if c.DataAtom == atom.Td || c.DataAtom == atom.Th {
			cells = append(cells, p.inlineText(c))
		}
	}
	return strings.Join(cells, "\t")
}

// Concatenate raw text content without rendering inline markup or collapsing whitespace.
func (p HtmlParser) literalText(n *html.Node) string {
	var sb strings.Builder
	p.writeLiteral(&sb, n)

	return sb.String()
}

func (p HtmlParser) writeLiteral(sb *strings.Builder, n *html.Node) {
	if n.Type == html.TextNode {
		sb.WriteString(n.Data)
		return
	}
	if n.Type == html.ElementNode {
		switch n.DataAtom {
		case atom.Script, atom.Style:
			return
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		p.writeLiteral(sb, c)
	}
}

func (p HtmlParser) serializeNode(n *html.Node) string {
	var sb strings.Builder
	if err := html.Render(&sb, n); err != nil {
		return ""
	}
	return strings.TrimSpace(sb.String())
}

func (p HtmlParser) embedNode(path string, n *html.Node) *ContextNode {
	src := getAttr(n, "src")
	alt := getAttr(n, "alt")

	if alt == "" {
		alt = getAttr(n, "title")
	}
	if src == "" {
		src = fallbackSrc(n)
	}
	if src == "" {
		return nil
	}

	format := embedFormat(n.DataAtom)
	content := formatEmbedContent(n.DataAtom, alt, src)

	return &ContextNode{SourceFile: path, NodeType: NodeEmbed, Content: content, Format: format}
}

// Resolve src for elements that may carry it via a non-`src` attribute or a child.
func fallbackSrc(n *html.Node) string {
	switch n.DataAtom {
	case atom.Video, atom.Audio:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && c.DataAtom == atom.Source {
				if s := getAttr(c, "src"); s != "" {
					return s
				}
			}
		}
	case atom.Object:
		return getAttr(n, "data")
	}
	return ""
}

func formatEmbedContent(a atom.Atom, alt, src string) string {
	if a == atom.Img {
		return "![" + alt + "](" + src + ")"
	}
	return "[" + alt + "](" + src + ")"
}

func embedFormat(a atom.Atom) string {
	switch a {
	case atom.Img:
		return "image"
	case atom.Video:
		return "video"
	case atom.Audio:
		return "audio"
	case atom.Iframe:
		return "iframe"
	}
	return "embed"
}

func getAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func normalizeWhitespace(s string) string {
	var b strings.Builder
	b.Grow(len(s))

	pendingSpace := false
	afterBreak := false
	for _, r := range s {
		if r == brSentinel {
			b.WriteByte('\n')

			pendingSpace = false
			afterBreak = true
			continue
		}

		if unicode.IsSpace(r) {
			if !afterBreak {
				pendingSpace = true
			}
			continue
		}

		if pendingSpace && b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(r)

		pendingSpace = false
		afterBreak = false
	}

	return strings.TrimSpace(b.String())
}
