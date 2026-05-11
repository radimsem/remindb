package ignore

import (
	"os"
	"path/filepath"
	"testing"
)

func writeIgnore(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	m, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m != nil {
		t.Error("expected nil matcher for missing file")
	}
}

func TestLoad_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	writeIgnore(t, dir, "")

	m, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil matcher for empty file")
	}
	if m.Match("anything.md", false) {
		t.Error("empty matcher should not match")
	}
}

func TestLoad_CommentsAndBlanks(t *testing.T) {
	dir := t.TempDir()
	writeIgnore(t, dir, "# comment\n\n   \n# another\n")

	m, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil matcher")
	}
	if len(m.patterns) != 0 {
		t.Errorf("expected 0 patterns, got %d", len(m.patterns))
	}
}

func TestMatch_Negation(t *testing.T) {
	dir := t.TempDir()
	writeIgnore(t, dir, "*.md\n!keep.md\n")

	m, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if !m.Match("notes.md", false) {
		t.Error("notes.md should be ignored by *.md")
	}
	if m.Match("keep.md", false) {
		t.Error("keep.md should be re-included by !keep.md")
	}
	if !m.Match("a/notes.md", false) {
		t.Error("nested notes.md should be ignored")
	}
	if m.Match("a/keep.md", false) {
		t.Error("nested keep.md should be re-included")
	}
}

func TestMatch_LastMatchWins(t *testing.T) {
	dir := t.TempDir()
	// Re-ignore after a negation: order matters.
	writeIgnore(t, dir, "*.md\n!keep.md\nkeep.md\n")

	m, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if !m.Match("keep.md", false) {
		t.Error("trailing keep.md should win over !keep.md")
	}
}

func TestMatch_CharRange(t *testing.T) {
	dir := t.TempDir()
	writeIgnore(t, dir, "file[abc].md\n")

	m, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	for _, p := range []string{"filea.md", "fileb.md", "filec.md"} {
		if !m.Match(p, false) {
			t.Errorf("%q should match file[abc].md", p)
		}
	}
	if m.Match("filed.md", false) {
		t.Error("filed.md should not match file[abc].md")
	}
}

func TestMatch_QuestionWildcard(t *testing.T) {
	dir := t.TempDir()
	writeIgnore(t, dir, "fo?.md\n")

	m, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if !m.Match("foo.md", false) {
		t.Error("foo.md should match fo?.md")
	}
	if !m.Match("fob.md", false) {
		t.Error("fob.md should match fo?.md")
	}
	if m.Match("fooo.md", false) {
		t.Error("fooo.md should not match fo?.md (? is exactly one char)")
	}
}

func TestMatch_LeadingSlashAnchor(t *testing.T) {
	dir := t.TempDir()
	writeIgnore(t, dir, "/anchored.md\n")

	m, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if !m.Match("anchored.md", false) {
		t.Error("anchored.md at root should match /anchored.md")
	}
	if m.Match("a/anchored.md", false) {
		t.Error("nested anchored.md should not match /anchored.md")
	}
}

func TestMatch_EscapedLeadingChar(t *testing.T) {
	dir := t.TempDir()
	writeIgnore(t, dir, "\\!literal.md\n\\#hash.md\n")

	m, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if !m.Match("!literal.md", false) {
		t.Error("!literal.md should match \\!literal.md")
	}
	if !m.Match("#hash.md", false) {
		t.Error("#hash.md should match \\#hash.md")
	}
	if m.Match("literal.md", false) {
		t.Error("literal.md should not match \\!literal.md (negation is escaped)")
	}
}

func TestMatch_EscapedSegmentChar(t *testing.T) {
	dir := t.TempDir()
	writeIgnore(t, dir, "foo\\*.md\n")

	m, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if !m.Match("foo*.md", false) {
		t.Error("foo*.md should match foo\\*.md literally")
	}
	if m.Match("foobar.md", false) {
		t.Error("foobar.md should not match foo\\*.md (escaped star)")
	}
}

func TestLoad_UnsupportedPattern_DoubleSlash(t *testing.T) {
	dir := t.TempDir()
	writeIgnore(t, dir, "a//b.md\n")

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for consecutive slashes")
	}
}

