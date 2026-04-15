package parser

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Parse reads r and parses it according to the extension of path. Only the
// extension is inspected; path itself is never opened. SourceFile on every
// returned node is set to path.
func Parse(path string, r io.Reader) ([]*ContextNode, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("parser: read %s: %w", path, err)
	}

	switch ext := strings.ToLower(filepath.Ext(path)); ext {
	case ".md", ".markdown":
		return parseMarkdown(path, data)
	case ".yml", ".yaml":
		return parseYaml(path, data)
	case ".json":
		return parseJson(path, data)
	default:
		return nil, fmt.Errorf("parser: unsupported extension %q", ext)
	}
}

// ParseFile reads path from disk and parses it.
func ParseFile(path string) ([]*ContextNode, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("parser: read %s: %w", path, err)
	}

	return ParseBytes(path, data)
}

// ParseBytes parses pre-read content.
func ParseBytes(path string, data []byte) ([]*ContextNode, error) {
	return Parse(path, bytes.NewReader(data))
}
