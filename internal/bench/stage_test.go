package bench

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/radimsem/remindb/internal/ignore"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	full := filepath.Join(dir, name)

	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCopySourceTree_RespectsIgnore(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	writeFile(t, src, "kept.md", "# Kept\n")
	writeFile(t, src, "session.jsonl", `{"event":"chat"}`)
	writeFile(t, src, "sessions/log.json", `{"id":1}`)
	writeFile(t, src, ignore.FileName, "*.jsonl\nsessions/\n")

	matcher, err := ignore.Load(src)
	if err != nil {
		t.Fatalf("ignore.Load: %v", err)
	}

	if err := copySourceTree(src, dst, matcher); err != nil {
		t.Fatalf("copySourceTree: %v", err)
	}

	excluded := []string{"session.jsonl", "sessions/log.json", ignore.FileName}
	for _, rel := range excluded {
		if _, err := os.Stat(filepath.Join(dst, rel)); !os.IsNotExist(err) {
			t.Errorf("expected %s to be excluded from copy, but it exists (err=%v)", rel, err)
		}
	}

	if _, err := os.Stat(filepath.Join(dst, "kept.md")); err != nil {
		t.Errorf("expected kept.md to be copied: %v", err)
	}
}

func TestCopySourceTree_NilMatcher(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	writeFile(t, src, "a.md", "# A\n")
	writeFile(t, src, "b.json", `{"k":"v"}`)

	if err := copySourceTree(src, dst, nil); err != nil {
		t.Fatalf("copySourceTree: %v", err)
	}

	for _, rel := range []string{"a.md", "b.json"} {
		if _, err := os.Stat(filepath.Join(dst, rel)); err != nil {
			t.Errorf("expected %s in copy: %v", rel, err)
		}
	}
}
