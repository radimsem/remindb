package parser

import (
	"fmt"

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

	root = p.normalize(root)
	if root == nil {
		return nil, nil
	}

	return []*ContextNode{buildNode(path, "", root, 1)}, nil
}

// Walk v and convert every map[any]any to map[string]any so downstream code only handles a single map shape.
func (p YamlParser) normalize(v any) any {
	switch x := v.(type) {
	case map[any]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[fmt.Sprint(k)] = p.normalize(val)
		}
		return out
	case map[string]any:
		for k, val := range x {
			x[k] = p.normalize(val)
		}
		return x
	case []any:
		for i, item := range x {
			x[i] = p.normalize(item)
		}
		return x
	}
	return v
}
