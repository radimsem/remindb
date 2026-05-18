package resources

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/radimsem/remindb/pkg/mcp/rescanstat"
)

func readRescan(t *testing.T, d *Deps) (string, rescanEnvelope) {
	t.Helper()

	res, err := d.HandleRescan(context.Background(), nil)
	if err != nil {
		t.Fatalf("HandleRescan: %v", err)
	}

	if len(res.Contents) != 1 {
		t.Fatalf("contents: got %d, want 1", len(res.Contents))
	}
	if res.Contents[0].URI != RescanURI || res.Contents[0].MIMEType != mimeJSON {
		t.Fatalf("envelope: got %q/%q", res.Contents[0].URI, res.Contents[0].MIMEType)
	}

	var env rescanEnvelope
	if err := json.Unmarshal([]byte(res.Contents[0].Text), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return res.Contents[0].Text, env
}

func TestHandleRescan_NilDepsSafe(t *testing.T) {
	body, env := readRescan(t, &Deps{})

	if body != `{"interval_s":0,"last_meta":{"run_at":0,"error":"","added":0,"modified":0,"removed":0,"purged_files":[]}}` {
		t.Errorf("nil-deps body: got %s", body)
	}
	if env.LastMeta.PurgedFiles == nil {
		t.Error("purged_files must marshal as [] not null")
	}
}

func TestHandleRescan_ProjectsStatus(t *testing.T) {
	status := rescanstat.New()
	status.Set(30, rescanstat.Snapshot{
		RunAt:       1716000000,
		Added:       2,
		Modified:    1,
		Removed:     0,
		PurgedFiles: []rescanstat.PurgedFile{{Path: "notes/old.md", Nodes: 4}},
	})

	_, env := readRescan(t, &Deps{RescanStatus: status})

	if env.IntervalS != 30 {
		t.Errorf("interval_s = %d, want 30", env.IntervalS)
	}
	if env.LastMeta.RunAt != 1716000000 || env.LastMeta.Added != 2 || env.LastMeta.Modified != 1 {
		t.Errorf("last_meta mismatch: %+v", env.LastMeta)
	}
	if len(env.LastMeta.PurgedFiles) != 1 || env.LastMeta.PurgedFiles[0] != (rescanstat.PurgedFile{Path: "notes/old.md", Nodes: 4}) {
		t.Errorf("purged_files mismatch: %+v", env.LastMeta.PurgedFiles)
	}
}
