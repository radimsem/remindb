package temperature

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/radimsem/remindb/internal/testutil"
	"github.com/radimsem/remindb/pkg/store"
)

func TestRun_CallsHandler(t *testing.T) {
	st := testutil.OpenTestDB(t)
	ctx := context.Background()

	seedNode(t, st, "cold0001", 0.05)

	cfg := DefaultConfig()
	cfg.TickInterval = 50 * time.Millisecond
	cfg.ColdThreshold = 0.1
	tr := NewTracker(st, cfg, nil)

	var mu sync.Mutex
	var got []*store.Node

	handler := func(_ context.Context, nodes []*store.Node) {
		mu.Lock()
		got = append(got, nodes...)
		mu.Unlock()
	}

	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		tr.Run(ctx, handler)
		close(done)
	}()

	// Wait for at least one tick.
	time.Sleep(150 * time.Millisecond)
	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()

	if len(got) == 0 {
		t.Error("handler never called, expected cold nodes")
	}
}

func TestRun_StopsOnCancel(t *testing.T) {
	st := testutil.OpenTestDB(t)

	cfg := DefaultConfig()
	cfg.TickInterval = 50 * time.Millisecond
	tr := NewTracker(st, cfg, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		tr.Run(ctx, nil)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not stop after cancel")
	}
}
