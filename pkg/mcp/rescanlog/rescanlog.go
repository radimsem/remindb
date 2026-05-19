// Package rescanlog appends each serve source-rescan tick to an append-only .remindb/rescan.jsonl.
package rescanlog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/radimsem/remindb/pkg/config"
	"github.com/radimsem/remindb/pkg/mcp/rescanstat"
)

const fileName = "rescan.jsonl"

type Sink struct {
	path        string
	maxFileSize int64

	mu sync.Mutex
}

// New ensures <workspace>/.remindb exists and returns a sink bounded by maxFileSize bytes.
func New(workspace string, maxFileSize int64) (*Sink, error) {
	dir := filepath.Join(workspace, config.DirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create: rescan log dir: %w", err)
	}

	return &Sink{path: filepath.Join(dir, fileName), maxFileSize: maxFileSize}, nil
}

// Path returns the rescan-history file under workspace.
func Path(workspace string) string {
	return filepath.Join(workspace, config.DirName, fileName)
}

// Append writes one tick snapshot as a JSON line, rotating once when the cap is reached.
func (s *Sink) Append(snap rescanstat.Snapshot) error {
	line, err := json.Marshal(snap)
	if err != nil {
		return fmt.Errorf("failed to marshal: rescan snapshot: %w", err)
	}
	line = append(line, '\n')

	s.mu.Lock()
	defer s.mu.Unlock()

	// Single-generation rotation: the prior .1 is intentionally discarded.
	fi, statErr := os.Stat(s.path)
	overCap := statErr == nil && fi.Size() > 0 &&
		int64(len(line)) <= s.maxFileSize &&
		fi.Size()+int64(len(line)) > s.maxFileSize

	if overCap {
		if err := os.Rename(s.path, s.path+".1"); err != nil {
			return fmt.Errorf("failed to rotate: rescan log %s: %w", s.path, err)
		}
	}

	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open: rescan log %s: %w", s.path, err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Write(line); err != nil {
		return fmt.Errorf("failed to write: rescan log %s: %w", s.path, err)
	}
	return nil
}
