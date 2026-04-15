package parser

import (
	"fmt"
	"sort"
	"strings"
)

// Build one ContextNode for value. Scalars become a "key: value" leaf; maps inline small subkeys and promote large ones; arrays render as leaf content.
func buildNode(path, key string, value any, depth int) *ContextNode {
	switch v := value.(type) {
	case map[string]any:
		return buildMap(path, key, v, depth)
	case []any:
		return buildList(path, key, v, depth)
	default:
		content := formatScalar(v)
		if key != "" {
			content = key + ": " + content
		}
		return &ContextNode{
			SourceFile: path,
			NodeType:   NodeKV,
			Content:    content,
			Depth:      depth,
			Format:     FormatPlain,
		}
	}
}

// Build a map node. Sub-threshold values inline into Content; above-threshold values promote to Children.
func buildMap(path, key string, m map[string]any, depth int) *ContextNode {
	n := &ContextNode{
		SourceFile: path,
		NodeType:   NodeKV,
		Depth:      depth,
		Format:     FormatPlain,
	}

	inlined := make(map[string]any, len(m))
	for _, k := range sortedKeys(m) {
		v := m[k]
		if promotes(v) {
			n.Children = append(n.Children, buildNode(path, k, v, depth+1))
			continue
		}
		inlined[k] = v
	}

	n.Content = renderMap(key, inlined)
	return n
}

// Build an array node. Arrays render as a single leaf; elements are not further recursed. TOON-encoded when uniform and savings beat the threshold.
func buildList(path, key string, items []any, depth int) *ContextNode {
	plain := renderList(key, items)
	content, format := plain, FormatPlain
	if encoded, ok := tryToonList(key, items, plain); ok {
		content = encoded
		format = FormatToon
	}

	return &ContextNode{
		SourceFile: path,
		NodeType:   NodeList,
		Content:    content,
		Depth:      depth,
		Format:     format,
	}
}

// A value promotes to its own node when it's a map or array with at least MaxInlineFields entries.
func promotes(v any) bool {
	switch x := v.(type) {
	case map[string]any:
		return len(x) >= MaxInlineFields
	case []any:
		return len(x) >= MaxInlineFields
	}
	return false
}

// Render a map under an optional leading key as block YAML-style text with sorted keys.
func renderMap(key string, m map[string]any) string {
	var sb strings.Builder
	if key != "" {
		sb.WriteString(key)
		sb.WriteByte(':')
		if len(m) == 0 {
			return sb.String()
		}
		sb.WriteByte('\n')
		writeMap(&sb, m, "  ")
		return sb.String()
	}
	writeMap(&sb, m, "")
	return sb.String()
}

// Render an array under an optional leading key as block YAML-style text.
func renderList(key string, items []any) string {
	var sb strings.Builder
	if key != "" {
		sb.WriteString(key)
		sb.WriteByte(':')
		if len(items) == 0 {
			sb.WriteString(" []")
			return sb.String()
		}
		sb.WriteByte('\n')
		writeList(&sb, items, "")
		return sb.String()
	}
	writeList(&sb, items, "")
	return sb.String()
}

func writeMap(sb *strings.Builder, m map[string]any, indent string) {
	for i, k := range sortedKeys(m) {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(indent)
		sb.WriteString(k)
		sb.WriteByte(':')
		writeValue(sb, m[k], indent)
	}
}

func writeList(sb *strings.Builder, items []any, indent string) {
	for i, it := range items {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(indent)
		sb.WriteByte('-')
		writeValue(sb, it, indent)
	}
}

func writeValue(sb *strings.Builder, v any, indent string) {
	switch x := v.(type) {
	case map[string]any:
		if len(x) == 0 {
			sb.WriteString(" {}")
			return
		}
		sb.WriteByte('\n')
		writeMap(sb, x, indent+"  ")
	case []any:
		if len(x) == 0 {
			sb.WriteString(" []")
			return
		}
		sb.WriteByte('\n')
		writeList(sb, x, indent)
	default:
		sb.WriteByte(' ')
		sb.WriteString(formatScalar(x))
	}
}

func formatScalar(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		return x
	default:
		return fmt.Sprint(x)
	}
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
