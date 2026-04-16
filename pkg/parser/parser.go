package parser

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Read r and route it to a format-specific parser based on path's extension.
func Parse(path string, r io.Reader) ([]*ContextNode, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read: %s: %w", path, err)
	}

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".md", ".markdown":
		return parseMarkdown(path, data)
	case ".yml", ".yaml":
		return parseYaml(path, data)
	case ".json":
		return parseJson(path, data)
	case ".toon":
		return parseToon(path, data)
	default:
		return nil, fmt.Errorf("unsupported extension %q", ext)
	}
}

// Read path from disk and parse it.
func ParseFile(path string) ([]*ContextNode, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read: %s: %w", path, err)
	}

	return ParseBytes(path, data)
}

func ParseBytes(path string, data []byte) ([]*ContextNode, error) {
	return Parse(path, bytes.NewReader(data))
}
