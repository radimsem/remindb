package resources

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strconv"
	"testing"

	"github.com/radimsem/remindb/pkg/logbuf"
)

func readLogs(t *testing.T, d *Deps) logsEnvelope {
	t.Helper()

	res, err := d.HandleLogs(context.Background(), nil)
	if err != nil {
		t.Fatalf("HandleLogs: %v", err)
	}

	if len(res.Contents) != 1 {
		t.Fatalf("contents: got %d, want 1", len(res.Contents))
	}
	if res.Contents[0].URI != LogsURI || res.Contents[0].MIMEType != mimeJSON {
		t.Fatalf("envelope: got %q/%q", res.Contents[0].URI, res.Contents[0].MIMEType)
	}

	var env logsEnvelope
	if err := json.Unmarshal([]byte(res.Contents[0].Text), &env); err != nil {
		t.Fatalf("unmarshal resource body: %v", err)
	}
	return env
}

func TestHandleLogs_OrderingAndOverflow(t *testing.T) {
	buf := logbuf.NewBuffer(3)
	log := slog.New(logbuf.NewHandler(slog.NewTextHandler(io.Discard, nil), buf))
	for i := range 5 {
		log.Info("m" + strconv.Itoa(i))
	}

	env := readLogs(t, &Deps{LogBuffer: buf})
	if len(env.Records) != 3 {
		t.Fatalf("records: got %d, want 3", len(env.Records))
	}

	if env.Records[0].Msg != "m2" || env.Records[2].Msg != "m4" {
		t.Errorf("ordering: got %q..%q, want m2..m4 (newest last)", env.Records[0].Msg, env.Records[2].Msg)
	}
	if env.Dropped != 2 {
		t.Errorf("dropped: got %d, want 2", env.Dropped)
	}
}

func TestHandleLogs_NilBufferSafe(t *testing.T) {
	res, err := (&Deps{}).HandleLogs(context.Background(), nil)
	if err != nil {
		t.Fatalf("HandleLogs: %v", err)
	}

	if got := res.Contents[0].Text; got != `{"records":[],"dropped":0}` {
		t.Errorf("nil-buffer body: got %s, want empty envelope", got)
	}
}
