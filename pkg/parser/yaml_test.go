package parser

import "testing"

func TestYamlParser_FlatMapSortedKeys(t *testing.T) {
	nodes, err := parseYaml("t.yaml", []byte("b: 2\na: 1\n"))
	if err != nil {
		t.Fatalf("parseYaml: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("len = %d, want 2", len(nodes))
	}

	if nodes[0].Content != "a: 1" {
		t.Errorf("nodes[0].Content = %q, want %q", nodes[0].Content, "a: 1")
	}
	if nodes[1].Content != "b: 2" {
		t.Errorf("nodes[1].Content = %q, want %q", nodes[1].Content, "b: 2")
	}

	for _, n := range nodes {
		if n.NodeType != NodeKV {
			t.Errorf("NodeType = %v, want NodeKV", n.NodeType)
		}
		if n.Depth != 1 {
			t.Errorf("Depth = %d, want 1", n.Depth)
		}
		if n.SourceFile != "t.yaml" {
			t.Errorf("SourceFile = %q", n.SourceFile)
		}
	}
}

func TestYamlParser_NestedMap(t *testing.T) {
	data := []byte("server:\n  host: localhost\n  port: 8080\n")
	nodes, err := parseYaml("t.yaml", data)
	if err != nil {
		t.Fatalf("parseYaml: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("len = %d, want 1", len(nodes))
	}

	n := nodes[0]
	if n.Content != "server" || n.NodeType != NodeKV {
		t.Errorf("parent: content=%q type=%v", n.Content, n.NodeType)
	}
	if len(n.Children) != 2 {
		t.Fatalf("Children = %d, want 2", len(n.Children))
	}

	if n.Children[0].Content != "host: localhost" {
		t.Errorf("Children[0] = %q", n.Children[0].Content)
	}
	if n.Children[1].Content != "port: 8080" {
		t.Errorf("Children[1] = %q", n.Children[1].Content)
	}
	if n.Children[0].Depth != 2 {
		t.Errorf("child Depth = %d, want 2", n.Children[0].Depth)
	}
}

func TestYamlParser_FlatSequence(t *testing.T) {
	nodes, err := parseYaml("t.yaml", []byte("tags:\n  - go\n  - mcp\n"))
	if err != nil {
		t.Fatalf("parseYaml: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("len = %d, want 1", len(nodes))
	}

	n := nodes[0]
	if n.NodeType != NodeList {
		t.Errorf("NodeType = %v, want NodeList", n.NodeType)
	}

	const want = "tags:\n- go\n- mcp"
	if n.Content != want {
		t.Errorf("Content = %q, want %q", n.Content, want)
	}
}

func TestYamlParser_Empty(t *testing.T) {
	nodes, err := parseYaml("empty.yaml", nil)
	if err != nil {
		t.Fatalf("parseYaml: %v", err)
	}
	if nodes != nil {
		t.Errorf("nodes = %v, want nil", nodes)
	}
}

func TestYamlParser_Determinism(t *testing.T) {
	data := []byte("c: 3\nb: 2\na: 1\n")

	a, _ := parseYaml("t.yaml", data)
	b, _ := parseYaml("t.yaml", data)
	if len(a) != len(b) {
		t.Fatalf("lengths differ: %d vs %d", len(a), len(b))
	}

	for i := range a {
		if a[i].Content != b[i].Content {
			t.Errorf("i=%d: %q != %q", i, a[i].Content, b[i].Content)
		}
	}
}
