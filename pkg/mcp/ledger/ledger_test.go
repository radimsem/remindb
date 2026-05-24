package ledger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/radimsem/remindb/pkg/config"
)

func newLedger(t *testing.T) (*Ledger, string) {
	t.Helper()

	ws := t.TempDir()
	l, err := New(ws, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	return l, ws
}

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"claude-code":           "claude-code",
		"Claude Code":           "claude-code",
		"  weird//name  ":       "weird-name",
		"":                      "client",
		"???":                   "client",
		strings.Repeat("a", 60): strings.Repeat("a", 40),
	}
	for in, want := range cases {
		if got := slug(in); got != want {
			t.Errorf("slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFileNameShape(t *testing.T) {
	c := ClientMeta{Name: "Claude Code", Version: "1.0", Protocol: "2025-06-18"}
	name := fileName(c, "stdio")

	if !strings.HasPrefix(name, "claude-code-") || !strings.HasSuffix(name, ".jsonl") {
		t.Fatalf("filename shape: %q", name)
	}
	if h := Hash(c, "stdio"); !strings.Contains(name, h) {
		t.Errorf("filename %q missing hash %q", name, h)
	}

	// Identity is stable across reconnects: a fresh but equal struct hashes same.
	same := ClientMeta{Name: "Claude Code", Version: "1.0", Protocol: "2025-06-18"}
	if Hash(c, "stdio") != Hash(same, "stdio") {
		t.Error("hash not deterministic for equal client metadata")
	}
	if Hash(c, "stdio") == Hash(c, "http") {
		t.Error("transport must participate in identity")
	}
}

func TestAppendAndAggregate(t *testing.T) {
	l, _ := newLedger(t)
	c := ClientMeta{Name: "claude-code", Version: "1", Protocol: "p"}

	// One open session, two checkpoints — collapse keeps the last.
	must(t, l.Append(Record{SessionID: "s1", Client: c, Transport: "stdio", ConnectedAt: 100, LastSeen: 110, ToolCalls: 2}))
	must(t, l.Append(Record{SessionID: "s1", Client: c, Transport: "stdio", ConnectedAt: 100, LastSeen: 160, ToolCalls: 5}))
	// A second, cleanly-closed session.
	must(t, l.Append(Record{SessionID: "s2", Client: c, Transport: "stdio", ConnectedAt: 200, LastSeen: 250, DisconnectedAt: 250, ToolCalls: 3}))

	clients, err := l.Clients()
	if err != nil {
		t.Fatalf("Clients: %v", err)
	}
	if len(clients) != 1 {
		t.Fatalf("clients: got %d, want 1", len(clients))
	}

	cl := clients[0]
	if cl.Sessions != 2 {
		t.Errorf("sessions: got %d, want 2", cl.Sessions)
	}
	if cl.ToolCalls != 8 {
		t.Errorf("tool_calls: got %d, want 8 (5 collapsed + 3)", cl.ToolCalls)
	}
	if cl.LifetimeSeconds != 110 { // s1: 160-100=60 (last wins), s2: 250-200=50
		t.Errorf("lifetime: got %d, want 110", cl.LifetimeSeconds)
	}
	if cl.LastDisconnect != 250 {
		t.Errorf("last_disconnect: got %d, want 250", cl.LastDisconnect)
	}
	if cl.Hash != Hash(c, "stdio") {
		t.Errorf("hash: got %q, want %q", cl.Hash, Hash(c, "stdio"))
	}

	byHash, err := l.Client(cl.Hash)
	if err != nil {
		t.Fatalf("Client(%q): %v", cl.Hash, err)
	}

	if byHash.ToolCalls != cl.ToolCalls || byHash.Sessions != cl.Sessions {
		t.Errorf("by-hash mismatch: %+v vs %+v", byHash, cl)
	}
	if _, err := l.Client("nope"); err == nil {
		t.Error("Client(unknown) should error")
	}
}

// A crashed process leaves an open line (no disconnected_at).
func TestCrashRecoveryNoDoubleCount(t *testing.T) {
	l, _ := newLedger(t)
	c := ClientMeta{Name: "agent", Version: "1", Protocol: "p"}

	must(t, l.Append(Record{SessionID: "crashed", Client: c, Transport: "stdio", ConnectedAt: 10, LastSeen: 40, ToolCalls: 4}))
	must(t, l.Append(Record{SessionID: "reconnect", Client: c, Transport: "stdio", ConnectedAt: 50, LastSeen: 90, DisconnectedAt: 90, ToolCalls: 1}))

	clients, err := l.Clients()
	if err != nil {
		t.Fatalf("Clients: %v", err)
	}

	cl := clients[0]
	if cl.Sessions != 2 {
		t.Fatalf("sessions: got %d, want 2", cl.Sessions)
	}
	if cl.LifetimeSeconds != 70 { // crashed: 40-10=30, reconnect: 90-50=40
		t.Errorf("lifetime: got %d, want 70", cl.LifetimeSeconds)
	}
	if cl.ToolCalls != 5 {
		t.Errorf("tool_calls: got %d, want 5", cl.ToolCalls)
	}
}

func TestCompactionShrinksFile(t *testing.T) {
	l, ws := newLedger(t)
	c := ClientMeta{Name: "claude-code", Version: "1", Protocol: "p"}

	for i := range 20 {
		must(t, l.Append(Record{SessionID: "s1", Client: c, Transport: "stdio", ConnectedAt: 1, LastSeen: int64(1 + i)}))
	}

	dir := filepath.Join(ws, config.DirName, subDir)
	path := filepath.Join(dir, fileName(c, "stdio"))
	if lines := countLines(t, path); lines != 20 {
		t.Fatalf("pre-compaction lines: got %d, want 20", lines)
	}

	if _, err := New(ws, nil); err != nil { // re-open triggers compaction
		t.Fatalf("re-New: %v", err)
	}
	if lines := countLines(t, path); lines != 1 {
		t.Fatalf("post-compaction lines: got %d, want 1", lines)
	}

	// The surviving line must be the newest checkpoint, not the oldest.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(data), `"last_seen":20`) {
		t.Errorf("compaction lost the latest checkpoint: %s", data)
	}
}

func TestConcurrentProcessesNoCorruption(t *testing.T) {
	_, ws := newLedger(t)
	a, _ := New(ws, nil)
	b, _ := New(ws, nil)
	c := ClientMeta{Name: "x", Version: "1", Protocol: "p"}

	done := make(chan struct{}, 2)
	for _, l := range []*Ledger{a, b} {
		go func(l *Ledger) {
			for i := range 50 {
				_ = l.Append(Record{SessionID: "s", Client: c, Transport: "stdio", ConnectedAt: 1, LastSeen: int64(i)})
			}
			done <- struct{}{}
		}(l)
	}

	<-done
	<-done

	if _, err := a.Clients(); err != nil {
		t.Fatalf("ledger corrupted after concurrent appends: %v", err)
	}
}

func TestNoPayloadFields(t *testing.T) {
	// The on-disk shape must never carry node bodies, payloads, or summaries.
	r := Record{
		SessionID: "s", Client: ClientMeta{Name: "n"}, Transport: "stdio",
		ConnectedAt: 1, LastSeen: 2, DisconnectedAt: 3, ToolCalls: 4,
	}

	line, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	for _, bad := range []string{"payload", "summary", "content", "body"} {
		if strings.Contains(string(line), bad) {
			t.Errorf("Record JSON exposes forbidden key %q: %s", bad, line)
		}
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
}

func countLines(t *testing.T, path string) int {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	return strings.Count(strings.TrimRight(string(data), "\n"), "\n") + 1
}
