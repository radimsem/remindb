// Package pathmatch parses gitignore-style sidecar files under .remindb/.
package pathmatch

import (
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

type Matcher struct {
	patterns []pattern
}

type pattern struct {
	raw      string
	segments []string
	dirOnly  bool
	anchored bool
	negated  bool
}

// Report whether relPath matches the pattern set; last-match-wins gitignore semantics.
func (m *Matcher) Match(relPath string, isDir bool) bool {
	if m == nil || len(m.patterns) == 0 {
		return false
	}

	relPath = filepath.ToSlash(relPath)
	if relPath == "" || relPath == "." {
		return false
	}

	pathSegs := strings.Split(relPath, "/")
	matched := false
	for _, p := range m.patterns {
		if p.dirOnly && !isDir {
			continue
		}
		if matchPattern(p, pathSegs) {
			matched = !p.negated
		}
	}
	return matched
}

func parsePattern(raw string) (pattern, error) {
	p := pattern{raw: raw}
	s := raw

	// A leading \! or \# escapes the special meaning of the first char.
	escaped := false
	if len(s) >= 2 && s[0] == '\\' && (s[1] == '!' || s[1] == '#') {
		s = s[1:]
		escaped = true
	}

	if !escaped && strings.HasPrefix(s, "!") {
		p.negated = true
		s = s[1:]
	}

	if strings.HasPrefix(s, "/") {
		p.anchored = true
		s = s[1:]
	}
	if strings.HasSuffix(s, "/") {
		p.dirOnly = true
		s = strings.TrimSuffix(s, "/")
	}
	if !p.anchored && strings.Contains(s, "/") {
		p.anchored = true
	}

	if s == "" {
		return pattern{}, fmt.Errorf("empty pattern: %s", raw)
	}

	segs := strings.Split(s, "/")
	if slices.Contains(segs, "") {
		return pattern{}, fmt.Errorf("empty path segment (consecutive slashes): %s", raw)
	}

	for _, seg := range segs {
		if seg == "**" {
			continue
		}
		if _, err := path.Match(seg, ""); err != nil {
			return pattern{}, fmt.Errorf("invalid pattern %q: %w", raw, err)
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
