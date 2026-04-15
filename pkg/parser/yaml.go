package parser

import (
	"fmt"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
)

type YamlParser struct{}

func parseYaml(path string, data []byte) ([]*ContextNode, error) {
	return YamlParser{}.parse(path, data)
}

func (p YamlParser) parse(path string, data []byte) ([]*ContextNode, error) {
	var root any
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("failed to parse: yaml %s: %w", path, err)
	}

	switch v := root.(type) {
	case nil:
		return nil, nil
	case map[string]any:
		return p.fromMap(path, v, 1), nil
	case map[any]any:
		return p.fromMap(path, p.normalizeKeys(v), 1), nil
	case []any:
		return []*ContextNode{p.fromSlice(path, "", v, 1)}, nil
	default:
		content := p.formatScalar(v)

		return []*ContextNode{{
			SourceFile: path,
			NodeType:   NodeKV,
			Content:    content,
			Depth:      1,
		}}, nil
	}
}

// Convert a YAML mapping to ContextNodes, one per key, in sorted-key order for determinism.
func (p YamlParser) fromMap(path string, m map[string]any, depth int) []*ContextNode {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]*ContextNode, 0, len(keys))
	for _, k := range keys {
		v := m[k]
		out = append(out, p.fromKV(path, k, v, depth))
	}
	return out
}

// Produce one ContextNode for a single key-value pair.
func (p YamlParser) fromKV(path, key string, value any, depth int) *ContextNode {
	switch v := value.(type) {
	case map[string]any:
		return &ContextNode{
			SourceFile: path,
			NodeType:   NodeKV,
			Content:    key,
			Depth:      depth,
			Children:   p.fromMap(path, v, depth+1),
		}

	case map[any]any:
		return &ContextNode{
			SourceFile: path,
			NodeType:   NodeKV,
			Content:    key,
			Depth:      depth,
			Children:   p.fromMap(path, p.normalizeKeys(v), depth+1),
		}

	case []any:
		return p.fromSlice(path, key, v, depth)

	default:
		content := key + ": " + p.formatScalar(v)

		return &ContextNode{
			SourceFile: path,
			NodeType:   NodeKV,
			Content:    content,
			Depth:      depth,
		}
	}
}

// Convert a YAML sequence to a NodeList.
func (p YamlParser) fromSlice(path, key string, items []any, depth int) *ContextNode {
	head := key
	if head == "" {
		head = "list"
	}

	if p.sliceIsFlat(items) {
		lines := make([]string, 0, len(items))
		for _, it := range items {
			lines = append(lines, "- "+p.formatScalar(it))
		}
		content := head + ":\n" + strings.Join(lines, "\n")

		return &ContextNode{
			SourceFile: path,
			NodeType:   NodeList,
			Content:    content,
			Depth:      depth,
		}
	}

	n := &ContextNode{
		SourceFile: path,
		NodeType:   NodeList,
		Content:    head,
		Depth:      depth,
	}
	for i, it := range items {
		childKey := fmt.Sprintf("[%d]", i)
		n.Children = append(n.Children, p.fromKV(path, childKey, it, depth+1))
	}
	return n
}

// Report whether every element of items is a YAML scalar.
func (p YamlParser) sliceIsFlat(items []any) bool {
	for _, it := range items {
		switch it.(type) {
		case map[string]any, map[any]any, []any:
			return false
		}
	}
	return true
}

// Stringify non-string keys so the mapping is uniformly keyed by strings.
func (p YamlParser) normalizeKeys(m map[any]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[fmt.Sprint(k)] = v
	}
	return out
}

// Render a YAML scalar as its canonical string form.
func (p YamlParser) formatScalar(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return "null"
	default:
		return fmt.Sprint(x)
	}
}
