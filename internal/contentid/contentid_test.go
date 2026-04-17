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

func TestIdentify_Length(t *testing.T) {
	id := Identify("file.md", "", "content")
	if len(id) != idLen {
		t.Errorf("id len = %d, want %d", len(id), idLen)
	}
}

func TestIdentify_Deterministic(t *testing.T) {
	id1 := Identify("file.md", "parent", "content")
	id2 := Identify("file.md", "parent", "content")
	if id1 != id2 {
		t.Errorf("ids differ: %q vs %q", id1, id2)
	}
}

func TestIdentify_DistinguishesContext(t *testing.T) {
	base := Identify("file.md", "parent", "content")

	bySource := Identify("other.md", "parent", "content")
	if base == bySource {
		t.Error("same ID for different source_file")
	}

	byParent := Identify("file.md", "other", "content")
	if base == byParent {
		t.Error("same ID for different parent")
	}

	byContent := Identify("file.md", "parent", "other")
	if base == byContent {
		t.Error("same ID for different content")
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
