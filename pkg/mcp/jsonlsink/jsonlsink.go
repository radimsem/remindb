// Package jsonlsink is the shared append-only JSONL file primitive.
package jsonlsink

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type Sink struct {
	dir         string
	maxFileSize int64

	mu sync.Mutex
}

func New(dir string, maxFileSize int64) (*Sink, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create: jsonl sink dir %s: %w", dir, err)
	}

	return &Sink{dir: dir, maxFileSize: maxFileSize}, nil
}

func (s *Sink) Dir() string {
	return s.dir
}

func (s *Sink) Append(name string, line []byte) error {
	path := filepath.Join(s.dir, name)

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.rotateIfOverCap(path, len(line)); err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open: jsonl sink %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Write(line); err != nil {
		return fmt.Errorf("failed to write: jsonl sink %s: %w", path, err)
	}
	return nil
}

// rotateIfOverCap moves a non-empty path to path+".1" (discarding any prior
// .1) when appending lineLen more bytes would cross the cap.
func (s *Sink) rotateIfOverCap(path string, lineLen int) error {
	if s.maxFileSize <= 0 {
		return nil
	}

	fi, err := os.Stat(path)
	if err != nil || fi.Size() == 0 {
		return nil
	}
	if fi.Size()+int64(lineLen) <= s.maxFileSize {
		return nil
	}

	if err := os.Rename(path, path+".1"); err != nil {
		return fmt.Errorf("failed to rotate: jsonl sink %s: %w", path, err)
	}
	return nil
}
