package parser

import "testing"

func TestJsonParser_FlatObjectInlined(t *testing.T) {
	nodes, err := parseJson("t.json", []byte(`{"b": 2, "a": 1}`))
	if err != nil {
		t.Fatalf("parseJson: %v", err)
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
	if len(n.Children) != 0 {
		t.Errorf("Children = %d, want 0", len(n.Children))
	}
}

func TestJsonParser_NestedObjectInlined(t *testing.T) {
	data := []byte(`{"server": {"host": "localhost", "port": 8080}}`)
	nodes, err := parseJson("t.json", data)
	if err != nil {
		t.Fatalf("parseJson: %v", err)
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

func TestJsonParser_ShortArrayInlined(t *testing.T) {
	nodes, err := parseJson("t.json", []byte(`{"tags": ["a", "b", "c"]}`))
	if err != nil {
		t.Fatalf("parseJson: %v", err)
	}

	n := nodes[0]
	const want = "tags:\n- a\n- b\n- c"
	if n.Content != want {
		t.Errorf("Content = %q, want %q", n.Content, want)
	}
}

func TestJsonParser_LongArrayPromoted(t *testing.T) {
	data := []byte(`{"tags": ["a", "b", "c", "d", "e"]}`)
	nodes, err := parseJson("t.json", data)
	if err != nil {
		t.Fatalf("parseJson: %v", err)
	}

	root := nodes[0]
	if len(root.Children) != 1 {
		t.Fatalf("Children = %d, want 1 (tags promoted at 5 elements)", len(root.Children))
	}

	tags := root.Children[0]
	if tags.NodeType != NodeList {
		t.Errorf("tags.NodeType = %v, want NodeList", tags.NodeType)
	}
	if tags.Depth != 2 {
		t.Errorf("tags.Depth = %d, want 2", tags.Depth)
	}
}

func TestJsonParser_LargeObjectPromoted(t *testing.T) {
	data := []byte(`{"config": {"a":1,"b":2,"c":3,"d":4,"e":5}, "scalar": 42}`)
	nodes, err := parseJson("t.json", data)
	if err != nil {
		t.Fatalf("parseJson: %v", err)
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

func TestJsonParser_NumberPrecision(t *testing.T) {
	// 2^53 + 1 cannot be represented exactly as float64; json.Number keeps it.
	data := []byte(`{"id": 9007199254740993}`)
	nodes, err := parseJson("t.json", data)
	if err != nil {
		t.Fatalf("parseJson: %v", err)
	}

	const want = "id: 9007199254740993"
	if nodes[0].Content != want {
		t.Errorf("Content = %q, want %q", nodes[0].Content, want)
	}
}
