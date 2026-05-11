package transformer

import (
	"path/filepath"
	"strings"

	"github.com/radimsem/remindb/pkg/parser"
)

// Strip compileRoot (or the longest common dir if empty) so hashed paths stay stable across call shapes.
func compressPrefix(nodes []*parser.ContextNode, compileRoot string) {
	if len(nodes) == 0 {
		return
	}

	prefix := compileRoot
	if prefix != "" {
		prefix = filepath.Clean(prefix)
		if !strings.HasSuffix(prefix, string(filepath.Separator)) {
			prefix += string(filepath.Separator)
		}
	} else {
		prefix = commonDirPrefix(nodes)
	}

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
	i := 0
	for i < min(len(a), len(b)) && a[i] == b[i] {
		i++
	}
	return a[:i]
}
