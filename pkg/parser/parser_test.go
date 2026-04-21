package parser

import (
	"bytes"
	"testing"
)

func TestParse_DispatchByExtension(t *testing.T) {
	tests := []struct {
		name string
		path string
		data string
	}{
		{"markdown", "t.md", "# H1\n"},
		{"markdown alt", "t.markdown", "# H1\n"},
		{"yaml", "t.yaml", "a: 1\n"},
		{"yml", "t.yml", "a: 1\n"},
		{"json", "t.json", `{"a": 1}`},
		{"jsonl", "t.jsonl", "{\"a\":1}\n{\"a\":2}\n"},
		{"ndjson", "t.ndjson", "{\"a\":1}\n{\"a\":2}\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodes, err := Parse(tt.path, bytes.NewReader([]byte(tt.data)))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if len(nodes) == 0 {
				t.Error("got zero nodes")
			}
		})
	}
}

func TestParse_UnknownExtension(t *testing.T) {
	_, err := Parse("t.xml", bytes.NewReader([]byte("")))
	if err == nil {
		t.Fatal("expected error for unsupported extension")
	}
}

func TestParseFile_NotFound(t *testing.T) {
	_, err := ParseFile("/this/path/does/not/exist.md")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestParseBytes_Equivalent(t *testing.T) {
	data := []byte("# H1\ntext\n")

	a, err := ParseBytes("t.md", data)
	if err != nil {
		t.Fatalf("ParseBytes: %v", err)
	}
	b, err := Parse("t.md", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(a) != len(b) || a[0].Content != b[0].Content {
		t.Errorf("ParseBytes ≠ Parse: %+v vs %+v", a, b)
	}
}
