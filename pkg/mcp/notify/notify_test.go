package notify

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	mcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/pkg/config"
	"github.com/radimsem/remindb/pkg/mcp/resources"
)

func dur(d time.Duration) *config.Duration {
	c := config.Duration(d)
	return &c
}

func TestNewPublisher_IntervalResolution(t *testing.T) {
	cfg := config.ResourcesConfig{
		Debounce:  dur(300 * time.Millisecond),
		Overrides: map[string]*config.Duration{"logs": dur(5 * time.Second)},
	}

	p, err := NewPublisher(cfg, nil)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}

	cases := map[string]time.Duration{
		resources.GraphURI:       300 * time.Millisecond, // global default
		resources.TemperatureURI: 2 * time.Second,        // builtin floor, not overridden
		resources.LogsURI:        5 * time.Second,        // config override wins over builtin
	}
	for uri, want := range cases {
		if got := p.perURI[uri]; got != want {
			t.Errorf("perURI[%s] = %v, want %v", uri, got, want)
		}
	}
}

func TestNewPublisher_DefaultWhenAbsent(t *testing.T) {
	p, err := NewPublisher(config.ResourcesConfig{}, nil)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}

	if got := p.perURI[resources.GraphURI]; got != DefaultDebounce {
		t.Errorf("graph debounce = %v, want %v", got, DefaultDebounce)
	}
}

func TestNewPublisher_UnknownOverrideRejected(t *testing.T) {
	cfg := config.ResourcesConfig{Overrides: map[string]*config.Duration{"nope": dur(time.Second)}}

	if _, err := NewPublisher(cfg, nil); err == nil {
		t.Fatal("expected error for unknown override resource")
	}
}

func TestHandleSubscribe_Validation(t *testing.T) {
	p, _ := NewPublisher(config.ResourcesConfig{}, nil)

	known := &mcp.SubscribeRequest{Params: &mcp.SubscribeParams{URI: resources.GraphURI}}
	if err := p.HandleSubscribe(context.Background(), known); err != nil {
		t.Errorf("subscribe to known URI: %v", err)
	}

	bad := &mcp.SubscribeRequest{Params: &mcp.SubscribeParams{URI: "remindb://overview"}}
	if err := p.HandleSubscribe(context.Background(), bad); err == nil {
		t.Error("expected error subscribing to non-subscribable URI")
	}
}

func TestTouch_UnknownURIIsNoOp(t *testing.T) {
	p, _ := NewPublisher(config.ResourcesConfig{}, nil)

	p.Touch("remindb://overview") // not subscribable; must not schedule or panic

	p.mu.Lock()
	n := len(p.timers)
	p.mu.Unlock()

	if n != 0 {
		t.Errorf("unknown Touch scheduled %d timers, want 0", n)
	}
}

func TestTouch_CoalescesBurstToOneNotification(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := config.ResourcesConfig{Debounce: dur(40 * time.Millisecond)}
	pub, err := NewPublisher(cfg, nil)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	defer pub.Close()

	srv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.1.0"}, &mcp.ServerOptions{
		SubscribeHandler:   pub.HandleSubscribe,
		UnsubscribeHandler: pub.HandleUnsubscribe,
	})

	srv.AddResource(&mcp.Resource{Name: "graph", URI: resources.GraphURI, MIMEType: "application/json"},
		func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{}, nil
		})
	pub.Attach(srv)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := srv.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}

	var got atomic.Int32
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0.1.0"}, &mcp.ClientOptions{
		ResourceUpdatedHandler: func(context.Context, *mcp.ResourceUpdatedNotificationRequest) {
			got.Add(1)
		},
	})
	cs, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() { _ = cs.Close() }()

	if err := cs.Subscribe(ctx, &mcp.SubscribeParams{URI: resources.GraphURI}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	for range 20 {
		pub.Touch(resources.GraphURI)
		time.Sleep(time.Millisecond)
	}

	time.Sleep(200 * time.Millisecond) // > debounce + slack
	if n := got.Load(); n != 1 {
		t.Errorf("got %d notifications, want exactly 1 (coalesced)", n)
	}
}
