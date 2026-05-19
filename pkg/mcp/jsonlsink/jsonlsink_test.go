package jsonlsink

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readLines(t *testing.T, path string) []string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	trimmed := strings.TrimRight(string(data), "\n")
	if trimmed == "" {
		return nil
	}

	return strings.Split(trimmed, "\n")
}

func TestNew_CreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b")

	s, err := New(dir, 1<<20)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if s.Dir() != dir {
		t.Errorf("Dir() = %q, want %q", s.Dir(), dir)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Errorf("dir not created: %v", err)
	}
}

func TestAppend_OneLinePerCallSurvivesNewSink(t *testing.T) {
	dir := t.TempDir()

	s, _ := New(dir, 1<<20)
	if err := s.Append("x.jsonl", []byte("a\n")); err != nil {
		t.Fatalf("Append: %v", err)
	}

	// A fresh sink against the same dir must append, not truncate.
	s2, _ := New(dir, 1<<20)
	if err := s2.Append("x.jsonl", []byte("b\n")); err != nil {
		t.Fatalf("Append (re-New): %v", err)
	}

	if lines := readLines(t, filepath.Join(dir, "x.jsonl")); len(lines) != 2 {
		t.Fatalf("lines = %d, want 2 (append survived re-New)", len(lines))
	}
}

func TestAppend_RotatesOnceAtCap(t *testing.T) {
	dir := t.TempDir()
	line := []byte("0123456789\n") // 11 bytes

	s, _ := New(dir, 25) // two lines fit (22), third crosses
	for range 3 {
		if err := s.Append("r.jsonl", line); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	path := filepath.Join(dir, "r.jsonl")
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("expected rotated file: %v", err)
	}

	if lines := readLines(t, path); len(lines) != 1 {
		t.Fatalf("active lines = %d, want 1 after rotation", len(lines))
	}
}

func TestAppend_LoneOversizeLineRotatesNotGrows(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir, 8) // cap smaller than a single record — the old footgun

	line := []byte("this single line is far over the cap\n")
	for range 5 {
		if err := s.Append("big.jsonl", line); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	path := filepath.Join(dir, "big.jsonl")
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("active file missing: %v", err)
	}

	// Fixed: a too-big line rotates the prior file away and lands alone,
	// so the active file holds exactly one line — not five accumulated.
	if fi.Size() != int64(len(line)) {
		t.Errorf("active size = %d, want %d (one line; growth bounded)", fi.Size(), len(line))
	}
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Errorf("expected rotated .1 to exist: %v", err)
	}
}

func TestAppend_NonPositiveCapDisablesRotation(t *testing.T) {
	for _, cap := range []int64{0, -1} {
		dir := t.TempDir()
		s, _ := New(dir, cap)

		for range 10 {
			if err := s.Append("n.jsonl", []byte("line\n")); err != nil {
				t.Fatalf("Append (cap=%d): %v", cap, err)
			}
		}

		path := filepath.Join(dir, "n.jsonl")
		if _, err := os.Stat(path + ".1"); !os.IsNotExist(err) {
			t.Errorf("cap=%d: rotation should be disabled, .1 exists", cap)
		}

		if lines := readLines(t, path); len(lines) != 10 {
			t.Errorf("cap=%d: lines = %d, want 10 (no rotation)", cap, len(lines))
		}
	}
}

func TestAppend_ConcurrentNoCorruption(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir, 1<<20)

	done := make(chan struct{}, 2)
	for range 2 {
		go func() {
			for range 50 {
				_ = s.Append("c.jsonl", []byte("0123456789\n"))
			}
			done <- struct{}{}
		}()
	}
	<-done
	<-done

	lines := readLines(t, filepath.Join(dir, "c.jsonl"))
	if len(lines) != 100 {
		t.Fatalf("lines = %d, want 100 (no interleaved writes)", len(lines))
	}

	for i, l := range lines {
		if l != "0123456789" {
			t.Fatalf("line %d corrupted: %q", i, l)
		}
	}
}
