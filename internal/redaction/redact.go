// Package redaction scrubs secrets from content on the way into the store.
package redaction

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

type Hit struct {
	Kind  string
	Start int
	End   int
}

type Redactor struct {
	patterns []kindPattern
}

func New(cfg Config) (*Redactor, error) {
	pats := make([]kindPattern, 0, len(cfg.BuiltinKinds)+len(cfg.Custom))

	for _, kind := range cfg.BuiltinKinds {
		re, ok := builtinPatterns[kind]
		if !ok {
			return nil, fmt.Errorf("unknown built-in kind: %s", kind)
		}
		pats = append(pats, kindPattern{kind: kind, re: re})
	}

	for _, cp := range cfg.Custom {
		if cp.Kind == "" {
			return nil, fmt.Errorf("custom pattern kind must be non-empty")
		}

		re, err := regexp.Compile(cp.Pattern)
		if err != nil {
			return nil, fmt.Errorf("failed to compile: custom pattern %q: %w", cp.Kind, err)
		}

		pats = append(pats, kindPattern{kind: cp.Kind, re: re})
	}

	return &Redactor{patterns: pats}, nil
}

// A nil receiver is a no-op that returns the input unchanged.
func (r *Redactor) Scrub(s string) (string, []Hit) {
	if r == nil || s == "" || len(r.patterns) == 0 {
		return s, nil
	}

	var hits []Hit
	for _, kp := range r.patterns {
		for _, idx := range kp.re.FindAllStringIndex(s, -1) {
			hits = append(hits, Hit{Kind: kp.kind, Start: idx[0], End: idx[1]})
		}
	}
	if len(hits) == 0 {
		return s, nil
	}

	// Earliest first; longest wins on ties so a containing pattern shadows inner matches.
	slices.SortFunc(hits, func(a, b Hit) int {
		if a.Start != b.Start {
			return cmp.Compare(a.Start, b.Start)
		}
		return cmp.Compare(b.End, a.End)
	})

	accepted := hits[:0]
	lastEnd := 0
	for _, h := range hits {
		if h.Start < lastEnd {
			continue
		}

		accepted = append(accepted, h)
		lastEnd = h.End
	}

	var b strings.Builder
	b.Grow(len(s))
	cursor := 0

	for _, h := range accepted {
		b.WriteString(s[cursor:h.Start])
		b.WriteString("«redacted:")
		b.WriteString(h.Kind)
		b.WriteString("»")
		cursor = h.End
	}
	b.WriteString(s[cursor:])

	return b.String(), accepted
}

func KindCounts(hits []Hit) map[string]int {
	if len(hits) == 0 {
		return nil
	}

	counts := make(map[string]int, len(hits))
	for _, h := range hits {
		counts[h.Kind]++
	}

	return counts
}

func Kinds() []string {
	return slices.Clone(builtinKindOrder)
}
