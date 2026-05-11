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
	compressPrefix(nodes, "")

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

func TestCompressPrefix_DifferentRoots(t *testing.T) {
	nodes := []*parser.ContextNode{
		{SourceFile: "/tmp/a.go"},
		{SourceFile: "/var/b.go"},
	}
	orig0, orig1 := nodes[0].SourceFile, nodes[1].SourceFile
	compressPrefix(nodes, "")
	if nodes[0].SourceFile != orig0 || nodes[1].SourceFile != orig1 {
		t.Errorf("paths changed: %q, %q", nodes[0].SourceFile, nodes[1].SourceFile)
	}
}

func TestCompressPrefix_ExplicitRoot(t *testing.T) {
	cases := []struct {
		name        string
		compileRoot string
	}{
		{"trailing-separator", "/home/me/notes/"},
		{"no-trailing-separator", "/home/me/notes"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nodes := []*parser.ContextNode{
				{SourceFile: "/home/me/notes/foo.md"},
				{SourceFile: "/home/me/notes/sub/bar.md"},
			}
			compressPrefix(nodes, tc.compileRoot)

			want := []string{"foo.md", "sub/bar.md"}
			for i, n := range nodes {
				if n.SourceFile != want[i] {
					t.Errorf("nodes[%d].SourceFile = %q, want %q", i, n.SourceFile, want[i])
				}
			}
		})
	}
}

func TestCompressPrefix_StableAcrossCallShapes(t *testing.T) {
	const root = "/home/me/notes"

	batch := []*parser.ContextNode{
		{SourceFile: "/home/me/notes/foo.md"},
		{SourceFile: "/home/me/notes/sub/bar.md"},
		{SourceFile: "/home/me/notes/sub/baz.md"},
	}
	compressPrefix(batch, root)

	solo := []*parser.ContextNode{
		{SourceFile: "/home/me/notes/sub/bar.md"},
	}
	compressPrefix(solo, root)

	if batch[1].SourceFile != solo[0].SourceFile {
		t.Errorf("unstable strip: batch=%q solo=%q", batch[1].SourceFile, solo[0].SourceFile)
	}
	if want := "sub/bar.md"; batch[1].SourceFile != want {
		t.Errorf("batch[1].SourceFile = %q, want %q", batch[1].SourceFile, want)
	}
}
