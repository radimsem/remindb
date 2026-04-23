package transformer

import (
	"context"
	"fmt"
	"testing"

	"github.com/radimsem/remindb/pkg/parser"
)

func benchNodes(n int) []*parser.ContextNode {
	nodes := make([]*parser.ContextNode, 0, n)
	types := []parser.NodeType{
		parser.NodeHeading, parser.NodeList, parser.NodeCode,
		parser.NodeText, parser.NodeKV,
	}
	contents := []string{
		"Architecture Design Decisions for the Platform",
		"- First item in the list\n- Second item with more detail\n- Third item concludes",
		"go\nfunc main() {\n\tfmt.Println(\"hello\")\n}",
		"This is a paragraph of text. It has multiple sentences. The third one concludes the thought.",
		"name: gateway\nversion: 2.1.0\nport: 8080\nreplicas: 3",
	}

	for i := range n {
		idx := i % len(types)
		nodes = append(nodes, &parser.ContextNode{
			SourceFile: fmt.Sprintf("bench/doc_%d.md", i/10),
			NodeType:   types[idx],
			Content:    contents[idx],
			Format:     parser.FormatPlain,
			Depth:      (i % 4) + 1,
		})
	}
	return nodes
}

func BenchmarkTransform(b *testing.B) {
	for _, size := range []int{10, 100, 500} {
		b.Run(fmt.Sprintf("nodes/%d", size), func(b *testing.B) {
			nodes := benchNodes(size)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				for _, n := range nodes {
					n.ID = ""
					n.ContentHash = ""
					n.Label = ""
					n.TokenCount = 0
					n.ParentID = ""
				}
				b.StartTimer()
				_ = Transform(context.Background(), nodes)
			}
		})
	}
}

func BenchmarkCompress(b *testing.B) {
	n := &parser.ContextNode{}
	raw := "  Line one  \r\n\r\n\r\n  Line two  \r\n  Line three  \n\n\n\nLine four\n\n"
	b.ReportAllocs()
	for b.Loop() {
		n.Content = raw
		compress(n)
	}
}

func BenchmarkSetLabel(b *testing.B) {
	cases := []struct {
		name string
		node *parser.ContextNode
	}{
		{"heading", &parser.ContextNode{NodeType: parser.NodeHeading, Content: "Architecture Design Decisions for the Platform"}},
		{"list", &parser.ContextNode{NodeType: parser.NodeList, Content: "- First item\n- Second item\n- Third item\n- Fourth item"}},
		{"code", &parser.ContextNode{NodeType: parser.NodeCode, Content: "go\nfunc main() {\n\tfmt.Println(\"hello\")\n}"}},
		{"text", &parser.ContextNode{NodeType: parser.NodeText, Content: "This is a paragraph of text. It has multiple sentences. The third one is here."}},
		{"kv", &parser.ContextNode{NodeType: parser.NodeKV, Content: "name: gateway\nversion: 2.1.0\nport: 8080"}},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				tc.node.Label = ""
				setLabel(tc.node)
			}
		})
	}
}
