package mcp

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/radimsem/remindb/internal/loghelper"
	"github.com/radimsem/remindb/pkg/compiler"
	"github.com/radimsem/remindb/pkg/store"
)

// Run an initial compile when the store is empty; no-op otherwise.
func MaybeInitialCompile(ctx context.Context, st *store.Store, dir string, logger *slog.Logger) error {
	logger = loghelper.OrDiscard(logger)

	stats, err := st.GetStats(ctx)
	if err != nil {
		return fmt.Errorf("failed to stat: %w", err)
	}
	if stats.NodeCount > 0 {
		return nil
	}

	logger.Info("serve: empty DB detected, running initial compile", "source", dir)

	result, err := compiler.CompileDir(ctx, st, dir, "initial", compiler.WithLogger(logger))
	if err != nil {
		return fmt.Errorf("failed to compile: %w", err)
	}

	logger.Info("serve: initial compile done",
		"added", result.Added,
		"modified", result.Modified,
		"removed", result.Removed,
		"total", result.Total,
	)
	return nil
}
