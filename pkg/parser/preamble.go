package parser

import (
	"bytes"
	"fmt"
)

type preambleKind int

const (
	preambleNone preambleKind = iota
	preambleYaml
	preambleToml
)

// Separate a leading preamble from the body of a markdown file.
func splitPreamble(data []byte) (front, body []byte, kind preambleKind) {
	switch {
	case hasDelimLine(data, "---"):
		return splitAtDelim(data, "---", preambleYaml)
	case hasDelimLine(data, "+++"):
		return splitAtDelim(data, "+++", preambleToml)
	}
	return nil, data, preambleNone
}

// Report whether data begins with delim followed by a newline.
func hasDelimLine(data []byte, delim string) bool {
	return bytes.HasPrefix(data, []byte(delim+"\n")) ||
		bytes.HasPrefix(data, []byte(delim+"\r\n"))
}

// Scan for a matching closing line and split data into (front, body).
func splitAtDelim(data []byte, delim string, kind preambleKind) (front, body []byte, outKind preambleKind) {
	after := data[len(delim):]
	after = trimLeadingNewline(after)

	var rest []byte
	var ok bool
	front, rest, ok = bytes.Cut(after, []byte("\n"+delim))
	if !ok {
		return nil, data, preambleNone
	}

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

func trimLeadingNewline(b []byte) []byte {
	switch {
	case bytes.HasPrefix(b, []byte("\r\n")):
		return b[2:]
	case bytes.HasPrefix(b, []byte("\n")):
		return b[1:]
	}
	return b
}

// Build a NodePreamble from the raw preamble bytes of the given kind.
func preambleNode(path string, front []byte, kind preambleKind) (*ContextNode, error) {
	switch kind {
	case preambleYaml:
		nodes, err := YamlParser{}.parse(path, front)
		if err != nil {
			return nil, fmt.Errorf("failed to parse: preamble %s: %w", path, err)
		}
		if len(nodes) == 0 {
			return nil, nil
		}

		root := nodes[0]
		root.NodeType = NodePreamble
		return root, nil

	case preambleToml:
		content := string(bytes.TrimSpace(front))

		return &ContextNode{
			SourceFile: path,
			NodeType:   NodePreamble,
			Content:    content,
			Depth:      1,
			Format:     FormatPlain,
		}, nil
	}

	return nil, nil
}
