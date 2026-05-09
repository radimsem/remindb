package temperature

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/radimsem/remindb/pkg/store"
)

type NodeStore interface {
	BoostTemperatureBatch(ctx context.Context, ids []string, boost float64) error
	DecayTemperatures(ctx context.Context, factor float64) (int64, error)
	GetColdNodes(ctx context.Context, threshold float64, limit int) ([]*store.Node, error)
}

type TickResult struct {
	Decayed int64
	Cold    []*store.Node
}

type Tracker struct {
	store  NodeStore
	cfg    Config
	logger *slog.Logger

	mu               sync.Mutex
	recentlyNotified map[string]time.Time
}

func NewTracker(s NodeStore, cfg Config, logger *slog.Logger) (*Tracker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Tracker{
		store:            s,
		cfg:              cfg,
		logger:           logger,
		recentlyNotified: make(map[string]time.Time),
	}, nil
}

func (t *Tracker) RecordAccess(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if err := t.store.BoostTemperatureBatch(ctx, ids, t.cfg.AccessBoost); err != nil {
		return fmt.Errorf("failed to boost: %w", err)
	}
	return nil
}

func (t *Tracker) Tick(ctx context.Context, elapsed time.Duration) (*TickResult, error) {
	hours := elapsed.Hours()
	factor := decayFactor(t.cfg.DecayRate, hours)

	decayed, err := t.store.DecayTemperatures(ctx, factor)
	if err != nil {
		return nil, fmt.Errorf("failed to decay: %w", err)
	}

	cold, err := t.store.GetColdNodes(ctx, t.cfg.ColdThreshold, t.cfg.ColdNotifyLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to get cold nodes: %w", err)
	}

	return &TickResult{Decayed: decayed, Cold: t.dedupCold(cold, time.Now())}, nil
}

// Drop cold nodes notified within ColdNotifyTTL; evict stale entries.
func (t *Tracker) dedupCold(cold []*store.Node, now time.Time) []*store.Node {
	t.mu.Lock()
	defer t.mu.Unlock()

	ttl := t.cfg.ColdNotifyTTL
	for id, ts := range t.recentlyNotified {
		if now.Sub(ts) > ttl {
			delete(t.recentlyNotified, id)
		}
	}

	out := make([]*store.Node, 0, len(cold))
	for _, n := range cold {
		if _, ok := t.recentlyNotified[n.ID]; ok {
			continue
		}
		out = append(out, n)
	}
	return out
}

// Stamp ids as notified at time.Now(); pair with dedupCold after a successful send.
func (t *Tracker) MarkNotified(ids []string) {
	if len(ids) == 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	for _, id := range ids {
		t.recentlyNotified[id] = now
	}
}
