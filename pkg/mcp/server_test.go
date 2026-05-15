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
	cfg := temperature.DefaultConfig()
	cfg.ColdThreshold = cold
	cfg.NotifyThreshold = notify
	cfg.TickInterval = time.Minute

	tracker, err := temperature.NewTracker(st, cfg, nil)
	if err != nil {
		t.Fatalf("NewTracker: %v", err)
	}

	srv, err := NewServer(st, tracker, cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return srv
}

func mkNode(id string, temp float64) *store.Node {
	return &store.Node{ID: id, Label: "l-" + id, SourceFile: "f.md", Temperature: temp}
}

func TestNotifyColdNodes_FiltersAboveNotifyThreshold(t *testing.T) {
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

	if err := clientSession.SetLoggingLevel(ctx, &gomcp.SetLoggingLevelParams{Level: "debug"}); err != nil {
		t.Fatalf("SetLoggingLevel: %v", err)
	}

	// 0.08 is below ColdThreshold (0.1) but above NotifyThreshold (0.05) → filtered out.
	if got := s.NotifyColdNodes(ctx, []*store.Node{mkNode("warmish", 0.08)}); len(got) != 0 {
		t.Fatalf("notified = %v, want empty", got)
	}

	select {
	case params := <-received:
		t.Fatalf("unexpected notification for above-threshold node: %+v", params)
	case <-time.After(200 * time.Millisecond):
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

	notified := s.NotifyColdNodes(ctx, []*store.Node{mkNode("frozen", 0.01)})
	if len(notified) != 1 || notified[0] != "frozen" {
		t.Errorf("notified = %v, want [frozen]", notified)
	}

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
