package parser

import "testing"

func TestJsonParser_FlatObjectSortedKeys(t *testing.T) {
	nodes, err := parseJson("t.json", []byte(`{"b": 2, "a": 1}`))
	if err != nil {
		t.Fatalf("parseJson: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("len = %d, want 2", len(nodes))
	}

	if nodes[0].Content != "a: 1" {
		t.Errorf("nodes[0] = %q", nodes[0].Content)
	}
	if nodes[1].Content != "b: 2" {
		t.Errorf("nodes[1] = %q", nodes[1].Content)
	}
}

func TestJsonParser_NestedObject(t *testing.T) {
	data := []byte(`{"server": {"host": "localhost", "port": 8080}}`)
	nodes, err := parseJson("t.json", data)
	if err != nil {
		t.Fatalf("parseJson: %v", err)
	}
	if len(nodes) != 1 || nodes[0].Content != "server" {
		t.Fatalf("nodes = %+v", nodes)
	}

	ch := nodes[0].Children
	if len(ch) != 2 {
		t.Fatalf("Children = %d, want 2", len(ch))
	}
	if ch[0].Content != "host: localhost" || ch[1].Content != "port: 8080" {
		t.Errorf("Children = %+v", ch)
	}
}

func TestJsonParser_FlatArray(t *testing.T) {
	nodes, err := parseJson("t.json", []byte(`{"tags": ["a", "b", "c"]}`))
	if err != nil {
		t.Fatalf("parseJson: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("len = %d", len(nodes))
	}

	n := nodes[0]
	if n.NodeType != NodeList {
		t.Errorf("NodeType = %v, want NodeList", n.NodeType)
	}

	const want = "tags:\n- a\n- b\n- c"
	if n.Content != want {
		t.Errorf("Content = %q, want %q", n.Content, want)
	}
}

func TestJsonParser_NumberPrecision(t *testing.T) {
	// 2^53 + 1 cannot be represented exactly as float64; json.Number keeps it.
	data := []byte(`{"id": 9007199254740993}`)
	nodes, err := parseJson("t.json", data)
	if err != nil {
		t.Fatalf("parseJson: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("len = %d", len(nodes))
	}

	const want = "id: 9007199254740993"
	if nodes[0].Content != want {
		t.Errorf("Content = %q, want %q", nodes[0].Content, want)
	}
}
