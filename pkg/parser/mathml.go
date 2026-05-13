package parser

import (
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
)

// Minimum byte-savings fraction LaTeX must beat over the raw MathML XML before it's preferred.
const MathmlSavingsThreshold = 0.15

// Tabular cell separator inside a \begin{matrix}...\end{matrix} body.
const matrixColumnSeparator = " & "

// Tabular row separator inside a \begin{matrix}...\end{matrix} body.
const matrixRowSeparator = ` \\ `

const (
	fencedDefaultOpen       = "("
	fencedDefaultClose      = ")"
	fencedDefaultSeparators = ","
)

// LaTeX command characters disallowed inside the literal payload of <ms>/<mtext>.
const latexSpecialChars = `{}\$&%#_^`

type mathmlConverter struct{}

// Convert a MathML element subtree to LaTeX.
func mathmlToLatex(n *html.Node) (string, bool) {
	return mathmlConverter{}.convert(n)
}

// Decide whether a LaTeX form saves enough bytes over the raw MathML XML to be worth substituting.
func beatsXml(xml, latex string) bool {
	if len(xml) == 0 {
		return false
	}

	saved := float64(len(xml)-len(latex)) / float64(len(xml))
	return saved >= MathmlSavingsThreshold
}

// Map a single MathML node to LaTeX; dispatch is by lowercased local tag name.
func (c mathmlConverter) convert(n *html.Node) (string, bool) {
	if n == nil {
		return "", false
	}
	if n.Type == html.TextNode {
		return strings.TrimSpace(n.Data), true
	}
	if n.Type != html.ElementNode {
		return "", true
	}

	switch strings.ToLower(n.Data) {
	case "math", "mrow", "mpadded", "mstyle", "maction":
		return c.convertChildren(n)
	case "mi":
		return c.convertIdent(n)
	case "mn":
		return c.convertNumber(n)
	case "mo":
		return c.convertOp(n)
	case "ms":
		return c.convertString(n)
	case "mtext":
		return c.convertText(n)
	case "mspace":
		return `\,`, true
	case "mfrac":
		return c.convertFrac(n)
	case "msqrt":
		return c.convertSqrt(n)
	case "mroot":
		return c.convertRoot(n)
	case "msup":
		return c.convertSup(n)
	case "msub":
		return c.convertSub(n)
	case "msubsup":
		return c.convertSubsup(n)
	case "mover":
		return c.convertOver(n)
	case "munder":
		return c.convertUnder(n)
	case "munderover":
		return c.convertUnderover(n)
	case "mfenced":
		return c.convertFenced(n)
	case "mtable":
		return c.convertTable(n)
	case "mtr":
		return c.convertTr(n)
	case "mtd":
		return c.convertChildren(n)
	case "mphantom":
		return c.convertPhantom(n)
	case "menclose":
		return c.convertEnclose(n)
	case "semantics":
		return c.convertSemantics(n)
	}

	return "", false
}

// Return only element-typed direct children, skipping whitespace text nodes and comments.
func (c mathmlConverter) elementChildren(n *html.Node) []*html.Node {
	var out []*html.Node

	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		if ch.Type == html.ElementNode {
			out = append(out, ch)
		}
	}

	return out
}

// Convert every child of n (text and element) and join the non-empty results with single spaces.
func (c mathmlConverter) convertChildren(n *html.Node) (string, bool) {
	var parts []string

	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		if ch.Type == html.TextNode {
			t := strings.TrimSpace(ch.Data)
			if t != "" {
				parts = append(parts, t)
			}
			continue
		}

		if ch.Type != html.ElementNode {
			continue
		}

		s, ok := c.convert(ch)
		if !ok {
			return "", false
		}

		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " "), true
}

// Concatenate every text descendant of n, preserving order.
func (c mathmlConverter) leafText(n *html.Node) string {
	var sb strings.Builder
	c.collectText(n, &sb)

	return strings.TrimSpace(sb.String())
}

func (c mathmlConverter) collectText(n *html.Node, sb *strings.Builder) {
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		if ch.Type == html.TextNode {
			sb.WriteString(ch.Data)
			continue
		}

		c.collectText(ch, sb)
	}
}

func (c mathmlConverter) convertIdent(n *html.Node) (string, bool) {
	raw := c.leafText(n)

	if raw == "" {
		return "", true
	}
	if fn, ok := functionLatex[raw]; ok {
		return fn, true
	}

	return c.mapRunes(raw), true
}

func (c mathmlConverter) convertNumber(n *html.Node) (string, bool) {
	return c.leafText(n), true
}

