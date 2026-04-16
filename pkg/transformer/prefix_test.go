package transformer

import (
	"testing"

	"github.com/radimsem/remindb/pkg/parser"
)

func TestCompressPrefix_CommonDir(t *testing.T) {
	nodes := []*parser.ContextNode{
		{SourceFile: "/home/user/project/pkg/parser/yaml.go"},
		{SourceFile: "/home/user/project/pkg/parser/json.go"},
		{SourceFile: "/home/user/project/pkg/transformer/anchor.go"},
	}
	compressPrefix(nodes)

	want := []string{
		"parser/yaml.go",
		"parser/json.go",
		"transformer/anchor.go",
	}
	for i, n := range nodes {
		if n.SourceFile != want[i] {
			t.Errorf("nodes[%d].SourceFile = %q, want %q", i, n.SourceFile, want[i])
		}
	}
}

func TestCompressPrefix_SingleNode(t *testing.T) {
	nodes := []*parser.ContextNode{
		{SourceFile: "/home/user/project/main.go"},
	}
	orig := nodes[0].SourceFile
	compressPrefix(nodes)
	if nodes[0].SourceFile != orig {
		t.Errorf("single node changed: %q", nodes[0].SourceFile)
	}
}

func TestCompressPrefix_DifferentRoots(t *testing.T) {
	nodes := []*parser.ContextNode{
		{SourceFile: "/tmp/a.go"},
		{SourceFile: "/var/b.go"},
	}
	orig0, orig1 := nodes[0].SourceFile, nodes[1].SourceFile
	compressPrefix(nodes)
	if nodes[0].SourceFile != orig0 || nodes[1].SourceFile != orig1 {
		t.Errorf("paths changed: %q, %q", nodes[0].SourceFile, nodes[1].SourceFile)
	}
}
