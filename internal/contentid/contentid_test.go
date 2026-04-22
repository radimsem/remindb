package contentid

import "testing"

func TestContentHash_Length(t *testing.T) {
	hash := ContentHash("test content")
	if len(hash) != 16 {
		t.Errorf("hash len = %d, want 16", len(hash))
	}
}

func TestContentHash_Deterministic(t *testing.T) {
	h1 := ContentHash("hello")
	h2 := ContentHash("hello")
	if h1 != h2 {
		t.Errorf("hashes differ")
	}
}

func TestContentHash_DifferentContent(t *testing.T) {
	h1 := ContentHash("foo")
	h2 := ContentHash("bar")
	if h1 == h2 {
		t.Error("different content same hash")
	}
}

func TestIdentifyNode_Length(t *testing.T) {
	id := IdentifyNode("file.md", "", 0)
	if len(id) != idLen {
		t.Errorf("id len = %d, want %d", len(id), idLen)
	}
}

func TestIdentifyNode_Deterministic(t *testing.T) {
	id1 := IdentifyNode("file.md", "parent", 2)
	id2 := IdentifyNode("file.md", "parent", 2)
	if id1 != id2 {
		t.Errorf("ids differ: %q vs %q", id1, id2)
	}
}

func TestIdentifyNode_DistinguishesPosition(t *testing.T) {
	base := IdentifyNode("file.md", "parent", 0)

	bySource := IdentifyNode("other.md", "parent", 0)
	if base == bySource {
		t.Error("same ID for different source_file")
	}

	byParent := IdentifyNode("file.md", "other", 0)
	if base == byParent {
		t.Error("same ID for different parent")
	}

	bySibling := IdentifyNode("file.md", "parent", 1)
	if base == bySibling {
		t.Error("same ID for different siblingIndex")
	}
}

func TestIdentifyPayload_SamePayloadDedupes(t *testing.T) {
	id1 := IdentifyPayload("mcp:write", "hello")
	id2 := IdentifyPayload("mcp:write", "hello")
	if id1 != id2 {
		t.Errorf("identical payloads produced different IDs: %q vs %q", id1, id2)
	}

	id3 := IdentifyPayload("mcp:write", "goodbye")
	if id1 == id3 {
		t.Error("different payloads produced the same ID")
	}
}

func TestEncodeBase62_ZeroPadded(t *testing.T) {
	result := encodeBase62(0)
	if len(result) != 11 {
		t.Errorf("len = %d, want 11", len(result))
	}
	if result != "00000000000" {
		t.Errorf("encodeBase62(0) = %q, want %q", result, "00000000000")
	}
}
