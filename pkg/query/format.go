package query

import (
	"fmt"
	"strings"
)

func Format(result *Result) string {
	var b strings.Builder

	for i, sn := range result.Nodes {
		if i > 0 {
			b.WriteString("\n---\n\n")
		}
		n := sn.Node
		fmt.Fprintf(&b, "[%s] %s (score=%.2f)\n", n.NodeType, n.Label, sn.Score)

		b.WriteString(n.Content)
		b.WriteByte('\n')
	}
	return b.String()
}

func FormatCompact(result *Result) string {
	if len(result.Nodes) == 0 {
		return "no results"
	}

	var b strings.Builder
	for _, sn := range result.Nodes {
		n := sn.Node
		fmt.Fprintf(&b, "[%s] %s (id=%s file=%s score=%.2f temp=%.2f tok=%d)\n",
			n.NodeType, n.Label, n.ID, n.SourceFile, sn.Score, n.Temperature, n.TokenCount)
	}
	return b.String()
}
