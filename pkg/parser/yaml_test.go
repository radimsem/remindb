package parser

import (
	"strings"
	"testing"
)

func TestYamlParser_FlatMapInlined(t *testing.T) {
	nodes, err := parseYaml("t.yaml", []byte("b: 2\na: 1\n"))
	if err != nil {
		t.Fatalf("parseYaml: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("len = %d, want 1 root", len(nodes))
	}

	n := nodes[0]
	if n.NodeType != NodeKV {
		t.Errorf("NodeType = %v, want NodeKV", n.NodeType)
	}
	if n.Depth != 1 {
		t.Errorf("Depth = %d, want 1", n.Depth)
	}
	if n.Format != FormatPlain {
		t.Errorf("Format = %q, want %q", n.Format, FormatPlain)
	}
	if n.SourceFile != "t.yaml" {
		t.Errorf("SourceFile = %q", n.SourceFile)
	}

	const want = "a: 1\nb: 2"
	if n.Content != want {
		t.Errorf("Content = %q, want %q", n.Content, want)
	}
	if len(n.Children) != 0 {
		t.Errorf("Children = %d, want 0", len(n.Children))
	}
}

func TestYamlParser_NestedMapInlined(t *testing.T) {
	data := []byte("server:\n  host: localhost\n  port: 8080\n")
	nodes, err := parseYaml("t.yaml", data)
	if err != nil {
		t.Fatalf("parseYaml: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("len = %d, want 1", len(nodes))
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

func TestYamlParser_ShortSequenceInlined(t *testing.T) {
	nodes, err := parseYaml("t.yaml", []byte("tags:\n  - go\n  - mcp\n"))
	if err != nil {
		t.Fatalf("parseYaml: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("len = %d, want 1", len(nodes))
	}

	n := nodes[0]
	if n.NodeType != NodeKV {
		t.Errorf("NodeType = %v, want NodeKV (root map with inlined list)", n.NodeType)
	}

	const want = "tags:\n- go\n- mcp"
	if n.Content != want {
		t.Errorf("Content = %q, want %q", n.Content, want)
	}
	if len(n.Children) != 0 {
		t.Errorf("Children = %d, want 0", len(n.Children))
	}
}

func TestYamlParser_LargeMapPromoted(t *testing.T) {
	data := []byte(strings.Join([]string{
		"config:",
		"  a: 1",
		"  b: 2",
		"  c: 3",
		"  d: 4",
		"  e: 5",
		"scalar: 42",
	}, "\n") + "\n")

	nodes, err := parseYaml("t.yaml", data)
	if err != nil {
		t.Fatalf("parseYaml: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("len = %d, want 1 root", len(nodes))
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
	if config.Depth != 2 {
		t.Errorf("config.Depth = %d, want 2", config.Depth)
	}

	const wantConfig = "config:\n  a: 1\n  b: 2\n  c: 3\n  d: 4\n  e: 5"
	if config.Content != wantConfig {
		t.Errorf("config.Content = %q, want %q", config.Content, wantConfig)
	}
}

func TestYamlParser_LongScalarSequenceToon(t *testing.T) {
	data := []byte("tags:\n  - a\n  - b\n  - c\n  - d\n  - e\n")
	nodes, err := parseYaml("t.yaml", data)
	if err != nil {
		t.Fatalf("parseYaml: %v", err)
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
	if tags.Format != FormatToon {
		t.Errorf("tags.Format = %q, want %q", tags.Format, FormatToon)
	}

	const want = "tags[5]: a,b,c,d,e"
	if tags.Content != want {
		t.Errorf("tags.Content = %q, want %q", tags.Content, want)
	}
}

func TestYamlParser_UniformObjectArrayToon(t *testing.T) {
	data := []byte(`users:
  - {id: 1, name: alice}
  - {id: 2, name: bob}
  - {id: 3, name: carol}
  - {id: 4, name: dave}
  - {id: 5, name: eve}
`)
	nodes, err := parseYaml("t.yaml", data)
	if err != nil {
		t.Fatalf("parseYaml: %v", err)
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
	if a[0].Content != b[0].Content {
		t.Errorf("non-deterministic: %q != %q", a[0].Content, b[0].Content)
	}
}
