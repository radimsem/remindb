package transformer

import (
	"context"
	"testing"

	"github.com/radimsem/remindb/pkg/parser"
)

func TestTransform_Integration(t *testing.T) {
	roots := []*parser.ContextNode{
		{
			SourceFile: "/project/doc.md",
			NodeType:   parser.NodeHeading,
			Content:    "Getting Started\r\n",
			Format:     parser.FormatPlain,
			Children: []*parser.ContextNode{
				{
					SourceFile: "/project/doc.md",
					NodeType:   parser.NodeText,
					Content:    "Follow these steps. They are easy.",
					Format:     parser.FormatPlain,
				},
				{
					SourceFile: "/project/doc.md",
					NodeType:   parser.NodeCode,
					Content:    "go\nfmt.Println(\"hello\")",
					Format:     parser.FormatPlain,
				},
			},
		},
	}

	err := Transform(context.Background(), roots)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}

	root := roots[0]

	// Compressed: CRLF + trailing newline stripped
	if root.Content != "Getting Started" {
		t.Errorf("root.Content = %q", root.Content)
	}

	// Anchor: 8-char ID, 16-char hex hash
	if len(root.ID) != 8 {
		t.Errorf("root.ID = %q, want 8-char", root.ID)
	}
	if len(root.ContentHash) != 16 {
		t.Errorf("root.ContentHash = %q, want 16-char hex", root.ContentHash)
	}

	// Label
	if root.Label != "Getting Started" {
		t.Errorf("root.Label = %q", root.Label)
	}

	// Token count
	if root.TokenCount <= 0 {
		t.Errorf("root.TokenCount = %d", root.TokenCount)
	}

	// Parent IDs wired
	text := root.Children[0]
	code := root.Children[1]
	if root.ParentID != "" {
		t.Errorf("root.ParentID = %q, want empty", root.ParentID)
	}
	if text.ParentID != root.ID {
		t.Errorf("text.ParentID = %q, want %q", text.ParentID, root.ID)
	}
	if code.ParentID != root.ID {
		t.Errorf("code.ParentID = %q, want %q", code.ParentID, root.ID)
	}

	// Code label
	want := `Code (go): fmt.Println("hello")`
	if code.Label != want {
		t.Errorf("code.Label = %q, want %q", code.Label, want)
	}

	// Text label: first sentence
	if text.Label != "Follow these steps." {
		t.Errorf("text.Label = %q", text.Label)
	}
}

func TestTransform_Empty(t *testing.T) {
	if err := Transform(context.Background(), nil); err != nil {
		t.Fatalf("Transform(nil): %v", err)
	}
}

func TestFlatten(t *testing.T) {
	roots := []*parser.ContextNode{
		{
			Content: "a",
			Children: []*parser.ContextNode{
				{Content: "b"},
				{Content: "c", Children: []*parser.ContextNode{
					{Content: "d"},
				}},
			},
		},
		{Content: "e"},
	}

	flat := parser.Flatten(roots)
	if len(flat) != 5 {
		t.Fatalf("len = %d, want 5", len(flat))
	}

	want := []string{"a", "b", "c", "d", "e"}
	for i, n := range flat {
		if n.Content != want[i] {
			t.Errorf("flat[%d].Content = %q, want %q", i, n.Content, want[i])
		}
	}
}

func TestWireParents(t *testing.T) {
	child := &parser.ContextNode{ID: "child001"}
	root := &parser.ContextNode{
		ID:       "root0001",
		Children: []*parser.ContextNode{child},
	}

	wireParents([]*parser.ContextNode{root}, "")

	if root.ParentID != "" {
		t.Errorf("root.ParentID = %q", root.ParentID)
	}
	if child.ParentID != "root0001" {
		t.Errorf("child.ParentID = %q, want %q", child.ParentID, "root0001")
	}
}
