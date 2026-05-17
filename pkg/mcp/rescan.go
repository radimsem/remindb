package mcp

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"time"

	"github.com/radimsem/remindb/internal/contentid"
	"github.com/radimsem/remindb/internal/fileext"
	"github.com/radimsem/remindb/internal/ignore"
	"github.com/radimsem/remindb/pkg/compiler"
	"github.com/radimsem/remindb/pkg/config"
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
	store             *store.Store
	dir               string
	bootstrapInterval time.Duration
	interval          time.Duration
	settle            time.Duration
	enabled           bool
	configHash        string
	now               func() time.Time
	walkFn            func(root string, fn fs.WalkDirFunc) error
	modTimes          map[string]time.Time
	logger            *slog.Logger
	ignore            *ignore.Matcher
	compileOpts       []compiler.Option
}

func NewRescanLoop(st *store.Store, dir string, interval time.Duration, cc config.CompileConfig, logger *slog.Logger) (*RescanLoop, error) {
	if interval <= 0 {
		interval = defaultRescanInterval
	}
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	matcher, err := ignore.Load(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to load: %s: %w", ignore.Path, err)
	}

	return &RescanLoop{
		store:             st,
		dir:               dir,
		bootstrapInterval: interval,
		interval:          interval,
		settle:            defaultSettleTime,
		enabled:           true,
		now:               time.Now,
		walkFn:            filepath.WalkDir,
		modTimes:          make(map[string]time.Time),
		logger:            logger,
		ignore:            matcher,
		compileOpts:       compiler.ConfigOptions(cc),
	}, nil
}

func (r *RescanLoop) Run(ctx context.Context) {
	r.reloadConfig()

	// Startup reconcile catches edits made between the last compile and now.
	if r.enabled {
		r.scan(ctx)
	}

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if r.reloadConfig() {
				ticker.Reset(r.interval)
			}

			if !r.enabled {
				r.logger.Debug("rescan: disabled; skipping tick")
				continue
			}
			r.scan(ctx)
		}
	}
}

// Re-source the rescan block from config.json.
func (r *RescanLoop) reloadConfig() (intervalChanged bool) {
	path := filepath.Join(r.dir, config.DirName, config.FileName)

	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			r.logger.Warn("rescan: failed to read config; keeping last-good", "path", config.Path, "err", err)
			return false
		}
		data = nil
	}

	hash := contentid.ContentHash(string(data))
	if hash == r.configHash {
		return false
	}

	cfg, err := config.Parse(data)
	if err != nil {
		r.logger.Warn("rescan: invalid config; keeping last-good settings", "path", config.Path, "err", err)
		return false
	}
	r.configHash = hash

	enabled := true
	interval := r.bootstrapInterval
	settle := defaultSettleTime

	rc := cfg.Rescan
	if rc.Enabled != nil {
		enabled = *rc.Enabled
	}
	if rc.Interval != nil {
		interval = time.Duration(*rc.Interval)
	}
	if rc.Settle != nil {
		settle = time.Duration(*rc.Settle)
	}

	intervalChanged = interval != r.interval
	r.enabled, r.interval, r.settle = enabled, interval, settle
	return intervalChanged
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
			name := d.Name()
			if path != r.dir && (fileext.ShouldSkipDir(name) || name == config.DirName) {
				return filepath.SkipDir
			}

			if r.ignore.Match(rel, true) {
				return filepath.SkipDir
			}
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

	copts := append([]compiler.Option{
		compiler.WithPaths(changed),
		compiler.WithMessage("rescan"),
		compiler.WithCompileRoot(r.dir),
		compiler.WithLogger(r.logger),
	}, r.compileOpts...)

	result, err := compiler.Compile(ctx, r.store, copts...)
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
