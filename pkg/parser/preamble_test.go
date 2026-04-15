package parser

import (
	"bytes"
	"testing"
)

func TestSplitPreamble_YAML(t *testing.T) {
	data := []byte("---\ntitle: foo\n---\nbody here\n")
	front, body, kind := splitPreamble(data)

	if kind != preambleYaml {
		t.Fatalf("kind = %v, want preambleYaml", kind)
	}
	if string(front) != "title: foo" {
		t.Errorf("front = %q, want %q", front, "title: foo")
	}
	if string(body) != "body here\n" {
		t.Errorf("body = %q, want %q", body, "body here\n")
	}
}

func TestSplitPreamble_TOML(t *testing.T) {
	data := []byte("+++\ntitle = \"foo\"\n+++\nbody\n")
	front, body, kind := splitPreamble(data)

	if kind != preambleToml {
		t.Fatalf("kind = %v, want preambleToml", kind)
	}
	if !bytes.Contains(front, []byte("title")) {
		t.Errorf("front = %q, expected to contain 'title'", front)
	}
	if string(body) != "body\n" {
		t.Errorf("body = %q, want %q", body, "body\n")
	}
}

func TestSplitPreamble_None(t *testing.T) {
	data := []byte("# Heading\nno preamble here\n")
	front, body, kind := splitPreamble(data)

	if kind != preambleNone {
		t.Errorf("kind = %v, want preambleNone", kind)
	}
	if front != nil {
		t.Errorf("front = %q, want nil", front)
	}
	if !bytes.Equal(body, data) {
		t.Errorf("body should be full data")
	}
}

func TestSplitPreamble_UnclosedDelim(t *testing.T) {
	data := []byte("---\ntitle: foo\nno closing delim\n")
	_, body, kind := splitPreamble(data)

	if kind != preambleNone {
		t.Errorf("kind = %v, want preambleNone for unclosed delim", kind)
	}
	if !bytes.Equal(body, data) {
		t.Errorf("body should fall back to full data")
	}
}

func TestSplitPreamble_CRLF(t *testing.T) {
	data := []byte("---\r\ntitle: foo\r\n---\r\nbody\r\n")
	front, body, kind := splitPreamble(data)

	if kind != preambleYaml {
		t.Fatalf("kind = %v, want preambleYaml", kind)
	}
	if !bytes.Contains(front, []byte("title")) {
		t.Errorf("front = %q", front)
	}
	if !bytes.Contains(body, []byte("body")) {
		t.Errorf("body = %q", body)
	}
}
