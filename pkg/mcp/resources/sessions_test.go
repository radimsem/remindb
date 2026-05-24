package resources

import (
	"context"
	"encoding/json"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/pkg/mcp/session"
)

func readSessions(t *testing.T, d *Deps) (string, sessionsEnvelope) {
	t.Helper()

	res, err := d.HandleSessions(context.Background(), nil)
	if err != nil {
		t.Fatalf("HandleSessions: %v", err)
	}
	if len(res.Contents) != 1 {
		t.Fatalf("contents: got %d, want 1", len(res.Contents))
	}
	if res.Contents[0].URI != SessionsURI || res.Contents[0].MIMEType != mimeJSON {
		t.Fatalf("envelope: got %q/%q", res.Contents[0].URI, res.Contents[0].MIMEType)
	}

	var env sessionsEnvelope
	if err := json.Unmarshal([]byte(res.Contents[0].Text), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return res.Contents[0].Text, env
}

func TestHandleSessions_NilDepsSafe(t *testing.T) {
	body, env := readSessions(t, &Deps{})

	if body != `{"db_path":"","sessions":[]}` {
		t.Errorf("nil-deps body: got %s, want empty envelope", body)
	}
	if env.Sessions == nil {
		t.Error("sessions must marshal as [] not null")
	}
}

func TestHandleSessions_EmptyRegistry(t *testing.T) {
	srv := gomcp.NewServer(&gomcp.Implementation{Name: "t", Version: "0"}, nil)
	reg := session.NewRegistry(srv)

	_, env := readSessions(t, &Deps{Sessions: reg})

	if len(env.Sessions) != 0 {
		t.Errorf("no client connected: got %d sessions, want 0", len(env.Sessions))
	}
}
