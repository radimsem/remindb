package mcp

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"time"

	"github.com/radimsem/remindb/internal/fileext"
	"github.com/radimsem/remindb/internal/ignore"
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
	walkFn   func(root string, fn fs.WalkDirFunc) error
	modTimes map[string]time.Time
	logger   *slog.Logger
	ignore   *ignore.Matcher
}

func NewRescanLoop(st *store.Store, dir string, interval time.Duration, logger *slog.Logger) (*RescanLoop, error) {
	if interval <= 0 {
		interval = defaultRescanInterval
	}
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	matcher, err := ignore.Load(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to load: %s: %w", ignore.FileName, err)
	}

	return &RescanLoop{
		store:    st,
		dir:      dir,
		interval: interval,
		settle:   defaultSettleTime,
		now:      time.Now,
		walkFn:   filepath.WalkDir,
		modTimes: make(map[string]time.Time),
		logger:   logger,
		ignore:   matcher,
	}, nil
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
	var changed []string
	seen := make(map[string]bool, len(r.modTimes))
	pending := make(map[string]time.Time)
	now := r.now()

	walkErr := r.walkFn(r.dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			r.logger.Warn("rescan: walk error", "path", path, "err", err)
			return err
		}

		rel, _ := filepath.Rel(r.dir, path)
		rel = filepath.ToSlash(rel)

		if d.IsDir() {
			if path != r.dir && fileext.ShouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			if r.ignore.Match(rel, true) {
				return filepath.SkipDir
			}
			return nil
		}

		if rel == ignore.FileName {
			return nil
		}
		if !fileext.Supported(path) {
			return nil
		}
		if r.ignore.Match(rel, false) {
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

	// Purge entries for deleted files only when the walk was complete.
	if walkErr != nil {
		r.logger.Error("rescan: walk aborted", "dir", r.dir, "err", walkErr)
		return
	}

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

	// Lock only around store mutations.
	r.store.OpMu.Lock()
	defer r.store.OpMu.Unlock()

	r.reconcileDeleted(ctx, deleted)

	if len(changed) == 0 {
		r.logger.Debug("rescan: no changes", "watched", len(r.modTimes))
		return
	}

	result, err := compiler.Compile(ctx, r.store,
		compiler.WithPaths(changed),
		compiler.WithMessage("rescan"),
		compiler.WithCompileRoot(r.dir),
		compiler.WithLogger(r.logger),
	)
	if err != nil {
		r.logger.Error("rescan: compile failed", "err", err)
		return
	}

	maps.Copy(r.modTimes, pending)

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
