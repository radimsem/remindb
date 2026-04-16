package transformer

import (
	"fmt"
	"strings"

	"github.com/radimsem/remindb/pkg/parser"
)

const maxLabelLen = 80

func setLabel(n *parser.ContextNode) {
	switch n.NodeType {
	case parser.NodeHeading:
		n.Label = truncate(n.Content, maxLabelLen)
	case parser.NodeList:
		n.Label = labelList(n)
	case parser.NodeTable:
		n.Label = labelTable(n)
	case parser.NodeCode:
		n.Label = labelCode(n)
	case parser.NodeText:
		n.Label = firstSentence(n.Content, maxLabelLen)
	case parser.NodeKV:
		n.Label = labelKV(n)
	case parser.NodePreamble:
		n.Label = labelPreamble(n)
	default:
		n.Label = truncate(n.Content, maxLabelLen)
	}
}

func labelList(n *parser.ContextNode) string {
	if n.Format == parser.FormatToon {
		if idx := strings.IndexByte(n.Content, ':'); idx >= 0 {
			return truncate(n.Content[:idx], maxLabelLen)
		}
	}

	lines := strings.Split(n.Content, "\n")
	count := 0
	first := ""
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if !strings.HasPrefix(trimmed, "- ") {
			continue
		}

		count++
		if first == "" {
			first = strings.TrimPrefix(trimmed, "- ")
		}
	}

	if count == 0 {
		return truncate(n.Content, maxLabelLen)
	}

	label := fmt.Sprintf("%d-item list: %s", count, first)
	return truncate(label, maxLabelLen)
}

func labelTable(n *parser.ContextNode) string {
	lines := strings.Split(n.Content, "\n")
	if len(lines) == 0 {
		return "Table"
	}

	cols := strings.Join(strings.Split(lines[0], "\t"), ", ")
	rows := len(lines) - 1
	label := fmt.Sprintf("Table: %s (%d rows)", cols, rows)
	return truncate(label, maxLabelLen)
}

func labelCode(n *parser.ContextNode) string {
	lines := strings.SplitN(n.Content, "\n", 3)
	if len(lines) == 0 {
		return "Code"
	}

	first := strings.TrimSpace(lines[0])
	isLang := len(first) > 0 && len(first) < maxLabelLen && !strings.Contains(first, " ")

	if isLang && len(lines) > 1 {
		codeLine := strings.TrimSpace(lines[1])
		return truncate(fmt.Sprintf("Code (%s): %s", first, codeLine), maxLabelLen)
	}

	return truncate("Code: "+first, maxLabelLen)
}

func labelKV(n *parser.ContextNode) string {
	keys := extractTopKeys(n.Content, 3)
	if len(keys) == 0 {
		return truncate(n.Content, maxLabelLen)
	}
	return truncate(strings.Join(keys, ", "), maxLabelLen)
}

func labelPreamble(n *parser.ContextNode) string {
	keys := extractTopKeys(n.Content, 0)
	if len(keys) == 0 {
		return "Preamble"
	}
	return truncate("Preamble: "+strings.Join(keys, ", "), maxLabelLen)
}

func extractTopKeys(content string, limit int) []string {
	lines := strings.Split(content, "\n")
	var keys []string

	for _, l := range lines {
		if len(l) == 0 || l[0] == ' ' || l[0] == '\t' {
			continue
		}

		idx := strings.IndexByte(l, ':')
		if idx <= 0 {
			continue
		}

		keys = append(keys, l[:idx])
		if limit > 0 && len(keys) >= limit {
			break
		}
	}
	return keys
}

func firstSentence(s string, max int) string {
	for i, c := range s {
		if c != '.' && c != '!' && c != '?' {
			continue
		}

		end := i + 1
		if end >= len(s) || s[end] == ' ' || s[end] == '\n' {
			return truncate(s[:end], max)
		}
	}
	return truncate(s, max)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
