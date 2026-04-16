package parser

import "testing"

func TestToonParser_FlatObjectInlined(t *testing.T) {
	nodes, err := parseToon("t.toon", []byte("b: 2\na: 1\n"))
	if err != nil {
		t.Fatalf("parseToon: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("len = %d, want 1 root", len(nodes))
	}

	n := nodes[0]
	const want = "a: 1\nb: 2"
	if n.Content != want {
		t.Errorf("Content = %q, want %q", n.Content, want)
	}
	if n.Format != FormatPlain {
		t.Errorf("Format = %q, want %q", n.Format, FormatPlain)
	}
}

func TestToonParser_NestedObjectInlined(t *testing.T) {
	data := []byte("server:\n  host: localhost\n  port: 8080\n")
	nodes, err := parseToon("t.toon", data)
	if err != nil {
		t.Fatalf("parseToon: %v", err)
	}

	n := nodes[0]
	const want = "server:\n  host: localhost\n  port: 8080"
	if n.Content != want {
		t.Errorf("Content = %q, want %q", n.Content, want)
	}
	if len(n.Children) != 0 {
		t.Errorf("Children = %d, want 0 (server has <5 fields)", len(n.Children))
	}
}

func TestToonParser_ShortScalarArrayInlined(t *testing.T) {
	nodes, err := parseToon("t.toon", []byte("tags[3]: a,b,c\n"))
	if err != nil {
		t.Fatalf("parseToon: %v", err)
	}

	n := nodes[0]
	const want = "tags:\n- a\n- b\n- c"
	if n.Content != want {
		t.Errorf("Content = %q, want %q", n.Content, want)
	}
}

func TestToonParser_LongScalarArrayToon(t *testing.T) {
	nodes, err := parseToon("t.toon", []byte("tags[5]: a,b,c,d,e\n"))
	if err != nil {
		t.Fatalf("parseToon: %v", err)
	}

	tags := nodes[0].Children[0]
	if tags.NodeType != NodeList {
		t.Errorf("tags.NodeType = %v, want NodeList", tags.NodeType)
	}
	if tags.Format != FormatToon {
		t.Errorf("tags.Format = %q, want %q", tags.Format, FormatToon)
	}
	if tags.Content != "tags[5]: a,b,c,d,e" {
		t.Errorf("tags.Content = %q", tags.Content)
	}
}

func TestToonParser_UniformObjectArrayToon(t *testing.T) {
	data := []byte("users[5]{id,name}:\n  1,alice\n  2,bob\n  3,carol\n  4,dave\n  5,eve\n")
	nodes, err := parseToon("t.toon", data)
	if err != nil {
		t.Fatalf("parseToon: %v", err)
	}

	users := nodes[0].Children[0]
	if users.Format != FormatToon {
		t.Fatalf("users.Format = %q, want %q", users.Format, FormatToon)
	}

	const want = "users[5]{id,name}:\n  1,alice\n  2,bob\n  3,carol\n  4,dave\n  5,eve"
	if users.Content != want {
		t.Errorf("users.Content = %q, want %q", users.Content, want)
	}
}

func TestToonParser_LargeObjectPromoted(t *testing.T) {
	data := []byte("config:\n  a: 1\n  b: 2\n  c: 3\n  d: 4\n  e: 5\nscalar: 42\n")
	nodes, err := parseToon("t.toon", data)
	if err != nil {
		t.Fatalf("parseToon: %v", err)
	}

	root := nodes[0]
	if root.Content != "scalar: 42" {
		t.Errorf("root.Content = %q, want %q", root.Content, "scalar: 42")
	}
	if len(root.Children) != 1 {
		t.Fatalf("root.Children = %d, want 1 (config)", len(root.Children))
	}

	config := root.Children[0]
	if config.NodeType != NodeKV {
		t.Errorf("config.NodeType = %v, want NodeKV", config.NodeType)
	}

	const wantConfig = "config:\n  a: 1\n  b: 2\n  c: 3\n  d: 4\n  e: 5"
	if config.Content != wantConfig {
		t.Errorf("config.Content = %q, want %q", config.Content, wantConfig)
	}
}
