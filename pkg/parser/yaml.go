package parser

import (
	"fmt"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
)

// YamlParser converts YAML source into ContextNode trees. It carries no
// state; the struct exists so the helper methods don't need to share a
// yaml prefix.
type YamlParser struct{}

// parseYaml is the dispatcher's entry point for YAML: it constructs a
// YamlParser and delegates to its parse method.
func parseYaml(path string, data []byte) ([]*ContextNode, error) {
	return YamlParser{}.parse(path, data)
}

// parse unmarshals data as YAML under source path and returns top-level
// ContextNodes. Map keys are emitted in sorted order so content hashes
// are stable across re-parses of the same document.
func (p YamlParser) parse(path string, data []byte) ([]*ContextNode, error) {
	var root any
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parser: yaml %s: %w", path, err)
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
		return []*ContextNode{{
			SourceFile: path,
			NodeType:   NodeKV,
			Content:    p.formatScalar(v),
			Depth:      1,
		}}, nil
	}
}

// fromMap converts a YAML mapping to ContextNodes, one per key, in
// sorted-key order for determinism.
func (p YamlParser) fromMap(path string, m map[string]any, depth int) []*ContextNode {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]*ContextNode, 0, len(keys))
	for _, k := range keys {
		out = append(out, p.fromKV(path, k, m[k], depth))
	}
	return out
}

// fromKV produces one ContextNode for a single key-value pair. Scalars
// collapse to NodeKV with content "key: value"; nested maps become NodeKV
// with child nodes; sequences become NodeList.
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
		return &ContextNode{
			SourceFile: path,
			NodeType:   NodeKV,
			Content:    key + ": " + p.formatScalar(v),
			Depth:      depth,
		}
	}
}

// fromSlice converts a YAML sequence to a NodeList. Flat scalar lists
// inline items as dash-prefixed lines; sequences holding maps or nested
// sequences hang each item off as a child node.
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
		return &ContextNode{
			SourceFile: path,
			NodeType:   NodeList,
			Content:    head + ":\n" + strings.Join(lines, "\n"),
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

// sliceIsFlat reports whether every element of items is a YAML scalar.
// A flat slice renders inline; a non-flat slice expands into child nodes.
func (p YamlParser) sliceIsFlat(items []any) bool {
	for _, it := range items {
		switch it.(type) {
		case map[string]any, map[any]any, []any:
			return false
		}
	}
	return true
}

// normalizeKeys turns a non-string-keyed YAML mapping into a string-keyed
// one by stringifying each key. Used for YAML maps whose keys are
// integers, booleans, or other scalar types.
func (p YamlParser) normalizeKeys(m map[any]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[fmt.Sprint(k)] = v
	}
	return out
}

// formatScalar renders a YAML scalar as its canonical string form.
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
