package parser

import (
	"errors"
	"testing"
	"unicode/utf8"
)

func FuzzParseBytes(f *testing.F) {
	f.Add("doc.md", []byte("# Hello\nSome text"))
	f.Add("data.yaml", []byte("key: value\nnested:\n  a: 1"))
	f.Add("data.json", []byte(`{"name":"test","items":[1,2,3]}`))
	f.Add("file.toon", []byte("col1\tcol2\nval1\tval2"))
	f.Add("page.html", []byte("<h1>Title</h1><p>Body <a href=\"x\">link</a></p>"))
	f.Add("page.html", []byte("<html></html>"))
	f.Add("page.html", []byte("<p>open<span>unclosed"))
	f.Add("page.html", []byte("<p>visible</p><script>alert(1)</script><style>p{}</style>"))
	f.Add("PAGE.HTML", []byte("<img src=\"x.png\" alt=\"a\">"))
	f.Add("page.htm", []byte("<table><tr><td>a</td></tr></table>"))
	f.Add("", []byte("no extension"))
	f.Add("file.unknown", []byte("unsupported"))
	f.Add("FILE.MD", []byte("# Upper case extension"))
	f.Add("file.md", []byte{})
	f.Add("file.md", []byte("---\nkey: val\n---\n# Body"))
	f.Add("file.yaml", []byte("- a\n- b\n- c\n- d\n- e\n- f"))
	f.Add("file.json", []byte(`{"a":{"b":{"c":{"d":"deep"}}}}`))
	f.Add("file.md", []byte("---\n+++\n---\n# mixed delimiters"))

	f.Add("file.md", []byte{0xe3})
	f.Add("file.json", []byte{0xff, 0xfe})

	f.Add("page.html", []byte("<math><mfrac><mi>x</mi></mfrac></math>"))
	f.Add("page.html", []byte("<math><mroot></mroot></math>"))
	f.Add("page.html", []byte("<math><mi>x</mi><mo>="))
	f.Add("page.html", []byte("<math><mmultiscripts><mi>x</mi></mmultiscripts>"))
	f.Add("page.html", []byte("<math><ms>contains{brace}</ms></math>"))
	f.Add("page.html", []byte("<math><munderover><mo>∑</mo><mi>i</mi></munderover></math>"))

	f.Fuzz(func(t *testing.T, path string, data []byte) {
		// Must never panic regardless of input.
		nodes, err := ParseBytes(path, data)

		if !utf8.Valid(data) {
			if !errors.Is(err, ErrInvalidUTF8) {
				t.Errorf("expected ErrInvalidUTF8 for invalid UTF-8 input, got: %v", err)
			}
			return
		}

		if err != nil {
			return
		}
		for _, n := range nodes {
			if n.SourceFile != path {
				t.Errorf("SourceFile = %q, want %q", n.SourceFile, path)
			}
		}
	})
}

func FuzzExtractWikilinks(f *testing.F) {
	f.Add("[[Architecture]]")
	f.Add("[[X; w=1.5]]")
	f.Add("[[X; w=not-a-number]]")
	f.Add("[[X; w=1; source=docs/x.md; id=3kGXxidmWBp]]")
	f.Add("[[]]")
	f.Add("[[ ]]")
	f.Add("[[X; ]]")
	f.Add("[[X; =empty]]")
	f.Add("[[a]] and [[b]]")
	f.Add("[[a [[ nested ]]")
	f.Add("[[[X]]]")
	f.Add("no links here")
	f.Add("[[3kGXxidmWBp]]")
	f.Add("[[What; why; how]]")
	f.Add("")
	f.Add("[[")
	f.Add("]]")
	f.Add("[[\x00\xff\xfe]]")
	f.Add("[[" + string(make([]byte, 4096)) + "]]")

	f.Fuzz(func(t *testing.T, in string) {
		// Must never panic, must always return well-formed output.
		out, refs := ExtractWikilinks(in)

		for i, r := range refs {
			if r.Label == "" {
				t.Errorf("ref[%d] has empty Label: in=%q out=%q refs=%+v", i, in, out, refs)
			}
		}

		// Idempotence: extracting from the rewritten output must not change it.
		out2, _ := ExtractWikilinks(out)
		if out != out2 {
			t.Errorf("not idempotent: in=%q first=%q second=%q", in, out, out2)
		}
	})
}

func FuzzSplitPreamble(f *testing.F) {
	f.Add([]byte("---\nkey: val\n---\n# body"))
	f.Add([]byte("+++\ntitle = \"t\"\n+++\nbody"))
	f.Add([]byte("no preamble at all"))
	f.Add([]byte("---\nunclosed preamble"))
	f.Add([]byte("---\r\nkey: val\r\n---\r\nbody"))
	f.Add([]byte("---\n---\n"))
	f.Add([]byte("---\n"))
	f.Add([]byte(""))
	f.Add([]byte("---"))
	f.Add([]byte("---\n---"))
	f.Add([]byte("+++\n+++"))
	f.Add([]byte("---\ncontent with --- inside\n---\nbody"))

	f.Fuzz(func(t *testing.T, data []byte) {
		front, body, kind := splitPreamble(data)

		if kind == preambleNone {
			if front != nil {
				t.Error("front must be nil when kind is preambleNone")
			}
			if string(body) != string(data) {
				t.Error("body must equal data when no preamble detected")
			}
			return
		}

		// When a preamble is detected, front + body must not exceed original data.
		if len(front)+len(body) > len(data) {
			t.Errorf("front (%d) + body (%d) > data (%d)", len(front), len(body), len(data))
		}
	})
}
