package ignore

import (
	"os"
	"path/filepath"
	"strings"
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

func TestLoad_UnsupportedPattern_Negation(t *testing.T) {
	dir := t.TempDir()
	writeIgnore(t, dir, "*.md\n!important.md\n")

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for negation pattern")
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Errorf("error should reference line 2, got: %v", err)
	}
}

func TestLoad_UnsupportedPattern_CharRange(t *testing.T) {
	dir := t.TempDir()
	writeIgnore(t, dir, "file[abc].md\n")

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for char range")
	}
	if !strings.Contains(err.Error(), "line 1") {
		t.Errorf("error should reference line 1, got: %v", err)
	}
}

func TestLoad_UnsupportedPattern_Escape(t *testing.T) {
	dir := t.TempDir()
	writeIgnore(t, dir, "foo\\bar\n")

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for escape sequence")
	}
}

func TestLoad_UnsupportedPattern_Question(t *testing.T) {
	dir := t.TempDir()
	writeIgnore(t, dir, "fo?.md\n")

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for ? wildcard")
	}
}

func TestLoad_UnsupportedPattern_LeadingSlash(t *testing.T) {
	dir := t.TempDir()
	writeIgnore(t, dir, "/anchored.md\n")

	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error for leading slash")
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
