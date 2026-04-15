package parser

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// JsonParser converts JSON source into ContextNode trees. It carries no
// state; the struct exists so the helper methods don't need to share a
// json prefix.
type JsonParser struct{}

// parseJson is the dispatcher's entry point for JSON: it constructs a
// JsonParser and delegates to its parse method.
func parseJson(path string, data []byte) ([]*ContextNode, error) {
	return JsonParser{}.parse(path, data)
}

// parse unmarshals data as JSON under source path and returns top-level
// ContextNodes. Numbers are decoded as json.Number so their exact textual
// form is preserved for stable content hashes; map keys are emitted in
// sorted order for the same reason.
func (p JsonParser) parse(path string, data []byte) ([]*ContextNode, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()

	var root any
	if err := dec.Decode(&root); err != nil {
		return nil, fmt.Errorf("parser: json %s: %w", path, err)
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

// fromMap converts a JSON object to ContextNodes, one per key, in
// sorted-key order for determinism.
func (p JsonParser) fromMap(path string, m map[string]any, depth int) []*ContextNode {
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
// collapse to NodeKV with content "key: value"; nested objects become
// NodeKV with child nodes; arrays become NodeList.
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

// fromSlice converts a JSON array to a NodeList. Flat scalar arrays inline
// items as dash-prefixed lines; arrays holding objects or nested arrays
// hang each item off as a child node.
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

// sliceIsFlat reports whether every element of items is a JSON scalar.
// A flat slice renders inline; a non-flat slice expands into child nodes.
func (p JsonParser) sliceIsFlat(items []any) bool {
	for _, it := range items {
		switch it.(type) {
		case map[string]any, []any:
			return false
		}
	}
	return true
}

// formatScalar renders a JSON scalar as its canonical string form.
// json.Number is preserved verbatim so large integers and exact decimals
// survive the round-trip unchanged.
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
