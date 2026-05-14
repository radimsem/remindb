package query

import (
	"fmt"
	"strings"

	"github.com/radimsem/remindb/pkg/store"
)

func Format(result *Result) string {
	var b strings.Builder

	for i, sn := range result.Nodes {
		if i > 0 {
			b.WriteString("\n---\n\n")
		}
		n := sn.Node
		fmt.Fprintf(&b, "[%s] (score=%.2f)\n", n.NodeType, sn.Score)

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

func FormatRelated(related []*store.RelatedNode, budget int) string {
	if len(related) == 0 {
		return "no related nodes"
	}

	var b strings.Builder
	used := 0
	for _, r := range related {
		n := r.Node
		if budget > 0 && used+n.TokenCount > budget {
			break
		}

		fmt.Fprintf(&b, "[%s] %s (id=%s file=%s hop=%d weight=%.2f temp=%.2f tok=%d)\n",
			n.NodeType, n.Label, n.ID, n.SourceFile, r.Hop, r.Weight, n.Temperature, n.TokenCount)
		used += n.TokenCount
	}

	if b.Len() == 0 {
		return "no related nodes"
	}
	return b.String()
}
