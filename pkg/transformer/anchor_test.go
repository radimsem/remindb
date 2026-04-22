package transformer

import (
	"testing"

	"github.com/radimsem/remindb/pkg/parser"
)

func TestSetContentHash_Length(t *testing.T) {
	n := &parser.ContextNode{Content: "test content"}
	setContentHash(n)
	if len(n.ContentHash) != 16 {
		t.Errorf("ContentHash len = %d, want 16", len(n.ContentHash))
	}
}

func TestSetContentHash_Deterministic(t *testing.T) {
	a := &parser.ContextNode{Content: "hello world"}
	b := &parser.ContextNode{Content: "hello world"}
	setContentHash(a)
	setContentHash(b)

	if a.ContentHash != b.ContentHash {
		t.Errorf("hashes differ: %q vs %q", a.ContentHash, b.ContentHash)
	}
}

func TestSetContentHash_DifferentContent(t *testing.T) {
	a := &parser.ContextNode{Content: "foo"}
	b := &parser.ContextNode{Content: "bar"}
	setContentHash(a)
	setContentHash(b)

	if a.ContentHash == b.ContentHash {
		t.Errorf("different content same hash: %q", a.ContentHash)
	}
}

func TestSetIdentity_Length(t *testing.T) {
	n := &parser.ContextNode{SourceFile: "f.md", Content: "x"}
	setIdentity(n, "", 0)
	if len(n.ID) != 11 {
		t.Errorf("ID len = %d, want 11", len(n.ID))
	}
}

func TestSetIdentity_DistinguishesParent(t *testing.T) {
	a := &parser.ContextNode{SourceFile: "f.md", Content: "same"}
	b := &parser.ContextNode{SourceFile: "f.md", Content: "same"}
	setIdentity(a, "parentA", 0)
	setIdentity(b, "parentB", 0)

	if a.ID == b.ID {
		t.Errorf("same ID %q for different parents", a.ID)
	}
	if a.ParentID != "parentA" || b.ParentID != "parentB" {
		t.Errorf("ParentID not set: %q, %q", a.ParentID, b.ParentID)
	}
}

func TestSetIdentity_DistinguishesSource(t *testing.T) {
	a := &parser.ContextNode{SourceFile: "a.md", Content: "same"}
	b := &parser.ContextNode{SourceFile: "b.md", Content: "same"}
	setIdentity(a, "", 0)
	setIdentity(b, "", 0)

	if a.ID == b.ID {
		t.Errorf("same ID %q for different source files", a.ID)
	}
}

func TestSetIdentity_StableAcrossContentEdits(t *testing.T) {
	a := &parser.ContextNode{SourceFile: "f.md", Content: "original text"}
	b := &parser.ContextNode{SourceFile: "f.md", Content: "edited text"}
	setIdentity(a, "parent", 3)
	setIdentity(b, "parent", 3)

	if a.ID != b.ID {
		t.Errorf("content edit changed ID: %q vs %q", a.ID, b.ID)
	}
}
