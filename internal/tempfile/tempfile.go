// Package tempfile resolves pre-seeded temperatures from .remindb/temperatures.json.
package tempfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/radimsem/remindb/pkg/config"
)

const (
	FileName = "temperatures.json"
	Path     = config.DirName + "/" + FileName
)

type entry struct {
	temp     *float64
	children map[string]*entry
}

type Resolver struct {
	root *entry
}

// Read <dir>/.remindb/temperatures.json; (nil, nil) if the file is absent.
func Load(dir string) (*Resolver, error) {
	data, err := os.ReadFile(filepath.Join(dir, config.DirName, FileName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read: %s: %w", Path, err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse: %s: %w", Path, err)
	}

	root, err := buildEntry(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", Path, err)
	}

	return &Resolver{root: root}, nil
}

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
	root := &entry{children: make(map[string]*entry, len(raw))}

	for k, v := range raw {
		leaf, err := parseValue(k, v)
		if err != nil {
			return nil, err
		}
		if err := insertKey(root, leaf, k); err != nil {
			return nil, err
		}
	}
	return root, nil
}

// Graft a leaf entry into the tree at the path encoded by key, splitting on /.
func insertKey(root, leaf *entry, key string) error {
	norm := strings.TrimSuffix(strings.TrimPrefix(filepath.ToSlash(key), "./"), "/")
	if norm == "" {
		return fmt.Errorf("empty key %q", key)
	}

	segments := strings.Split(norm, "/")
	for _, seg := range segments {
		if seg == "" || seg == "." || seg == ".." {
			return fmt.Errorf("invalid path segment %q in key %q", seg, key)
		}
	}

	curr := root
	last := len(segments) - 1
	for i, seg := range segments {
		if i == last {
			existing, ok := curr.children[seg]
			if !ok {
				curr.children[seg] = leaf
				return nil
			}
			return mergeEntry(existing, leaf, key)
		}

		next, ok := curr.children[seg]
		if !ok {
			next = &entry{children: make(map[string]*entry)}
			curr.children[seg] = next
		}

		if next.temp != nil {
			path := strings.Join(segments[:i+1], "/")
			return fmt.Errorf("conflicting temperatures for %q", path)
		}
		curr = next
	}
	return nil
}

// Merge src into dst at the resolved path, erroring on conflicting temperatures.
func mergeEntry(dst, src *entry, path string) error {
	if tempConflict(dst, src) {
		return fmt.Errorf("conflicting temperatures for %q", path)
	}
	if src.temp != nil {
		dst.temp = src.temp
		return nil
	}

	if dst.children == nil {
		dst.children = make(map[string]*entry, len(src.children))
	}
	for k, child := range src.children {
		existing, ok := dst.children[k]
		if !ok {
			dst.children[k] = child
			continue
		}

		if err := mergeEntry(existing, child, path+"/"+k); err != nil {
			return err
		}
	}
	return nil
}

func tempConflict(dst, src *entry) bool {
	if dst.temp != nil {
		return src.temp != nil || len(src.children) > 0
	}
	return src.temp != nil && len(dst.children) > 0
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
