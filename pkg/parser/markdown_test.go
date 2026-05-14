package parser

import (
	"strings"
	"testing"
)

func TestMarkdownParser_NestedHeadings(t *testing.T) {
	data := []byte("# H1\n\n## H2\n\n### H3\n\nend\n")
	nodes, err := parseMarkdown("t.md", data)
	if err != nil {
		t.Fatalf("parseMarkdown: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("len = %d, want 1", len(nodes))
	}

	h1 := nodes[0]
	if h1.NodeType != NodeHeading || h1.Content != "H1" {
		t.Errorf("h1 = %+v", h1)
	}
	if len(h1.Children) != 1 {
		t.Fatalf("h1.Children = %d, want 1", len(h1.Children))
	}

	h2 := h1.Children[0]
	if h2.Content != "H2" || len(h2.Children) != 1 {
		t.Fatalf("h2 = %+v", h2)
	}

	h3 := h2.Children[0]
	if h3.Content != "H3" {
		t.Errorf("h3 = %+v", h3)
	}
	if len(h3.Children) != 1 {
		t.Fatalf("h3.Children = %d", len(h3.Children))
	}

	para := h3.Children[0]
	if para.NodeType != NodeText || para.Content != "end" {
		t.Errorf("end para = %+v", para)
	}
}

func TestMarkdownParser_SiblingHeadings(t *testing.T) {
	data := []byte("# H1\n\n## A\n\npara1\n\n## B\n\npara2\n")
	nodes, err := parseMarkdown("t.md", data)
	if err != nil {
		t.Fatalf("parseMarkdown: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("len = %d", len(nodes))
	}

	h1 := nodes[0]
	if len(h1.Children) != 2 {
		t.Fatalf("H1.Children = %d, want 2", len(h1.Children))
	}
	if h1.Children[0].Content != "A" || h1.Children[1].Content != "B" {
		t.Errorf("siblings = %+v", h1.Children)
	}
}

func TestMarkdownParser_CodeBlock(t *testing.T) {
	nodes, err := parseMarkdown("t.md", []byte("```go\nx := 1\n```\n"))
	if err != nil {
		t.Fatalf("parseMarkdown: %v", err)
	}
	if len(nodes) != 1 || nodes[0].NodeType != NodeCode {
		t.Fatalf("nodes = %+v", nodes)
	}

	const want = "go\nx := 1"
	if nodes[0].Content != want {
		t.Errorf("Content = %q, want %q", nodes[0].Content, want)
	}
}

func TestMarkdownParser_List(t *testing.T) {
	nodes, err := parseMarkdown("t.md", []byte("- a\n- b\n- c\n"))
	if err != nil {
		t.Fatalf("parseMarkdown: %v", err)
	}
	if len(nodes) != 1 || nodes[0].NodeType != NodeList {
		t.Fatalf("nodes = %+v", nodes)
	}

	const want = "- a\n- b\n- c"
	if nodes[0].Content != want {
		t.Errorf("Content = %q, want %q", nodes[0].Content, want)
	}
}

func TestMarkdownParser_WithPreamble(t *testing.T) {
	data := []byte("---\ntitle: foo\n---\n# Body\n")
	nodes, err := parseMarkdown("t.md", data)
	if err != nil {
		t.Fatalf("parseMarkdown: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("len = %d, want 2 (preamble + body)", len(nodes))
	}

	if nodes[0].NodeType != NodePreamble {
		t.Errorf("nodes[0].NodeType = %v, want NodePreamble", nodes[0].NodeType)
	}
	if nodes[1].NodeType != NodeHeading || nodes[1].Content != "Body" {
		t.Errorf("nodes[1] = %+v", nodes[1])
	}
}

func TestMarkdownParser_PreservesLinks(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{"[anthropic](https://docs.anthropic.com)\n", "[anthropic](https://docs.anthropic.com)"},
		{"[anthropic](https://docs.anthropic.com \"docs\")\n", "[anthropic](https://docs.anthropic.com \"docs\")"},
		{"see [docs](https://x.com) here\n", "see [docs](https://x.com) here"},
	}
	for _, c := range cases {
		nodes, err := parseMarkdown("t.md", []byte(c.src))
		if err != nil {
			t.Fatalf("parseMarkdown(%q): %v", c.src, err)
		}

		if len(nodes) != 1 || nodes[0].Content != c.want {
			t.Errorf("src=%q: Content = %q, want %q", c.src, nodes[0].Content, c.want)
		}
	}
}

func TestMarkdownParser_PreservesImages(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{"![logo](https://x.com/logo.png)\n", "![logo](https://x.com/logo.png)"},
		{"![](https://x.com/logo.png)\n", "![](https://x.com/logo.png)"},
		{"![logo](https://x.com/logo.png \"alt\")\n", "![logo](https://x.com/logo.png \"alt\")"},
	}
	for _, c := range cases {
		nodes, err := parseMarkdown("t.md", []byte(c.src))
		if err != nil {
			t.Fatalf("parseMarkdown(%q): %v", c.src, err)
		}

		if len(nodes) != 1 || nodes[0].Content != c.want {
			t.Errorf("src=%q: Content = %q, want %q", c.src, nodes[0].Content, c.want)
		}
	}
}

func TestMarkdownParser_LinkInHeadingAndList(t *testing.T) {
	data := []byte("# see [docs](https://x.com)\n\n- read [more](https://y.com)\n")
	nodes, err := parseMarkdown("t.md", data)
	if err != nil {
		t.Fatalf("parseMarkdown: %v", err)
	}

	if len(nodes) != 1 || nodes[0].Content != "see [docs](https://x.com)" {
		t.Fatalf("heading = %+v", nodes[0])
	}
	if len(nodes[0].Children) != 1 {
		t.Fatalf("heading.Children = %d, want 1", len(nodes[0].Children))
	}

	list := nodes[0].Children[0]
	if list.NodeType != NodeList || list.Content != "- read [more](https://y.com)" {
		t.Errorf("list = %+v", list)
	}
}

func TestMarkdownParser_Empty(t *testing.T) {
	nodes, err := parseMarkdown("empty.md", []byte(""))
	if err != nil {
		t.Fatalf("parseMarkdown: %v", err)
	}
	if len(nodes) != 0 {
		t.Errorf("nodes = %+v, want empty", nodes)
	}
}

func TestMarkdownParser_WikilinkInHeading(t *testing.T) {
	nodes, err := parseMarkdown("t.md", []byte("# Foo [[Architecture; w=2.5]]\n"))
	if err != nil {
		t.Fatalf("parseMarkdown: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("len = %d, want 1", len(nodes))
	}

	h := nodes[0]
	if h.NodeType != NodeHeading {
		t.Errorf("NodeType = %s", h.NodeType)
	}
	if h.Content != "Foo [[Architecture]]" {
		t.Errorf("Content = %q, want %q (params stripped)", h.Content, "Foo [[Architecture]]")
	}

	if len(h.WikilinkRefs) != 1 {
		t.Fatalf("len(WikilinkRefs) = %d, want 1", len(h.WikilinkRefs))
	}
	if h.WikilinkRefs[0].Label != "Architecture" || h.WikilinkRefs[0].Weight != 2.5 {
		t.Errorf("ref = %+v", h.WikilinkRefs[0])
	}
}

func TestMarkdownParser_WikilinkInParagraph(t *testing.T) {
	nodes, err := parseMarkdown("t.md", []byte("See [[A]] and [[B; w=2]] for details.\n"))
	if err != nil {
		t.Fatalf("parseMarkdown: %v", err)
	}

	para := nodes[0]
	if para.NodeType != NodeText {
		t.Fatalf("NodeType = %s", para.NodeType)
	}
	if para.Content != "See [[A]] and [[B]] for details." {
		t.Errorf("Content = %q", para.Content)
	}

	if len(para.WikilinkRefs) != 2 {
		t.Fatalf("len(WikilinkRefs) = %d, want 2", len(para.WikilinkRefs))
	}
	if para.WikilinkRefs[0].Label != "A" || para.WikilinkRefs[1].Label != "B" {
		t.Errorf("refs out of order: %+v", para.WikilinkRefs)
	}
	if para.WikilinkRefs[1].Weight != 2.0 {
		t.Errorf("ref[1].Weight = %f, want 2.0", para.WikilinkRefs[1].Weight)
	}
}

func TestMarkdownParser_WikilinkInListItem(t *testing.T) {
	src := "- see [[A]]\n- and [[B; source=other.md]]\n"
	nodes, err := parseMarkdown("t.md", []byte(src))
	if err != nil {
		t.Fatalf("parseMarkdown: %v", err)
	}

	list := nodes[0]
	if list.NodeType != NodeList {
		t.Fatalf("NodeType = %s", list.NodeType)
	}
	if list.Content != "- see [[A]]\n- and [[B]]" {
		t.Errorf("Content = %q", list.Content)
	}

	if len(list.WikilinkRefs) != 2 {
		t.Fatalf("len(WikilinkRefs) = %d, want 2", len(list.WikilinkRefs))
	}
	if list.WikilinkRefs[1].SourceQual != "other.md" {
		t.Errorf("ref[1].SourceQual = %q", list.WikilinkRefs[1].SourceQual)
	}
}

func TestMarkdownParser_WikilinkInCodeBlockNotExtracted(t *testing.T) {
	src := "```\nSee [[X]] inside fenced code\n```\n"
	nodes, err := parseMarkdown("t.md", []byte(src))
	if err != nil {
		t.Fatalf("parseMarkdown: %v", err)
	}

	code := nodes[0]
	if code.NodeType != NodeCode {
		t.Fatalf("NodeType = %s", code.NodeType)
	}
	if !strings.Contains(code.Content, "[[X]]") {
		t.Errorf("Content lost [[X]]: %q", code.Content)
	}
	if len(code.WikilinkRefs) != 0 {
		t.Errorf("WikilinkRefs = %+v, want none (code block content is example text)", code.WikilinkRefs)
	}
}

func TestMarkdownParser_WikilinkInCodeSpanNotExtracted(t *testing.T) {
	src := "use `[[X]]` carefully\n"
	nodes, err := parseMarkdown("t.md", []byte(src))
	if err != nil {
		t.Fatalf("parseMarkdown: %v", err)
	}

	para := nodes[0]
	if para.NodeType != NodeText {
		t.Fatalf("NodeType = %s", para.NodeType)
	}
	if !strings.Contains(para.Content, "[[X]]") {
		t.Errorf("Content lost [[X]]: %q", para.Content)
	}
	if len(para.WikilinkRefs) != 0 {
		t.Errorf("WikilinkRefs = %+v, want none (code span is example text)", para.WikilinkRefs)
	}
}

func TestMarkdownParser_WikilinkInTable(t *testing.T) {
	src := "| Item | Ref |\n|---|---|\n| Foo | [[A]] |\n| Bar | [[B]] |\n"
	nodes, err := parseMarkdown("t.md", []byte(src))
	if err != nil {
		t.Fatalf("parseMarkdown: %v", err)
	}

	tbl := nodes[0]
	if tbl.NodeType != NodeTable {
		t.Fatalf("NodeType = %s", tbl.NodeType)
	}
	if len(tbl.WikilinkRefs) != 2 {
		t.Fatalf("len(WikilinkRefs) = %d, want 2", len(tbl.WikilinkRefs))
	}
}
