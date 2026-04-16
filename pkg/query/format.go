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
		fmt.Fprintf(&b, "[%s] %s\n", n.NodeType, n.Label)

		b.WriteString(n.Content)
		b.WriteByte('\n')
	}
	return b.String()
}
