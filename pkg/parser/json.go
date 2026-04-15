package parser

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type JsonParser struct{}

func parseJson(path string, data []byte) ([]*ContextNode, error) {
	return JsonParser{}.parse(path, data)
}

func (p JsonParser) parse(path string, data []byte) ([]*ContextNode, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()

	var root any
	if err := dec.Decode(&root); err != nil {
		return nil, fmt.Errorf("failed to parse: json %s: %w", path, err)
	}

	switch v := root.(type) {
	case nil:
		return nil, nil
	case map[string]any:
		return p.fromMap(path, v, 1), nil
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

// Convert a JSON object to ContextNodes, one per key, in sorted-key order for determinism.
func (p JsonParser) fromMap(path string, m map[string]any, depth int) []*ContextNode {
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
func (p JsonParser) fromKV(path, key string, value any, depth int) *ContextNode {
	switch v := value.(type) {
	case map[string]any:
		return &ContextNode{
			SourceFile: path,
			NodeType:   NodeKV,
			Content:    key,
			Depth:      depth,
			Children:   p.fromMap(path, v, depth+1),
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

// Convert a JSON array to a NodeList.
func (p JsonParser) fromSlice(path, key string, items []any, depth int) *ContextNode {
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

// Report whether every element of items is a JSON scalar.
func (p JsonParser) sliceIsFlat(items []any) bool {
	for _, it := range items {
		switch it.(type) {
		case map[string]any, []any:
			return false
		}
	}
	return true
}

// Render a JSON scalar as its canonical string form.
func (p JsonParser) formatScalar(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case nil:
		return "null"
	default:
		return fmt.Sprint(x)
	}
}
