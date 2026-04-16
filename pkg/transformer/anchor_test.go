package transformer

import (
	"testing"

	"github.com/radimsem/remindb/pkg/parser"
)

func TestSetAnchor_Deterministic(t *testing.T) {
	a := &parser.ContextNode{Content: "hello world"}
	b := &parser.ContextNode{Content: "hello world"}
	setAnchor(a)
	setAnchor(b)

	if a.ContentHash != b.ContentHash {
		t.Errorf("hashes differ: %q vs %q", a.ContentHash, b.ContentHash)
	}
	if a.ID != b.ID {
		t.Errorf("IDs differ: %q vs %q", a.ID, b.ID)
	}
}

func TestSetAnchor_Lengths(t *testing.T) {
	n := &parser.ContextNode{Content: "test content"}
	setAnchor(n)

	if len(n.ContentHash) != 16 {
		t.Errorf("ContentHash len = %d, want 16", len(n.ContentHash))
	}
	if len(n.ID) != 8 {
		t.Errorf("ID len = %d, want 8", len(n.ID))
	}
}

func TestSetAnchor_DifferentContent(t *testing.T) {
	a := &parser.ContextNode{Content: "foo"}
	b := &parser.ContextNode{Content: "bar"}
	setAnchor(a)
	setAnchor(b)

	if a.ContentHash == b.ContentHash {
		t.Errorf("different content same hash: %q", a.ContentHash)
	}
	if a.ID == b.ID {
		t.Errorf("different content same ID: %q", a.ID)
	}
}
