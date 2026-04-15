package parser

import (
	"bytes"
	"fmt"
)

// preambleKind identifies which delimiter pair wrapped the preamble at the
// start of a markdown file.
type preambleKind int

const (
	preambleNone preambleKind = iota
	preambleYaml
	preambleToml
)

// splitPreamble separates a leading preamble from the body of a markdown
// file. Supported delimiter pairs are "---" (YAML) and "+++" (TOML). If no
// preamble is detected, kind is preambleNone and body is the full input.
func splitPreamble(data []byte) (front, body []byte, kind preambleKind) {
	switch {
	case hasDelimLine(data, "---"):
		return splitAtDelim(data, "---", preambleYaml)
	case hasDelimLine(data, "+++"):
		return splitAtDelim(data, "+++", preambleToml)
	}
	return nil, data, preambleNone
}

// hasDelimLine reports whether data begins with delim followed by a newline.
func hasDelimLine(data []byte, delim string) bool {
	return bytes.HasPrefix(data, []byte(delim+"\n")) ||
		bytes.HasPrefix(data, []byte(delim+"\r\n"))
}

// splitAtDelim expects data to open with delim on its own line and scans for
// a matching closing line. On success, returns the bytes between the two
// delimiters, the bytes after the closing newline, and kind. On failure
// (no closing delimiter), returns the full input as body with kind
// preambleNone.
func splitAtDelim(data []byte, delim string, kind preambleKind) (front, body []byte, outKind preambleKind) {
	after := data[len(delim):]
	after = trimLeadingNewline(after)

	closing := "\n" + delim
	idx := bytes.Index(after, []byte(closing))
	if idx < 0 {
		return nil, data, preambleNone
	}

	front = after[:idx]
	rest := after[idx+len(closing):]

	switch {
	case len(rest) == 0:
		return front, nil, kind
	case bytes.HasPrefix(rest, []byte("\r\n")):
		return front, rest[2:], kind
	case bytes.HasPrefix(rest, []byte("\n")):
		return front, rest[1:], kind
	}

	return nil, data, preambleNone
}

// trimLeadingNewline strips a single leading "\n" or "\r\n" from b.
func trimLeadingNewline(b []byte) []byte {
	switch {
	case bytes.HasPrefix(b, []byte("\r\n")):
		return b[2:]
	case bytes.HasPrefix(b, []byte("\n")):
		return b[1:]
	}
	return b
}

// preambleNode parses front as a preamble block of the given kind and
// returns a NodePreamble attached at depth 1. YAML preambles are parsed
// by YamlParser and their top-level nodes become children; TOML preambles
// are kept as raw text since this project doesn't ship a TOML parser yet.
func preambleNode(path string, front []byte, kind preambleKind) (*ContextNode, error) {
	switch kind {
	case preambleYaml:
		children, err := YamlParser{}.parse(path, front)
		if err != nil {
			return nil, fmt.Errorf("parser: preamble %s: %w", path, err)
		}

		for _, c := range children {
			c.Depth = 2
		}

		return &ContextNode{
			SourceFile: path,
			NodeType:   NodePreamble,
			Content:    "preamble",
			Depth:      1,
			Children:   children,
		}, nil

	case preambleToml:
		return &ContextNode{
			SourceFile: path,
			NodeType:   NodePreamble,
			Content:    string(bytes.TrimSpace(front)),
			Depth:      1,
		}, nil
	}

	return nil, nil
}
