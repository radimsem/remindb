package parser

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

type JsonParser struct{}

func parseJson(path string, data []byte) ([]*ContextNode, error) {
	return JsonParser{}.parse(path, data)
}

func parseJsonLines(path string, data []byte) ([]*ContextNode, error) {
	return JsonParser{}.parseLines(path, data)
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

// Decode a stream of whitespace-separated JSON values and wrap them as a single list node.
func (p JsonParser) parseLines(path string, data []byte) ([]*ContextNode, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()

	var records []any
	for {
		var v any
		err := dec.Decode(&v)

		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to parse: jsonl %s: %w", path, err)
		}

		records = append(records, v)
	}

	if len(records) == 0 {
		return nil, nil
	}

	return []*ContextNode{buildNode(path, "", records, 1)}, nil
}
