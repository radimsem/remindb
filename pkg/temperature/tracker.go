package temperature

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/radimsem/remindb/internal/contentid"
	"github.com/radimsem/remindb/pkg/config"
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
	store            NodeStore
	bootstrap        Config
	dir              string
	logger           *slog.Logger
	configHash       string
	mu               sync.Mutex
	cfg              Config
	enabled          bool
	recentlyNotified map[string]time.Time
}

func NewTracker(s NodeStore, dir string, bootstrap Config, logger *slog.Logger) (*Tracker, error) {
	if err := bootstrap.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Tracker{
		store:            s,
		bootstrap:        bootstrap,
		dir:              dir,
		logger:           logger,
		cfg:              bootstrap,
		enabled:          bootstrap.Enabled,
		recentlyNotified: make(map[string]time.Time),
	}, nil
}

func (t *Tracker) snapshot() (Config, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.cfg, t.enabled
}

// Re-source the temperature block from config.json; keep last-good on failure.
func (t *Tracker) reloadConfig() (intervalChanged bool) {
	if t.dir == "" {
		return false
	}

	path := filepath.Join(t.dir, config.DirName, config.FileName)

	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			t.logger.Warn("temperature: failed to read config; keeping last-good", "path", config.Path, "err", err)
			return false
		}
		data = nil
	}

	hash := contentid.ContentHash(string(data))
	if hash == t.configHash {
		return false
	}

	cfg, err := config.Parse(data)
	if err != nil {
		t.logger.Warn("temperature: invalid config; keeping last-good settings", "path", config.Path, "err", err)
		return false
	}

	next := t.bootstrap.WithOverrides(cfg.Temperature)
	if err := next.Validate(); err != nil {
		t.logger.Warn("temperature: invalid config; keeping last-good settings", "path", config.Path, "err", err)
		return false
	}
	t.configHash = hash

	t.mu.Lock()
	intervalChanged = next.TickInterval != t.cfg.TickInterval
	t.cfg, t.enabled = next, next.Enabled
	t.mu.Unlock()

	return intervalChanged
}

func (t *Tracker) RecordAccess(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	cfg, _ := t.snapshot()
	if err := t.store.BoostTemperatureBatch(ctx, ids, cfg.AccessBoost); err != nil {
		return fmt.Errorf("failed to boost: %w", err)
	}
	return nil
}

func (t *Tracker) Tick(ctx context.Context, elapsed time.Duration) (*TickResult, error) {
	cfg, _ := t.snapshot()

	hours := elapsed.Hours()
	factor := decayFactor(cfg.DecayRate, hours)

	decayed, err := t.store.DecayTemperatures(ctx, factor)
	if err != nil {
		return nil, fmt.Errorf("failed to decay: %w", err)
	}

	cold, err := t.store.GetColdNodes(ctx, cfg.ColdThreshold, cfg.ColdNotifyLimit)
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
