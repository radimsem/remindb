package transformer

import (
	"path/filepath"
	"strings"

	"github.com/radimsem/remindb/pkg/parser"
)

// Strip the longest common directory prefix from all SourceFile paths.
func compressPrefix(nodes []*parser.ContextNode) {
	if len(nodes) < 2 {
		return
	}

	prefix := commonDirPrefix(nodes)
	if prefix == "" {
		return
	}

	for _, n := range nodes {
		n.SourceFile = strings.TrimPrefix(n.SourceFile, prefix)
	}
}

func commonDirPrefix(nodes []*parser.ContextNode) string {
	parts := splitPath(filepath.Dir(nodes[0].SourceFile))

	for _, n := range nodes[1:] {
		np := splitPath(filepath.Dir(n.SourceFile))
		parts = commonParts(parts, np)
		if len(parts) == 0 {
			return ""
		}
	}

	result := strings.Join(parts, string(filepath.Separator))
	if result == "" || result == "." || result == "/" {
		return ""
	}

	return result + string(filepath.Separator)
}

func splitPath(p string) []string {
	p = filepath.Clean(p)
	if p == "." {
		return nil
	}

	return strings.Split(p, string(filepath.Separator))
}

func commonParts(a, b []string) []string {
	n := len(a)
	m := len(b)
	if m < n {
		n = m
	}

	i := 0
	for i < n && a[i] == b[i] {
		i++
	}

	return a[:i]
}
