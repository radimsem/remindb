package resources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/pkg/mcp/sessionlog"
)

const (
	SessionsLogsURI         = "remindb://sessions/logs"
	SessionsLogByIDTemplate = "remindb://sessions/logs/{id}"

	logExt     = ".log"
	rotatedExt = ".log.1"
)

type sessionLogInfo struct {
	SessionID  string `json:"session_id"`
	SizeBytes  int64  `json:"size_bytes"`
	Rotated    bool   `json:"rotated"`
	ModifiedAt int64  `json:"modified_at"`
}

type sessionsLogsEnvelope struct {
	DBPath string           `json:"db_path"`
	Logs   []sessionLogInfo `json:"logs"`
}

type sessionLogEnvelope struct {
	SessionID string              `json:"session_id"`
	Entries   []sessionlog.Record `json:"entries"`
}

func (d *Deps) HandleSessionsLogs(ctx context.Context, _ *gomcp.ReadResourceRequest) (*gomcp.ReadResourceResult, error) {
	env := sessionsLogsEnvelope{Logs: []sessionLogInfo{}}
	if d.Store != nil {
		env.DBPath = d.Store.Path
	}

	if d.SessionLogDir != "" {
		infos, err := scanSessionLogs(d.SessionLogDir)
		if err != nil {
			return nil, err
		}
		env.Logs = infos
	}

	body, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal: sessions logs: %w", err)
	}

	return &gomcp.ReadResourceResult{
		Contents: []*gomcp.ResourceContents{{
			URI:      SessionsLogsURI,
			MIMEType: mimeJSON,
			Text:     string(body),
		}},
	}, nil
}

func (d *Deps) HandleSessionsLogByID(ctx context.Context, req *gomcp.ReadResourceRequest) (*gomcp.ReadResourceResult, error) {
	u, err := url.Parse(req.Params.URI)
	if err != nil {
		return nil, fmt.Errorf("failed to parse: sessions log uri: %w", err)
	}

	id := strings.TrimPrefix(u.Path, "/logs/")
	if id == "" || strings.Contains(id, "/") {
		return nil, fmt.Errorf("sessions log uri missing session id")
	}

	if d.SessionLogDir == "" {
		return nil, fmt.Errorf("unknown session %q", id)
	}

	path := filepath.Join(d.SessionLogDir, sessionlog.Slug(id)+logExt)

	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("unknown session %q", id)
		}
		return nil, fmt.Errorf("failed to open: session log: %w", err)
	}
	defer func() { _ = f.Close() }()

	entries, err := sessionlog.ParseLog(f)
	if err != nil {
		return nil, fmt.Errorf("failed to read: session log: %w", err)
	}
	if entries == nil {
		entries = []sessionlog.Record{}
	}

	env := sessionLogEnvelope{SessionID: sessionlog.Slug(id), Entries: entries}
	body, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal: sessions log: %w", err)
	}

	return &gomcp.ReadResourceResult{
		Contents: []*gomcp.ResourceContents{{
			URI:      req.Params.URI,
			MIMEType: mimeJSON,
			Text:     string(body),
		}},
	}, nil
}

// scanSessionLogs reports one entry per active logfile.
func scanSessionLogs(dir string) ([]sessionLogInfo, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return []sessionLogInfo{}, nil
		}
		return nil, fmt.Errorf("failed to read: session logs dir: %w", err)
	}

	rotated := make(map[string]bool, len(ents))
	for _, e := range ents {
		if name := e.Name(); strings.HasSuffix(name, rotatedExt) {
			rotated[strings.TrimSuffix(name, rotatedExt)] = true
		}
	}

	infos := make([]sessionLogInfo, 0, len(ents))
	for _, e := range ents {
		name := e.Name()
		// A .log.1 tail fails the .log suffix check, so it's excluded here.
		if e.IsDir() || !strings.HasSuffix(name, logExt) {
			continue
		}

		fi, err := e.Info()
		if err != nil {
			return nil, fmt.Errorf("failed to stat: session log: %w", err)
		}

		stem := strings.TrimSuffix(name, logExt)
		infos = append(infos, sessionLogInfo{
			SessionID:  stem,
			SizeBytes:  fi.Size(),
			Rotated:    rotated[stem],
			ModifiedAt: fi.ModTime().Unix(),
		})
	}

	return infos, nil
}
