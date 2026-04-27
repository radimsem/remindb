// Package ignore filters source-tree walks via a .remindb.ignore sidecar file.
package ignore

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const FileName = ".remindb.ignore"

type Matcher struct {
	patterns []pattern
}

type pattern struct {
	raw      string
	segments []string
	dirOnly  bool
	anchored bool
}

// Load reads <dir>/.remindb.ignore. Returns (nil, nil) if absent.
func Load(dir string) (*Matcher, error) {
	f, err := os.Open(filepath.Join(dir, FileName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read: %s: %w", FileName, err)
	}
	defer func() { _ = f.Close() }()

	var patterns []pattern
	scanner := bufio.NewScanner(f)
	line := 0
	for scanner.Scan() {
		line++

		raw := strings.TrimSpace(scanner.Text())
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}

		p, err := parsePattern(raw)
		if err != nil {
			return nil, fmt.Errorf("unsupported pattern at line %d: %w", line, err)
		}
		patterns = append(patterns, p)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read: %s: %w", FileName, err)
	}

	return &Matcher{patterns: patterns}, nil
}

// Match reports whether relPath is excluded. relPath is relative to the source
// root, slash-separated. isDir distinguishes dir patterns (trailing "/") from
// file patterns. A nil receiver returns false.
func (m *Matcher) Match(relPath string, isDir bool) bool {
	if m == nil || len(m.patterns) == 0 {
		return false
	}

	relPath = filepath.ToSlash(relPath)
	if relPath == "" || relPath == "." {
		return false
	}

	pathSegs := strings.Split(relPath, "/")
	for _, p := range m.patterns {
		if p.dirOnly && !isDir {
			continue
		}
		if matchPattern(p, pathSegs) {
			return true
		}
	}
	return false
}

func parsePattern(raw string) (pattern, error) {
	if strings.HasPrefix(raw, "!") {
		return pattern{}, fmt.Errorf("negation (leading %q) not supported: %s", "!", raw)
	}
	if strings.HasPrefix(raw, "/") {
		return pattern{}, fmt.Errorf("leading slash anchor not supported: %s", raw)
	}
	if strings.ContainsAny(raw, "[]?\\") {
		return pattern{}, fmt.Errorf("char ranges, ? wildcards, and escapes not supported: %s", raw)
	}

	p := pattern{raw: raw}
	s := raw

	if strings.HasSuffix(s, "/") {
		p.dirOnly = true
		s = strings.TrimSuffix(s, "/")
	}
	if strings.Contains(s, "/") {
		p.anchored = true
	}

	if s == "" {
		return pattern{}, fmt.Errorf("empty pattern: %s", raw)
	}

	segs := strings.Split(s, "/")
	for _, seg := range segs {
		if seg == "" {
			return pattern{}, fmt.Errorf("empty path segment (consecutive slashes): %s", raw)
		}
	}
	p.segments = segs

	return p, nil
}

func matchPattern(p pattern, pathSegs []string) bool {
	if !p.anchored {
		return matchSegment(p.segments[0], pathSegs[len(pathSegs)-1])
	}
	return matchPath(p.segments, pathSegs)
}

func matchPath(patSegs, pathSegs []string) bool {
	if len(patSegs) == 0 {
		return len(pathSegs) == 0
	}
	if patSegs[0] == "**" {
		for i := 0; i <= len(pathSegs); i++ {
			if matchPath(patSegs[1:], pathSegs[i:]) {
				return true
			}
		}
		return false
	}
	if len(pathSegs) == 0 {
		return false
	}
	if !matchSegment(patSegs[0], pathSegs[0]) {
		return false
	}
	return matchPath(patSegs[1:], pathSegs[1:])
}

func matchSegment(pat, seg string) bool {
	ok, _ := path.Match(pat, seg)
	return ok
}
