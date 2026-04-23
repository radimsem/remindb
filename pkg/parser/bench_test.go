package parser

import (
	"os"
	"testing"

	mdparser "github.com/gomarkdown/markdown/parser"
)

func readBenchFile(b *testing.B, name string) []byte {
	b.Helper()
	data, err := os.ReadFile("../../testdata/bench/" + name)
	if err != nil {
		b.Fatal(err)
	}
	return data
}

func BenchmarkParseBytes(b *testing.B) {
	cases := []struct {
		name string
		file string
	}{
		{"markdown/small", "small.md"},
		{"markdown/medium", "medium.md"},
		{"markdown/large", "large.md"},
		{"yaml", "config.yaml"},
		{"json", "data.json"},
	}

	for _, tc := range cases {
		data := readBenchFile(b, tc.file)
		b.Run(tc.name, func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, _ = ParseBytes(tc.file, data)
			}
		})
	}
}

func BenchmarkExtractText(b *testing.B) {
	data := readBenchFile(b, "large.md")
	_, body, _ := splitPreamble(data)

	exts := mdparser.CommonExtensions | mdparser.Tables | mdparser.FencedCode
	p := mdparser.NewWithExtensions(exts)
	doc := p.Parse(body)

	children := doc.GetChildren()
	if len(children) == 0 {
		b.Fatal("no children in parsed doc")
	}

	// Pick the node with the most children for a realistic extraction.
	target := children[0]
	for _, c := range children {
		if len(c.GetChildren()) > len(target.GetChildren()) {
			target = c
		}
	}

	b.ReportAllocs()

	for b.Loop() {
		extractText(target)
	}
}
