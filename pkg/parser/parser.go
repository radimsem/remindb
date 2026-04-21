package parser

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

var (
	ErrUnsupportedExt = errors.New("unsupported extension")
	ErrInvalidUTF8    = errors.New("invalid UTF-8")
)

// Read r and route it to a format-specific parser based on path's extension.
func Parse(path string, r io.Reader) ([]*ContextNode, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read: %s: %w", path, err)
	}
	return ParseBytes(path, data)
}

func ParseFile(path string) ([]*ContextNode, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read: %s: %w", path, err)
	}
	return ParseBytes(path, data)
}

func ParseBytes(path string, data []byte) ([]*ContextNode, error) {
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("%w: %s", ErrInvalidUTF8, path)
	}

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".md", ".markdown":
		return parseMarkdown(path, data)
	case ".yml", ".yaml":
		return parseYaml(path, data)
	case ".json":
		return parseJson(path, data)
	case ".jsonl", ".ndjson":
		return parseJsonLines(path, data)
	case ".toon":
		return parseToon(path, data)
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedExt, ext)
	}
}
