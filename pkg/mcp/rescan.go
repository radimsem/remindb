package mcp

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/radimsem/remindb/internal/fileext"
	"github.com/radimsem/remindb/pkg/compiler"
	"github.com/radimsem/remindb/pkg/store"
)

const (
	defaultRescanInterval = 30 * time.Second
	defaultSettleTime     = 500 * time.Millisecond
)

type RescanLoop struct {
	store    *store.Store
	dir      string
	interval time.Duration
	settle   time.Duration
	now      func() time.Time
	modTimes map[string]time.Time
	logger   *slog.Logger
}

func NewRescanLoop(st *store.Store, dir string, interval time.Duration, logger *slog.Logger) *RescanLoop {
	if interval <= 0 {
		interval = defaultRescanInterval
	}
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &RescanLoop{
		store:    st,
		dir:      dir,
		interval: interval,
		settle:   defaultSettleTime,
		now:      time.Now,
		modTimes: make(map[string]time.Time),
		logger:   logger,
	}
}

func (r *RescanLoop) Run(ctx context.Context) {
	// Avoid recompiling all files on the first tick.
	r.seedMtimes()

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.scan(ctx)
		}
	}
}

func (r *RescanLoop) seedMtimes() {
	_ = filepath.WalkDir(r.dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != r.dir && fileext.ShouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !fileext.Supported(path) {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		r.modTimes[path] = info.ModTime()
		return nil
	})
}

func (r *RescanLoop) scan(ctx context.Context) {
	var changed []string
	seen := make(map[string]bool, len(r.modTimes))
	now := r.now()

	_ = filepath.WalkDir(r.dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != r.dir && fileext.ShouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !fileext.Supported(path) {
			return nil
		}

		seen[path] = true

		info, err := d.Info()
		if err != nil {
			return nil
		}

		mtime := info.ModTime()

		// Skip files still settling.
		if now.Sub(mtime) < r.settle {
			return nil
		}

		prev, ok := r.modTimes[path]
		if !ok || mtime.After(prev) {
			changed = append(changed, path)
			r.modTimes[path] = mtime
		}
		return nil
	})

	// Purge entries for deleted files.
	for path := range r.modTimes {
		if !seen[path] {
			delete(r.modTimes, path)
		}
	}

	if len(changed) == 0 {
		r.logger.Debug("rescan: no changes", "watched", len(r.modTimes))
		return
	}

	result, err := compiler.Compile(ctx, r.store, changed, "rescan", r.dir, nil)
	if err != nil {
		r.logger.Error("rescan: compile failed", "err", err)
		return
	}

	r.logger.Info("rescan: applied",
		"added", result.Added,
		"modified", result.Modified,
		"removed", result.Removed,
		"total", result.Total,
	)
}
