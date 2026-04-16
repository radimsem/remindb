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
	f.Add("", []byte("no extension"))
	f.Add("file.unknown", []byte("unsupported"))
	f.Add("FILE.MD", []byte("# Upper case extension"))
	f.Add("file.md", []byte{})
	f.Add("file.md", []byte("---\nkey: val\n---\n# Body"))
	f.Add("file.yaml", []byte("- a\n- b\n- c\n- d\n- e\n- f"))
	f.Add("file.json", []byte(`{"a":{"b":{"c":{"d":"deep"}}}}`))
	f.Add("file.md", []byte("---\n+++\n---\n# mixed delimiters"))

	f.Add("file.md", []byte{0xe3})         // incomplete multi-byte sequence
	f.Add("file.json", []byte{0xff, 0xfe}) // BOM-like invalid bytes

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
