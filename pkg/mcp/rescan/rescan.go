// Package rescan periodically recompiles a source tree into the store.
package rescan

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/radimsem/remindb/internal/contentid"
	"github.com/radimsem/remindb/internal/fileext"
	"github.com/radimsem/remindb/internal/ignore"
	"github.com/radimsem/remindb/internal/loghelper"
	"github.com/radimsem/remindb/pkg/compiler"
	"github.com/radimsem/remindb/pkg/config"
	"github.com/radimsem/remindb/pkg/diff"
	"github.com/radimsem/remindb/pkg/emitter"
	"github.com/radimsem/remindb/pkg/mcp/rescanlog"
	"github.com/radimsem/remindb/pkg/mcp/rescanstat"
	"github.com/radimsem/remindb/pkg/parser"
	"github.com/radimsem/remindb/pkg/store"
)

const (
	defaultRescanInterval = 30 * time.Second
	defaultSettleTime     = 500 * time.Millisecond
)

type Option func(*options)

type options struct {
	compileConfig config.CompileConfig
	logger        *slog.Logger
	status        *rescanstat.Status
	rescanLog     *rescanlog.Sink
}

func WithCompileConfig(cc config.CompileConfig) Option {
	return func(o *options) { o.compileConfig = cc }
}

func WithLogger(l *slog.Logger) Option {
	return func(o *options) { o.logger = l }
}

func WithStatus(s *rescanstat.Status) Option {
	return func(o *options) { o.status = s }
}

func WithRescanLog(s *rescanlog.Sink) Option {
	return func(o *options) { o.rescanLog = s }
}

type Loop struct {
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
	status            *rescanstat.Status
	rescanLog         *rescanlog.Sink
	onChange          func()
}

func (r *Loop) SetChangeObserver(fn func()) {
	r.onChange = fn
}

func (r *Loop) notifyChange() {
	if r.onChange != nil {
		r.onChange()
	}
}

func New(st *store.Store, dir string, interval time.Duration, opts ...Option) (*Loop, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}

	if interval <= 0 {
		interval = defaultRescanInterval
	}
	logger := loghelper.OrDiscard(o.logger)
	status := o.status
	if status == nil {
		status = rescanstat.New()
	}

	matcher, err := ignore.Load(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to load: %s: %w", ignore.Path, err)
	}

	return &Loop{
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
		compileOpts:       compiler.ConfigOptions(o.compileConfig),
		status:            status,
		rescanLog:         o.rescanLog,
	}, nil
}

func (r *Loop) Run(ctx context.Context) {
	r.reloadConfig()

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

func (r *Loop) reloadConfig() (intervalChanged bool) {
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

func (r *Loop) scan(ctx context.Context) {
	var changed []string
	seen := make(map[string]bool, len(r.modTimes))
	pending := make(map[string]time.Time)
	now := r.now()

	snap := rescanstat.Snapshot{RunAt: now.Unix()}
	defer func() {
		r.status.Set(int64(r.interval/time.Second), snap)

		if r.rescanLog != nil {
			if err := r.rescanLog.Append(snap); err != nil {
				r.logger.Warn("rescan: failed to persist tick", "err", err)
			}
		}
	}()

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
		r.logger.Error("rescan: walk aborted", "dir", r.dir, "err", walkErr)
		snap.Error = walkErr.Error()
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

	r.store.OpMu.Lock()
	defer r.store.OpMu.Unlock()

	snap.PurgedFiles = r.reconcileDeleted(ctx, deleted)

	if len(changed) == 0 {
		if len(deleted) > 0 {
			r.notifyChange()
		}

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
		snap.Error = err.Error()
		return
	}

	maps.Copy(r.modTimes, pending)
	snap.Added, snap.Modified, snap.Removed = result.Added, result.Modified, result.Removed

	r.logger.Info("rescan: applied",
		"added", result.Added,
		"modified", result.Modified,
		"removed", result.Removed,
		"total", result.Total,
	)

	r.notifyChange()
}

func (r *Loop) reconcileDeleted(ctx context.Context, deleted []string) []rescanstat.PurgedFile {
	if len(deleted) == 0 {
		return nil
	}

	nodes, err := r.store.GetNodesByFiles(ctx, deleted)
	if err != nil {
		r.logger.Error("rescan: load deleted nodes failed", "err", err)
		return nil
	}
	if len(nodes) == 0 {
		return nil
	}

	deltas := make([]diff.Delta, 0, len(nodes))
	synthetic := make([]*parser.ContextNode, 0, len(nodes))
	counts := make(map[string]int, len(deleted))
	for _, n := range nodes {
		deltas = append(deltas, diff.Delta{
			NodeID:     n.ID,
			Op:         diff.OpRem,
			OldHash:    n.ContentHash,
			OldContent: n.Content,
		})

		synthetic = append(synthetic, &parser.ContextNode{ContentHash: "rem:" + n.ID + ":" + n.ContentHash})
		counts[n.SourceFile]++
	}

	msg := fmt.Sprintf("rescan: purged %d files", len(deleted))
	if err := emitter.Emit(ctx, r.store,
		emitter.WithDeltas(deltas),
		emitter.WithCursorHash(diff.CursorHashFlat(synthetic)),
		emitter.WithMessage(msg),
	); err != nil {
		r.logger.Error("rescan: purge emit failed", "err", err)
		return nil
	}

	r.logger.Info("rescan: purged deleted files",
		"files", len(deleted),
		"nodes", len(nodes),
		"paths", deleted,
	)

	purged := make([]rescanstat.PurgedFile, 0, len(counts))
	for path, c := range counts {
		purged = append(purged, rescanstat.PurgedFile{Path: path, Nodes: c})
	}

	sort.Slice(purged, func(i, j int) bool { return purged[i].Path < purged[j].Path })
	return purged
}
