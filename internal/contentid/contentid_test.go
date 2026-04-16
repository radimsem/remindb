package contentid

import "testing"

func TestHash_Lengths(t *testing.T) {
	hash, id := Hash("test content")
	if len(hash) != 16 {
		t.Errorf("hash len = %d, want 16", len(hash))
	}
	if len(id) != 8 {
		t.Errorf("id len = %d, want 8", len(id))
	}
}

func TestHash_Deterministic(t *testing.T) {
	h1, id1 := Hash("hello")
	h2, id2 := Hash("hello")
	if h1 != h2 {
		t.Errorf("hashes differ")
	}
	if id1 != id2 {
		t.Errorf("ids differ")
	}
}

func TestHash_DifferentContent(t *testing.T) {
	h1, _ := Hash("foo")
	h2, _ := Hash("bar")
	if h1 == h2 {
		t.Error("different content same hash")
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
