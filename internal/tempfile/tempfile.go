// Package tempfile resolves pre-seeded temperatures from .temp.json files.
package tempfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const FileName = ".temp.json"

type entry struct {
	temp     *float64
	children map[string]*entry
}

type Resolver struct {
	root *entry
}

// Load reads a .temp.json from dir. Returns nil, nil if the file does not exist.
func Load(dir string) (*Resolver, error) {
	data, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read: %s: %w", FileName, err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse: %s: %w", FileName, err)
	}

	root, err := buildEntry(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", FileName, err)
	}

	return &Resolver{root: root}, nil
}

// Resolve returns the pre-seeded temperature for a relative file path.
func (r *Resolver) Resolve(relPath string) (float64, bool) {
	if r == nil || r.root == nil {
		return 0, false
	}

	parts := strings.Split(filepath.ToSlash(relPath), "/")
	curr := r.root
	var fallback *float64

	for _, seg := range parts {
		if g, ok := curr.children["*"]; ok && g.temp != nil {
			fallback = g.temp
		}

		child, ok := curr.children[seg]
		if !ok {
			break
		}
		if child.temp != nil {
			return *child.temp, true
		}

		curr = child
	}

	if fallback != nil {
		return *fallback, true
	}
	return 0, false
}

func buildEntry(raw map[string]any) (*entry, error) {
	e := &entry{children: make(map[string]*entry, len(raw))}

	for k, v := range raw {
		child, err := parseValue(k, v)
		if err != nil {
			return nil, err
		}
		e.children[k] = child
	}
	return e, nil
}

func parseValue(key string, v any) (*entry, error) {
	switch val := v.(type) {
	case float64:
		if val < 0 || val > 1 {
			return nil, fmt.Errorf("temperature for %q out of range [0, 1]: %g", key, val)
		}
		return &entry{temp: &val}, nil

	case map[string]any:
		return buildEntry(val)

	default:
		return nil, fmt.Errorf("unexpected type for %q: %T (expected number or object)", key, v)
	}
}
