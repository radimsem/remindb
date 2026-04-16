package transformer

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/radimsem/remindb/pkg/parser"
)

func FuzzCompress(f *testing.F) {
	f.Add("hello world")
	f.Add("")
	f.Add("line1\nline2\nline3")
	f.Add("trailing spaces   \t\n  next line  ")
	f.Add("\n\n\n\n\nmany blanks\n\n\n\n\n")
	f.Add("\r\nwindows\r\nlines\r\n")
	f.Add("\r old mac \r lines \r")
	f.Add("   \t  \n  \t  \n  \t  ")
	f.Add(strings.Repeat("\n", 1000))
	f.Add("preserves  internal  double  spaces")

	f.Fuzz(func(t *testing.T, content string) {
		n := &parser.ContextNode{Content: content}
		compress(n)

		if strings.Contains(n.Content, "\r") {
			t.Error("compressed content still contains \\r")
		}
		if strings.Contains(n.Content, "\n\n\n") {
			t.Error("compressed content has 3+ consecutive newlines")
		}
		if strings.HasPrefix(n.Content, "\n") {
			t.Error("compressed content starts with newline")
		}
		if strings.HasSuffix(n.Content, "\n") {
			t.Error("compressed content ends with newline")
		}
	})
}

func FuzzTruncateAndLabel(f *testing.F) {
	f.Add("short", "heading")
	f.Add("", "heading")
	f.Add(strings.Repeat("a", 200), "heading")
	f.Add("Hello world. Second sentence.", "text")
	f.Add("- item one\n- item two\n- item three", "list")
	f.Add("col1\tcol2\nval1\tval2\nval3\tval4", "table")
	f.Add("go\nfmt.Println(\"hello\")\n", "code")
	f.Add("key1: val1\nkey2: val2\nkey3: val3\nkey4: val4", "kv")
	f.Add("name: test\nversion: 1.0", "preamble")
	f.Add(strings.Repeat("x", 10000), "text")
	f.Add("日本語テスト。次の文。", "text")
	f.Add("multi\nbyte\n日本語\ntruncation", "heading")

	f.Fuzz(func(t *testing.T, content, nodeType string) {
		nt := parser.NodeType(nodeType)
		switch nt {
		case parser.NodeHeading, parser.NodeList, parser.NodeTable,
			parser.NodeCode, parser.NodeText, parser.NodeKV,
			parser.NodePreamble:
		default:
			// Restrict to known types to keep tests meaningful.
			return
		}

		n := &parser.ContextNode{
			Content:  content,
			NodeType: nt,
			Format:   parser.FormatPlain,
		}
		setLabel(n)

		// Only check UTF-8 validity of output when input is valid UTF-8.
		if utf8.ValidString(content) && !utf8.ValidString(n.Label) {
			t.Errorf("label is not valid UTF-8 for valid UTF-8 content %q", content[:min(50, len(content))])
		}
		if len(n.Label) > maxLabelLen+3 {
			t.Errorf("label too long: %d > %d", len(n.Label), maxLabelLen+3)
		}
	})
}
