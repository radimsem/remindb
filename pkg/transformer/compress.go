package transformer

import (
	"strings"

	"github.com/radimsem/remindb/pkg/parser"
)

// Normalize whitespace: CRLF → LF, trim trailing spaces per line, collapse runs of blank lines, strip empty leading/trailing lines.
func compress(n *parser.ContextNode) {
	s := n.Content
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = trimTrailingPerLine(s)
	s = collapseBlankLines(s)
	s = trimEmptyEdges(s)
	n.Content = s
}

func trimTrailingPerLine(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return strings.Join(lines, "\n")
}

func collapseBlankLines(s string) string {
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return s
}

func trimEmptyEdges(s string) string {
	for strings.HasPrefix(s, "\n") {
		s = s[1:]
	}
	for strings.HasSuffix(s, "\n") {
		s = s[:len(s)-1]
	}
	return s
}
