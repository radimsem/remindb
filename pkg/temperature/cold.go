package temperature

import (
	"context"
	"time"

	"github.com/radimsem/remindb/pkg/store"
)

type ColdHandler func(ctx context.Context, nodes []*store.Node)

func (t *Tracker) Run(ctx context.Context, handler ColdHandler) {
	t.reloadConfig()

	ticker := time.NewTicker(t.tickInterval())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if t.reloadConfig() {
				ticker.Reset(t.tickInterval())
			}

			cfg, enabled := t.snapshot()
			if !enabled {
				t.logger.Debug("temperature: disabled; skipping tick")
				continue
			}

			result, err := t.Tick(ctx, cfg.TickInterval)
			if err != nil {
				t.logger.Error("temperature tick failed", "err", err)
				continue
			}

			t.logger.Debug("temperature tick", "decayed", result.Decayed, "cold", len(result.Cold))

			if result.Decayed > 0 && t.onTick != nil {
				t.onTick()
			}

			if len(result.Cold) == 0 {
				continue
			}
			if handler != nil {
				handler(ctx, result.Cold)
			}
		}
	}
}

// Resolve the ticker period, falling back to the bootstrap default when a disabled config left it non-positive.
func (t *Tracker) tickInterval() time.Duration {
	t.mu.Lock()
	d := t.cfg.TickInterval
	t.mu.Unlock()

	if d <= 0 {
		return t.bootstrap.TickInterval
	}
	return d
}
