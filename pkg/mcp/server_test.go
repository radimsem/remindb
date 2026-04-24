package mcp

import (
	"context"
	"testing"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/internal/testutil"
	"github.com/radimsem/remindb/pkg/store"
	"github.com/radimsem/remindb/pkg/temperature"
)

func newTestServer(t *testing.T, notify, cold float64) *Server {
	t.Helper()
	st := testutil.OpenTestDB(t)
	cfg := temperature.Config{
		ColdThreshold:   cold,
		NotifyThreshold: notify,
	}
	tracker := temperature.NewTracker(st, cfg, nil)
	return NewServer(st, tracker, cfg, nil)
}

func mkNode(id string, temp float64) *store.Node {
	return &store.Node{ID: id, Label: "l-" + id, SourceFile: "f.md", Temperature: temp}
}

func ids(ns []*store.Node) []string {
	out := make([]string, len(ns))
	for i, n := range ns {
		out[i] = n.ID
	}
	return out
}

func TestSelectNewNotifications_FiltersAboveNotifyThreshold(t *testing.T) {
	s := newTestServer(t, 0.05, 0.1)

	cold := []*store.Node{mkNode("warmish", 0.08), mkNode("frozen", 0.02)}

	got := s.selectNewNotifications(cold)

	if len(got) != 1 || got[0].ID != "frozen" {
		t.Fatalf("got %v, want [frozen]", ids(got))
	}
}

func TestSelectNewNotifications_DedupesAcrossTicks(t *testing.T) {
	s := newTestServer(t, 0.05, 0.1)

	first := s.selectNewNotifications([]*store.Node{mkNode("frozen", 0.02)})
	if len(first) != 1 {
		t.Fatalf("tick 1: got %d, want 1", len(first))
	}

	second := s.selectNewNotifications([]*store.Node{mkNode("frozen", 0.02)})
	if len(second) != 0 {
		t.Fatalf("tick 2: got %v, want empty", ids(second))
	}
}

func TestSelectNewNotifications_ReNotifiesAfterWarmup(t *testing.T) {
	s := newTestServer(t, 0.05, 0.1)

	if got := s.selectNewNotifications([]*store.Node{mkNode("frozen", 0.02)}); len(got) != 1 {
		t.Fatalf("tick 1: got %v, want [frozen]", ids(got))
	}

	// Node warmed above ColdThreshold → tracker excludes it → empty input evicts dedup.
	if got := s.selectNewNotifications(nil); len(got) != 0 {
		t.Fatalf("warmup tick: got %v, want empty", ids(got))
	}

	// Re-cools below NotifyThreshold → notify again.
	again := s.selectNewNotifications([]*store.Node{mkNode("frozen", 0.02)})
	if len(again) != 1 {
		t.Fatalf("re-cool tick: got %v, want [frozen]", ids(again))
	}
}

func TestSelectNewNotifications_KeepsDedupAcrossHysteresisBand(t *testing.T) {
	s := newTestServer(t, 0.05, 0.1)

	s.selectNewNotifications([]*store.Node{mkNode("frozen", 0.02)})

	// Still below ColdThreshold (0.1) but above NotifyThreshold (0.05).
	// Node stays in the cold set, so dedup entry survives.
	s.selectNewNotifications([]*store.Node{mkNode("frozen", 0.08)})

	// Drops back below NotifyThreshold without ever warming past ColdThreshold.
	// Must not re-notify.
	if got := s.selectNewNotifications([]*store.Node{mkNode("frozen", 0.02)}); len(got) != 0 {
		t.Fatalf("hysteresis band: got %v, want empty", ids(got))
	}
}

func TestNotifyColdNodes_ReachesClient(t *testing.T) {
	s := newTestServer(t, 0.05, 0.1)

	serverTransport, clientTransport := gomcp.NewInMemoryTransports()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := s.Connect(ctx, serverTransport); err != nil {
		t.Fatalf("server connect: %v", err)
	}

	received := make(chan *gomcp.LoggingMessageParams, 1)
	client := gomcp.NewClient(&gomcp.Implementation{Name: "test", Version: "0.1.0"}, &gomcp.ClientOptions{
		LoggingMessageHandler: func(_ context.Context, req *gomcp.LoggingMessageRequest) {
			received <- req.Params
		},
	})
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })

	if err := clientSession.SetLoggingLevel(ctx, &gomcp.SetLoggingLevelParams{Level: "debug"}); err != nil {
		t.Fatalf("SetLoggingLevel: %v", err)
	}

	s.NotifyColdNodes(ctx, []*store.Node{mkNode("frozen", 0.01)})

	select {
	case params := <-received:
		if params.Level != "warning" {
			t.Errorf("level = %q, want warning", params.Level)
		}
		if params.Logger != "remindb.temperature" {
			t.Errorf("logger = %q, want remindb.temperature", params.Logger)
		}
		data, ok := params.Data.(map[string]any)
		if !ok {
			t.Fatalf("data type = %T, want map[string]any", params.Data)
		}
		if data["suggested_action"] != "MemorySummarize" {
			t.Errorf("suggested_action = %v, want MemorySummarize", data["suggested_action"])
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for logging notification")
	}
}

func TestNotifyColdNodes_ClientWithoutSetLevelGetsNothing(t *testing.T) {
	s := newTestServer(t, 0.05, 0.1)

	serverTransport, clientTransport := gomcp.NewInMemoryTransports()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if _, err := s.Connect(ctx, serverTransport); err != nil {
		t.Fatalf("server connect: %v", err)
	}

	received := make(chan *gomcp.LoggingMessageParams, 1)
	client := gomcp.NewClient(&gomcp.Implementation{Name: "test", Version: "0.1.0"}, &gomcp.ClientOptions{
		LoggingMessageHandler: func(_ context.Context, req *gomcp.LoggingMessageRequest) {
			received <- req.Params
		},
	})
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })

	s.NotifyColdNodes(ctx, []*store.Node{mkNode("frozen", 0.01)})

	select {
	case params := <-received:
		t.Fatalf("unexpected notification before SetLoggingLevel: %+v", params)
	case <-time.After(200 * time.Millisecond):
	}
}
