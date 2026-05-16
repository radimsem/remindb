package transformer

import (
	"context"
	"strings"
	"testing"

	"github.com/radimsem/remindb/internal/redaction"
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

	err := Transform(context.Background(), roots, "", nil)
	if err != nil {
		t.Fatalf("Transform: %v", err)
	}

	root := roots[0]

	// Compressed: CRLF + trailing newline stripped
	if root.Content != "Getting Started" {
		t.Errorf("root.Content = %q", root.Content)
	}

	// Anchor: 11-char ID, 16-char hex hash
	if len(root.ID) != 11 {
		t.Errorf("root.ID = %q, want 11-char", root.ID)
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
	if err := Transform(context.Background(), nil, "", nil); err != nil {
		t.Fatalf("Transform(nil): %v", err)
	}
}

func TestTransform_ScrubsBeforeHash(t *testing.T) {
	red, err := redaction.New(redaction.DefaultConfig())
	if err != nil {
		t.Fatalf("redaction.New: %v", err)
	}

	roots := []*parser.ContextNode{
		{
			SourceFile: "/project/secrets.md",
			NodeType:   parser.NodeText,
			Content:    "leaked AKIAIOSFODNN7EXAMPLE in notes",
			Format:     parser.FormatPlain,
		},
	}

	if err := Transform(context.Background(), roots, "", red); err != nil {
		t.Fatalf("Transform: %v", err)
	}

	n := roots[0]
	if strings.Contains(n.Content, "AKIA") {
		t.Errorf("AKIA leaked into node content: %q", n.Content)
	}
	if !strings.Contains(n.Content, "«redacted:aws_access_key»") {
		t.Errorf("marker missing: %q", n.Content)
	}
	if n.ContentHash == "" {
		t.Error("ContentHash empty — hash pass must run after scrub")
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

func TestWireIdentity(t *testing.T) {
	child := &parser.ContextNode{SourceFile: "f.md", Content: "child"}
	root := &parser.ContextNode{
		SourceFile: "f.md",
		Content:    "root",
		Children:   []*parser.ContextNode{child},
	}

	wireIdentity([]*parser.ContextNode{root}, "")

	if root.ID == "" {
		t.Fatal("root.ID empty")
	}
	if root.ParentID != "" {
		t.Errorf("root.ParentID = %q", root.ParentID)
	}
	if child.ParentID != root.ID {
		t.Errorf("child.ParentID = %q, want %q", child.ParentID, root.ID)
	}
	if child.ID == root.ID {
		t.Errorf("child.ID == root.ID = %q", root.ID)
	}
}
