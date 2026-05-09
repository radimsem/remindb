package temperature

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/radimsem/remindb/pkg/store"
)

type NodeStore interface {
	BoostTemperatureBatch(ctx context.Context, ids []string, boost float64) error
	DecayTemperatures(ctx context.Context, factor float64) (int64, error)
	GetColdNodes(ctx context.Context, threshold float64) ([]*store.Node, error)
}

type TickResult struct {
	Decayed int64
	Cold    []*store.Node
}

type Tracker struct {
	store  NodeStore
	cfg    Config
	logger *slog.Logger
}

func NewTracker(s NodeStore, cfg Config, logger *slog.Logger) (*Tracker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Tracker{store: s, cfg: cfg, logger: logger}, nil
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

	cold, err := t.store.GetColdNodes(ctx, t.cfg.ColdThreshold)
	if err != nil {
		return nil, fmt.Errorf("failed to get cold nodes: %w", err)
	}

	return &TickResult{Decayed: decayed, Cold: cold}, nil
}
