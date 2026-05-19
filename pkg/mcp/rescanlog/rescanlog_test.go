package rescanlog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/radimsem/remindb/pkg/config"
	"github.com/radimsem/remindb/pkg/mcp/rescanstat"
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

func TestSink_AppendsOneLinePerCall(t *testing.T) {
	ws := t.TempDir()

	s, err := New(ws, 1<<20)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	snaps := []rescanstat.Snapshot{
		{RunAt: 1, Added: 2},
		{RunAt: 2, Error: "boom"},
		{RunAt: 3, PurgedFiles: []rescanstat.PurgedFile{{Path: "a.md", Nodes: 4}}},
	}
	for _, sn := range snaps {
		if err := s.Append(sn); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	lines := readLines(t, Path(ws))
	if len(lines) != len(snaps) {
		t.Fatalf("line count = %d, want %d", len(lines), len(snaps))
	}

	for i, l := range lines {
		var got rescanstat.Snapshot

		if err := json.Unmarshal([]byte(l), &got); err != nil {
			t.Fatalf("line %d not valid JSON: %v", i, err)
		}
		if got.RunAt != snaps[i].RunAt {
			t.Errorf("line %d run_at = %d, want %d", i, got.RunAt, snaps[i].RunAt)
		}
	}

	if got := lines[2]; !strings.Contains(got, `"path":"a.md"`) || !strings.Contains(got, `"nodes":4`) {
		t.Errorf("purge line missing per-file detail: %s", got)
	}
}

func TestSink_AppendsAcrossRestart(t *testing.T) {
	ws := t.TempDir()

	s1, err := New(ws, 1<<20)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s1.Append(rescanstat.Snapshot{RunAt: 1}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	// A fresh Sink against the same workspace must append, not truncate.
	s2, err := New(ws, 1<<20)
	if err != nil {
		t.Fatalf("New (restart): %v", err)
	}
	if err := s2.Append(rescanstat.Snapshot{RunAt: 2}); err != nil {
		t.Fatalf("Append (restart): %v", err)
	}

	lines := readLines(t, Path(ws))
	if len(lines) != 2 {
		t.Fatalf("line count = %d, want 2 (append survived restart)", len(lines))
	}
}

func TestSink_RotatesOnceAtCap(t *testing.T) {
	ws := t.TempDir()

	one, _ := json.Marshal(rescanstat.Snapshot{RunAt: 1})
	capBytes := int64(len(one)+1) + 1 // room for one line, second crosses the cap

	s, err := New(ws, capBytes)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := s.Append(rescanstat.Snapshot{RunAt: 1}); err != nil {
		t.Fatalf("Append 1: %v", err)
	}
	if err := s.Append(rescanstat.Snapshot{RunAt: 2}); err != nil {
		t.Fatalf("Append 2: %v", err)
	}

	rotated := Path(ws) + ".1"
	if _, err := os.Stat(rotated); err != nil {
		t.Fatalf("expected rotated file %s: %v", rotated, err)
	}

	lines := readLines(t, Path(ws))
	if len(lines) != 1 {
		t.Fatalf("active file line count = %d, want 1 after rotation", len(lines))
	}

	var got rescanstat.Snapshot
	if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
		t.Fatalf("active line invalid: %v", err)
	}
	if got.RunAt != 2 {
		t.Errorf("active file run_at = %d, want 2 (latest tick)", got.RunAt)
	}
}

func TestPath_UnderRemindbDir(t *testing.T) {
	want := filepath.Join("/ws", config.DirName, "rescan.jsonl")
	if got := Path("/ws"); got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
}