func TestMatch_BasenameGlob(t *testing.T) {
	dir := t.TempDir()
	writeIgnore(t, dir, "*.jsonl\n")

	m, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	cases := []struct {
		path string
		want bool
	}{
		{"foo.jsonl", true},
		{"a/foo.jsonl", true},
		{"a/b/c/foo.jsonl", true},
		{"foo.json", false},
		{"jsonl.md", false},
	}
	for _, tt := range cases {
		got := m.Match(tt.path, false)
		if got != tt.want {
			t.Errorf("Match(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestMatch_LiteralBasename(t *testing.T) {
	dir := t.TempDir()
	writeIgnore(t, dir, "TODO\n")

	m, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if !m.Match("TODO", false) {
		t.Error("TODO at root should match")
	}
	if !m.Match("a/b/TODO", false) {
		t.Error("nested TODO should match")
	}
	if m.Match("TODO.md", false) {
		t.Error("TODO.md should not match TODO")
	}
}

func TestMatch_AnchoredPath(t *testing.T) {
	dir := t.TempDir()
	writeIgnore(t, dir, "cache/scratch.md\n")

	m, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if !m.Match("cache/scratch.md", false) {
		t.Error("anchored path should match at root")
	}
	if m.Match("a/cache/scratch.md", false) {
		t.Error("anchored path should not match nested")
	}
	if m.Match("cache/other.md", false) {
		t.Error("anchored path should not match siblings")
	}
}

func TestMatch_DirectoryOnly(t *testing.T) {
	dir := t.TempDir()
	writeIgnore(t, dir, "sessions/\n")

	m, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if !m.Match("sessions", true) {
		t.Error("sessions dir at root should match")
	}
	if !m.Match("a/b/sessions", true) {
		t.Error("nested sessions dir should match")
	}
	if m.Match("sessions", false) {
		t.Error("sessions as a file should not match dirOnly pattern")
	}
	if m.Match("sessions.md", true) {
		t.Error("sessions.md as a dir should not match sessions/")
	}
}

func TestMatch_DoubleStar(t *testing.T) {
	cases := []struct {
		pat  string
		path string
		want bool
	}{
		{"**/sessions/**", "a/sessions/b.jsonl", true},
		{"**/sessions/**", "x/y/sessions/z/w.md", true},
		{"**/sessions/**", "sessions/a.md", true},
		{"**/sessions/**", "no/match/here.md", false},

		{"a/**/b", "a/b", true},
		{"a/**/b", "a/x/b", true},
		{"a/**/b", "a/x/y/z/b", true},
		{"a/**/b", "a/b/c", false},
		{"a/**/b", "x/a/b", false},

		{"build/**", "build/out.txt", true},
		{"build/**", "build", true},
		{"build/**", "src/build/out.txt", false},
	}

	for _, tt := range cases {
		dir := t.TempDir()
		writeIgnore(t, dir, tt.pat+"\n")
		m, err := Load(dir)
		if err != nil {
			t.Fatalf("Load %q: %v", tt.pat, err)
		}
		got := m.Match(tt.path, false)
		if got != tt.want {
			t.Errorf("Match(%q, %q) = %v, want %v", tt.pat, tt.path, got, tt.want)
		}
	}
}

func TestMatch_NilReceiver(t *testing.T) {
	var m *Matcher
	if m.Match("anything.md", false) {
		t.Error("nil matcher should not match")
	}
	if m.Match("a/b/c", true) {
		t.Error("nil matcher should not match dir")
	}
}

func TestMatch_EmptyOrDot(t *testing.T) {
	dir := t.TempDir()
	writeIgnore(t, dir, "*.md\n")

	m, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if m.Match("", false) {
		t.Error("empty path should not match")
	}
	if m.Match(".", true) {
		t.Error("dot path should not match")
	}
}

func TestMatch_MultiplePatterns(t *testing.T) {
	dir := t.TempDir()
	writeIgnore(t, dir, "# skip session logs\n*.jsonl\n\n# skip cache dirs\ncache/\n")

	m, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if !m.Match("a/b.jsonl", false) {
		t.Error("*.jsonl should match")
	}
	if !m.Match("any/cache", true) {
		t.Error("cache/ should match")
	}
	if m.Match("notes.md", false) {
		t.Error("notes.md should not match")
	}
}
