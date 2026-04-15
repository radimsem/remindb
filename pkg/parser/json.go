package parser

import (
	"bytes"
	"encoding/json"
	"fmt"
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

	if root == nil {
		return nil, nil
	}

	return []*ContextNode{buildNode(path, "", root, 1)}, nil
}
