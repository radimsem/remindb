package session

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/radimsem/remindb/internal/contentid"
)

func TestSessionID_PassthroughAndFallback(t *testing.T) {
	if got := sessionID("abc123", "stdio", 42); got != "abc123" {
		t.Errorf("non-empty id: got %q, want passthrough", got)
	}

	want := contentid.IdentifyPayload("session", "stdio0")
	got := sessionID("", "stdio", 0)

	if got != want {
		t.Errorf("empty id fallback: got %q, want %q", got, want)
	}
	if sessionID("", "stdio", 0) != got {
		t.Error("fallback not deterministic for the same inputs")
	}
}

func TestToInfo_HttpCarriesListen(t *testing.T) {
	r := &Registry{transport: transportHttp, listen: "127.0.0.1:7474"}
	c := ClientMeta{Name: "claude-code", Title: "Claude Code", Version: "1.2.0", Protocol: "2025-06-18"}
	info := r.toInfo("s1", &meta{connectedAt: 5, lastActivity: 6, toolCalls: 3}, c)

	if info.Listen != "127.0.0.1:7474" {
		t.Errorf("listen: got %q, want 127.0.0.1:7474", info.Listen)
	}
	if info.ConnectedAt != 5 || info.LastActivity != 6 || info.CountToolCalls != 3 {
		t.Errorf("fields: got %+v", info)
	}
	if info.Client != c {
		t.Errorf("client metadata: got %+v, want %+v", info.Client, c)
	}
}

func TestToInfo_StdioOmitsListenAndEmptyTitle(t *testing.T) {
	r := &Registry{transport: "stdio", listen: ""}
	info := r.toInfo("s1", &meta{connectedAt: 1, lastActivity: 1}, ClientMeta{Name: "agent"})

	body, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if strings.Contains(string(body), "listen") {
		t.Errorf("stdio session must omit listen, got %s", body)
	}
	if strings.Contains(string(body), `"title"`) {
		t.Errorf("empty title must be omitted, got %s", body)
	}
	if !strings.Contains(string(body), `"client_meta"`) {
		t.Errorf("client_meta object must always be present, got %s", body)
	}
}
