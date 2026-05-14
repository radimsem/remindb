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

func FormatBatch(result *Result, requested, missing []string) string {
	kept := make(map[string]bool, len(result.Nodes))
	for _, sn := range result.Nodes {
		kept[sn.Node.ID] = true
	}

	miss := make(map[string]bool, len(missing))
	for _, id := range missing {
		miss[id] = true
	}

	seen := make(map[string]bool, len(requested))
	var overBudget []string
	for _, id := range requested {
		if seen[id] || kept[id] || miss[id] {
			continue
		}
		seen[id] = true
		overBudget = append(overBudget, id)
	}

	if len(result.Nodes) == 0 && len(missing) == 0 && len(overBudget) == 0 {
		return "no results"
	}

	var b strings.Builder

	if len(result.Nodes) > 0 {
		b.WriteString(Format(result))
	}

	if len(missing) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n---\n\n")
		}
		fmt.Fprintf(&b, "not found: %s\n", strings.Join(missing, ", "))
	}

	if len(overBudget) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n---\n\n")
		}
		fmt.Fprintf(&b, "over budget: %s\n", strings.Join(overBudget, ", "))
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
