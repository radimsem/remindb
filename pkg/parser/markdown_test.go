package parser

import "testing"

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

func TestMarkdownParser_Empty(t *testing.T) {
	nodes, err := parseMarkdown("empty.md", []byte(""))
	if err != nil {
		t.Fatalf("parseMarkdown: %v", err)
	}
	if len(nodes) != 0 {
		t.Errorf("nodes = %+v, want empty", nodes)
	}
}
