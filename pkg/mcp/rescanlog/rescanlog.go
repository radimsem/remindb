// Package rescanlog appends each serve source-rescan tick to an append-only .remindb/rescan.jsonl.
package rescanlog

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/radimsem/remindb/pkg/config"
	"github.com/radimsem/remindb/pkg/mcp/jsonlsink"
	"github.com/radimsem/remindb/pkg/mcp/rescanstat"
)

const fileName = "rescan.jsonl"

type Sink struct {
	inner *jsonlsink.Sink
}

// New ensures <workspace>/.remindb exists and returns a sink bounded by maxFileSize bytes.
func New(workspace string, maxFileSize int64) (*Sink, error) {
	dir := filepath.Join(workspace, config.DirName)
	inner, err := jsonlsink.New(dir, maxFileSize)
	if err != nil {
		return nil, err
	}

	return &Sink{inner: inner}, nil
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

	return s.inner.Append(fileName, line)
}
