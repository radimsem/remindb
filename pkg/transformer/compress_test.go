package transformer

import (
	"testing"

	"github.com/radimsem/remindb/pkg/parser"
)

func TestCompress_CRLF(t *testing.T) {
	n := &parser.ContextNode{Content: "a\r\nb\r\nc"}
	compress(n)
	if n.Content != "a\nb\nc" {
		t.Errorf("Content = %q, want %q", n.Content, "a\nb\nc")
	}
}

func TestCompress_CollapseBlankLines(t *testing.T) {
	n := &parser.ContextNode{Content: "a\n\n\n\nb"}
	compress(n)
	if n.Content != "a\n\nb" {
		t.Errorf("Content = %q, want %q", n.Content, "a\n\nb")
	}
}

func TestCompress_TrimTrailingSpaces(t *testing.T) {
	n := &parser.ContextNode{Content: "a   \nb  \nc"}
	compress(n)
	if n.Content != "a\nb\nc" {
		t.Errorf("Content = %q, want %q", n.Content, "a\nb\nc")
	}
}

func TestCompress_TrimEmptyEdges(t *testing.T) {
	n := &parser.ContextNode{Content: "\n\nhello\nworld\n\n"}
	compress(n)
	if n.Content != "hello\nworld" {
		t.Errorf("Content = %q, want %q", n.Content, "hello\nworld")
	}
}

func TestCompress_PreservesIndentation(t *testing.T) {
	n := &parser.ContextNode{Content: "key:\n  sub: val"}
	compress(n)
	if n.Content != "key:\n  sub: val" {
		t.Errorf("Content = %q, want %q", n.Content, "key:\n  sub: val")
	}
}
