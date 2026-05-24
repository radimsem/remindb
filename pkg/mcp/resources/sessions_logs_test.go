package resources

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/pkg/mcp/sessionlog"
)

func writeLog(t *testing.T, dir, name, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func byID(t *testing.T, d *Deps, uri string) (*gomcp.ReadResourceResult, error) {
	t.Helper()

	return d.HandleSessionsLogByID(context.Background(),
		&gomcp.ReadResourceRequest{Params: &gomcp.ReadResourceParams{URI: uri}})
}

func TestHandleSessionsLogs_DisabledEmpty(t *testing.T) {
	res, err := (&Deps{}).HandleSessionsLogs(context.Background(), nil)
	if err != nil {
		t.Fatalf("HandleSessionsLogs: %v", err)
	}

	if got := res.Contents[0].Text; got != `{"db_path":"","logs":[]}` {
		t.Errorf("disabled body = %s, want empty envelope", got)
	}
}

func TestHandleSessionsLogs_MissingDirEmpty(t *testing.T) {
	d := &Deps{SessionLogDir: filepath.Join(t.TempDir(), "never-created")}

	res, err := d.HandleSessionsLogs(context.Background(), nil)
	if err != nil {
		t.Fatalf("HandleSessionsLogs: %v", err)
	}

	var env sessionsLogsEnvelope
	if err := json.Unmarshal([]byte(res.Contents[0].Text), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if env.Logs == nil || len(env.Logs) != 0 {
		t.Errorf("missing dir: logs = %v, want []", env.Logs)
	}
}

func TestHandleSessionsLogs_IndexReportsRotation(t *testing.T) {
	dir := t.TempDir()

	writeLog(t, dir, "sess-a.log", `{"time":"2026-05-19T00:00:00Z","level":"INFO","msg":"x"}`+"\n")
	writeLog(t, dir, "sess-b.log", "two\nlines\n")
	writeLog(t, dir, "sess-b.log.1", "rotated tail\n")
	writeLog(t, dir, "not-a-log.txt", "ignored")

	if err := os.Mkdir(filepath.Join(dir, "sub.log"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	res, err := (&Deps{SessionLogDir: dir}).HandleSessionsLogs(context.Background(), nil)
	if err != nil {
		t.Fatalf("HandleSessionsLogs: %v", err)
	}

	var env sessionsLogsEnvelope
	if err := json.Unmarshal([]byte(res.Contents[0].Text), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(env.Logs) != 2 {
		t.Fatalf("logs = %+v, want exactly sess-a and sess-b", env.Logs)
	}

	byStem := map[string]sessionLogInfo{}
	for _, l := range env.Logs {
		byStem[l.SessionID] = l
	}

	a, b := byStem["sess-a"], byStem["sess-b"]
	if a.Rotated {
		t.Errorf("sess-a rotated = true, want false")
	}
	if !b.Rotated {
		t.Errorf("sess-b rotated = false, want true (.log.1 present)")
	}

	if b.SizeBytes == 0 || b.ModifiedAt == 0 {
		t.Errorf("sess-b size/mtime = %d/%d, want non-zero", b.SizeBytes, b.ModifiedAt)
	}
}

func TestHandleSessionsLogByID_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	rec := sessionlog.Record{
		Level: "DEBUG", Msg: "mcp call",
		Fields: map[string]any{"tool": "MemoryWrite", "payload_bytes": float64(42)},
	}

	line, _ := json.Marshal(rec)
	writeLog(t, dir, sessionlog.Slug("sess-xy")+".log", string(line)+"\n\n")

	res, err := byID(t, &Deps{SessionLogDir: dir}, "remindb://sessions/logs/sess-xy")
	if err != nil {
		t.Fatalf("HandleSessionsLogByID: %v", err)
	}

	var env sessionLogEnvelope
	if err := json.Unmarshal([]byte(res.Contents[0].Text), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.SessionID != "sess-xy" || len(env.Entries) != 1 {
		t.Fatalf("env = %+v, want one entry for sess-xy", env)
	}

	e := env.Entries[0]
	if e.Msg != "mcp call" || e.Fields["tool"] != "MemoryWrite" || e.Fields["payload_bytes"] != float64(42) {
		t.Errorf("entry = %+v, want the round-tripped tool trace", e)
	}
}

func TestHandleSessionsLogByID_UnknownIsCleanError(t *testing.T) {
	dir := t.TempDir()

	for _, tc := range []struct {
		name string
		deps *Deps
		uri  string
	}{
		{"disabled", &Deps{}, "remindb://sessions/logs/whatever"},
		{"no file", &Deps{SessionLogDir: dir}, "remindb://sessions/logs/ghost"},
		{"missing id", &Deps{SessionLogDir: dir}, "remindb://sessions/logs/"},
	} {
		if _, err := byID(t, tc.deps, tc.uri); err == nil {
			t.Errorf("%s: expected an error, got nil", tc.name)
		}
	}
}