func (c mathmlConverter) convertOp(n *html.Node) (string, bool) {
	raw := c.leafText(n)
	if raw == "" {
		return "", true
	}

	return c.mapRunes(raw), true
}

func (c mathmlConverter) convertString(n *html.Node) (string, bool) {
	return c.convertTextual(n, `\text{"`, `"}`)
}

func (c mathmlConverter) convertText(n *html.Node) (string, bool) {
	return c.convertTextual(n, `\text{`, `}`)
}

func (c mathmlConverter) convertTextual(n *html.Node, openWrap, closeWrap string) (string, bool) {
	s := c.leafText(n)
	if s == "" {
		return "", true
	}

	if strings.ContainsAny(s, latexSpecialChars) {
		return "", false
	}
	return openWrap + s + closeWrap, true
}

// Replace each rune via unicodeLatex; runes not in the map pass through verbatim.
func (c mathmlConverter) mapRunes(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if mapped, ok := unicodeLatex[r]; ok {
			sb.WriteString(mapped)
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

func isAsciiLetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

// True for a single-rune string or a LaTeX command like `\alpha`.
func isSimpleAtom(s string) bool {
	if s == "" {
		return false
	}
	if utf8.RuneCountInString(s) == 1 {
		return true
	}

	if s[0] == '\\' {
		for i := 1; i < len(s); i++ {
			if !isAsciiLetter(rune(s[i])) {
				return false
			}
		}
		return true
	}
	return false
}

// Brace a sub-expression only when it isn't already a single atomic token.
func wrapAtom(s string) string {
	if isSimpleAtom(s) {
		return s
	}
	return "{" + s + "}"
}

func (c mathmlConverter) convertFrac(n *html.Node) (string, bool) {
	children := c.elementChildren(n)
	if len(children) != 2 {
		return "", false
	}

	num, ok := c.convert(children[0])
	if !ok {
		return "", false
	}

	den, ok := c.convert(children[1])
	if !ok {
		return "", false
	}
	return `\frac{` + num + "}{" + den + "}", true
}

func (c mathmlConverter) convertSqrt(n *html.Node) (string, bool) {
	inner, ok := c.convertChildren(n)
	if !ok {
		return "", false
	}

	return `\sqrt{` + inner + "}", true
}

func (c mathmlConverter) convertRoot(n *html.Node) (string, bool) {
	children := c.elementChildren(n)
	if len(children) != 2 {
		return "", false
	}

	radicand, ok := c.convert(children[0])
	if !ok {
		return "", false
	}

	index, ok := c.convert(children[1])
	if !ok {
		return "", false
	}
	return `\sqrt[` + index + "]{" + radicand + "}", true
}

func (c mathmlConverter) convertSub(n *html.Node) (string, bool) {
	base, sub, ok := c.convertScriptPair(n)
	if !ok {
		return "", false
	}

	return wrapAtom(base) + "_{" + sub + "}", true
}

func (c mathmlConverter) convertSup(n *html.Node) (string, bool) {
	base, sup, ok := c.convertScriptPair(n)
	if !ok {
		return "", false
	}

	return wrapAtom(base) + "^{" + sup + "}", true
}

func (c mathmlConverter) convertScriptPair(n *html.Node) (string, string, bool) {
	children := c.elementChildren(n)
	if len(children) != 2 {
		return "", "", false
	}

	base, ok := c.convert(children[0])
	if !ok {
		return "", "", false
	}

	script, ok := c.convert(children[1])
	if !ok {
		return "", "", false
	}
	return base, script, true
}

func (c mathmlConverter) convertSubsup(n *html.Node) (string, bool) {
	children := c.elementChildren(n)
	if len(children) != 3 {
		return "", false
	}

	base, ok := c.convert(children[0])
	if !ok {
		return "", false
	}

	sub, ok := c.convert(children[1])
	if !ok {
		return "", false
	}

	sup, ok := c.convert(children[2])
	if !ok {
		return "", false
	}
	return wrapAtom(base) + "_{" + sub + "}^{" + sup + "}", true
}

func (c mathmlConverter) convertUnder(n *html.Node) (string, bool) {
	return c.convertStacked(n, "_", `\underset`)
}

func (c mathmlConverter) convertOver(n *html.Node) (string, bool) {
	return c.convertStacked(n, "^", `\overset`)
}

// Render a two-child stacked construct (munder or mover).
func (c mathmlConverter) convertStacked(n *html.Node, position, cmd string) (string, bool) {
	children := c.elementChildren(n)
	if len(children) != 2 {
		return "", false
	}

	base, ok := c.convert(children[0])
	if !ok {
		return "", false
	}

	mark, ok := c.convert(children[1])
	if !ok {
		return "", false
	}

	if limitOperators[base] {
		return base + position + "{" + mark + "}", true
	}
	return cmd + "{" + mark + "}{" + base + "}", true
}

func (c mathmlConverter) convertUnderover(n *html.Node) (string, bool) {
	children := c.elementChildren(n)
	if len(children) != 3 {
		return "", false
	}

	base, ok := c.convert(children[0])
	if !ok {
		return "", false
	}

	under, ok := c.convert(children[1])
	if !ok {
		return "", false
	}

	over, ok := c.convert(children[2])
	if !ok {
		return "", false
	}

	if limitOperators[base] {
		return base + "_{" + under + "}^{" + over + "}", true
	}
	return `\underset{` + under + `}{\overset{` + over + "}{" + base + "}}", true
}

func (c mathmlConverter) convertPhantom(n *html.Node) (string, bool) {
	inner, ok := c.convertChildren(n)
	if !ok {
		return "", false
	}

	return `\phantom{` + inner + "}", true
}

func (c mathmlConverter) convertEnclose(n *html.Node) (string, bool) {
	inner, ok := c.convertChildren(n)
	if !ok {
		return "", false
	}

	return `\boxed{` + inner + "}", true
}

// Emit the first presentation child of <semantics>, skipping <annotation> and <annotation-xml> siblings.
func (c mathmlConverter) convertSemantics(n *html.Node) (string, bool) {
	for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
		if ch.Type != html.ElementNode {
			continue
		}

		name := strings.ToLower(ch.Data)
		if name == "annotation" || name == "annotation-xml" {
			continue
		}

		return c.convert(ch)
	}
	return "", false
}

func (c mathmlConverter) convertFenced(n *html.Node) (string, bool) {
	open := getAttr(n, "open")
	if open == "" {
		open = fencedDefaultOpen
	}

	closer := getAttr(n, "close")
	if closer == "" {
		closer = fencedDefaultClose
	}

	sepAttr := getAttr(n, "separators")
	if sepAttr == "" {
		sepAttr = fencedDefaultSeparators
	}

	children := c.elementChildren(n)
	parts := make([]string, 0, len(children))
	for _, k := range children {
		s, ok := c.convert(k)
		if !ok {
			return "", false
		}

		parts = append(parts, s)
	}

	firstSep, _ := utf8.DecodeRuneInString(sepAttr)
	sep := string(firstSep) + " "
	body := strings.Join(parts, sep)

	return `\left` + open + body + `\right` + closer, true
}

func (c mathmlConverter) convertTable(n *html.Node) (string, bool) {
	children := c.elementChildren(n)
	rows := make([]string, 0, len(children))

	for _, k := range children {
		if strings.ToLower(k.Data) != "mtr" {
			return "", false
		}

		row, ok := c.convertTr(k)
		if !ok {
			return "", false
		}

		rows = append(rows, row)
	}

	return `\begin{matrix} ` + strings.Join(rows, matrixRowSeparator) + ` \end{matrix}`, true
}

func (c mathmlConverter) convertTr(n *html.Node) (string, bool) {
	kids := c.elementChildren(n)
	cells := make([]string, 0, len(kids))

	for _, k := range kids {
		if strings.ToLower(k.Data) != "mtd" {
			return "", false
		}

		cell, ok := c.convertChildren(k)
		if !ok {
			return "", false
		}

		cells = append(cells, cell)
	}

	return strings.Join(cells, matrixColumnSeparator), true
}

var unicodeLatex = map[rune]string{
	'∑': `\sum`,
	'∏': `\prod`,
	'∫': `\int`,
	'∬': `\iint`,
	'∭': `\iiint`,
	'∮': `\oint`,
}

// limitOperators are the LaTeX commands that idiomatically take sub/superscripts
// instead of \underset / \overset wrappers when used in munder / mover / munderover.
var limitOperators = map[string]bool{
	`\sum`:   true,
	`\prod`:  true,
	`\int`:   true,
	`\iint`:  true,
	`\iiint`: true,
	`\oint`:  true,
	`\lim`:   true,
	`\inf`:   true,
	`\sup`:   true,
	`\min`:   true,
	`\max`:   true,
}

var functionLatex = map[string]string{
	"lim": `\lim`,
	"inf": `\inf`,
	"sup": `\sup`,
	"min": `\min`,
	"max": `\max`,
}
