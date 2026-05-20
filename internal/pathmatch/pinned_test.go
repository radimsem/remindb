package pathmatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/radimsem/remindb/pkg/config"
)

func writePinned(t *testing.T, dir, content string) {
	t.Helper()

	stateDir := filepath.Join(dir, config.DirName)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(stateDir, PinnedFileName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadPinned_MissingFile(t *testing.T) {
	m, err := LoadPinned(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if m != nil {
		t.Error("expected nil matcher for missing file")
	}
}

func TestLoadPinned_BasicMatchAndNegation(t *testing.T) {
	dir := t.TempDir()
	writePinned(t, dir, "README.md\nsrc/api/\n**/CONTEXT.md\n!src/api/deprecated.json\n")

	m, err := LoadPinned(dir)
	if err != nil {
		t.Fatalf("LoadPinned: %v", err)
	}

	cases := []struct {
		path  string
		isDir bool
		want  bool
	}{
		{"README.md", false, true},
		{"src/api", true, true},
		{"src/api/handlers.go", false, false},
		{"docs/CONTEXT.md", false, true},
		{"pkg/x/CONTEXT.md", false, true},
		{"src/api/deprecated.json", false, false},
	}
	for _, tt := range cases {
		got := m.Match(tt.path, tt.isDir)

		if got != tt.want {
			t.Errorf("Match(%q, isDir=%v) = %v, want %v", tt.path, tt.isDir, got, tt.want)
		}
	}
}

func TestLoadPinned_MalformedPattern_CitesPinnedPath(t *testing.T) {
	dir := t.TempDir()
	writePinned(t, dir, "a//b\n")

	_, err := LoadPinned(dir)
	if err == nil {
		t.Fatal("expected error for consecutive slashes")
	}
	if !strings.Contains(err.Error(), "consecutive slashes") {
		t.Errorf("error should describe the pattern problem, got: %v", err)
	}
}

func TestLoadPinned_ReadErrorCitesPinnedPath(t *testing.T) {
	dir := t.TempDir()

	// Make <dir>/.remindb/pinned a directory so os.Open succeeds but reads fail.
	stateDir := filepath.Join(dir, config.DirName, PinnedFileName)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := LoadPinned(dir)
	if err == nil {
		t.Fatal("expected error when pinned path is a directory")
	}
	if !strings.Contains(err.Error(), PinnedPath) {
		t.Errorf("error should mention %s, got: %v", PinnedPath, err)
	}
}
