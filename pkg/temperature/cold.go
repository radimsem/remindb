package temperature

import (
	"context"
	"time"

	"github.com/radimsem/remindb/pkg/store"
)

type ColdHandler func(ctx context.Context, nodes []*store.Node)

func (t *Tracker) Run(ctx context.Context, handler ColdHandler) {
	ticker := time.NewTicker(t.cfg.TickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			result, err := t.Tick(ctx, t.cfg.TickInterval)
			if err != nil {
				t.logger.Error("temperature tick failed", "err", err)
				continue
			}

			t.logger.Debug("temperature tick", "decayed", result.Decayed, "cold", len(result.Cold))

			if len(result.Cold) == 0 {
				continue
			}
			if handler != nil {
				handler(ctx, result.Cold)
			}
		}
	}
}
