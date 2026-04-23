package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/radimsem/remindb/internal/fileext"
	"github.com/radimsem/remindb/pkg/compiler"
	"github.com/radimsem/remindb/pkg/diff"
	"github.com/radimsem/remindb/pkg/emitter"
	"github.com/radimsem/remindb/pkg/parser"
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
	// Startup reconcile catches edits made between the last compile and now.
	r.scan(ctx)

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

func (r *RescanLoop) scan(ctx context.Context) {
	r.store.OpMu.Lock()
	defer r.store.OpMu.Unlock()

	var changed []string
	seen := make(map[string]bool, len(r.modTimes))
	pending := make(map[string]time.Time)
	now := r.now()

	walkErr := filepath.WalkDir(r.dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			r.logger.Warn("rescan: walk error", "path", path, "err", err)
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
			pending[path] = mtime
		}
		return nil
	})
	if walkErr != nil {
		r.logger.Error("rescan: walk failed", "dir", r.dir, "err", walkErr)
	}

	// Purge entries for deleted files.
	var deleted []string
	for path := range r.modTimes {
		if seen[path] {
			continue
		}
		delete(r.modTimes, path)

		rel, err := filepath.Rel(r.dir, path)
		if err != nil {
			rel = path
		}
		deleted = append(deleted, rel)
	}
	r.reconcileDeleted(ctx, deleted)

	if len(changed) == 0 {
		r.logger.Debug("rescan: no changes", "watched", len(r.modTimes))
		return
	}

	result, err := compiler.Compile(ctx, r.store,
		compiler.WithPaths(changed),
		compiler.WithMessage("rescan"),
		compiler.WithCompileRoot(r.dir),
	)
	if err != nil {
		r.logger.Error("rescan: compile failed", "err", err)
		return
	}

	for path, mtime := range pending {
		r.modTimes[path] = mtime
	}

	r.logger.Info("rescan: applied",
		"added", result.Added,
		"modified", result.Modified,
		"removed", result.Removed,
		"total", result.Total,
	)
}

func (r *RescanLoop) reconcileDeleted(ctx context.Context, deleted []string) {
	if len(deleted) == 0 {
		return
	}

	nodes, err := r.store.GetNodesByFiles(ctx, deleted)
	if err != nil {
		r.logger.Error("rescan: load deleted nodes failed", "err", err)
		return
	}
	if len(nodes) == 0 {
		return
	}

	deltas := make([]diff.Delta, 0, len(nodes))
	synthetic := make([]*parser.ContextNode, 0, len(nodes))
	for _, n := range nodes {
		deltas = append(deltas, diff.Delta{
			NodeID:     n.ID,
			Op:         diff.OpRem,
			OldHash:    n.ContentHash,
			OldContent: n.Content,
		})
		// "rem:" prefix keeps the delete cursor-hash domain disjoint from compile's.
		synthetic = append(synthetic, &parser.ContextNode{ContentHash: "rem:" + n.ID + ":" + n.ContentHash})
	}

	msg := fmt.Sprintf("rescan: purged %d files", len(deleted))
	if err := emitter.Emit(ctx, r.store,
		emitter.WithDeltas(deltas),
		emitter.WithCursorHash(diff.CursorHashFlat(synthetic)),
		emitter.WithMessage(msg),
	); err != nil {
		r.logger.Error("rescan: purge emit failed", "err", err)
		return
	}

	r.logger.Info("rescan: purged deleted files",
		"files", len(deleted),
		"nodes", len(nodes),
		"paths", deleted,
	)
}
