package ignore

import "testing"

func FuzzParseAndMatch(f *testing.F) {
	seeds := []struct {
		pat, rel string
		isDir    bool
	}{
		{"*.md", "notes.md", false},
		{"", "", false},
		{"!keep.md", "keep.md", false},
		{"fo?.md", "foo.md", false},
		{"file[abc].md", "filea.md", false},
		{"\\!literal.md", "!literal.md", false},
		{"\\#hash.md", "#hash.md", false},
		{"/anchored.md", "anchored.md", false},
		{"cache/", "cache", true},
		{"**/sessions/**", "a/sessions/b.jsonl", false},
		{"a/**/b", "a/x/y/z/b", false},
		{"[", "x", false},
		{"foo\\*.md", "foo*.md", false},
		{"日本語/*.md", "日本語/x.md", false},
		{"a//b", "a/b", false},
	}
	for _, s := range seeds {
		f.Add(s.pat, s.rel, s.isDir)
	}

	f.Fuzz(func(t *testing.T, pat, rel string, isDir bool) {
		p, err := parsePattern(pat)
		if err != nil {
			return
		}
		m := &Matcher{patterns: []pattern{p}}

		r1 := m.Match(rel, isDir)
		r2 := m.Match(rel, isDir)
		if r1 != r2 {
			t.Errorf("non-deterministic Match: %v != %v (pat=%q rel=%q isDir=%v)", r1, r2, pat, rel, isDir)
		}

		// A lone negated pattern can only re-include, never ignore.
		if p.negated && r1 {
			t.Errorf("lone negated pattern ignored a path: pat=%q rel=%q", pat, rel)
		}
	})
}
