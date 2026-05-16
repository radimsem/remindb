package compiler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/radimsem/remindb/internal/fileext"
	"github.com/radimsem/remindb/internal/ignore"
	"github.com/radimsem/remindb/internal/redaction"
	"github.com/radimsem/remindb/internal/tempfile"
	"github.com/radimsem/remindb/pkg/config"
	"github.com/radimsem/remindb/pkg/diff"
	"github.com/radimsem/remindb/pkg/emitter"
	"github.com/radimsem/remindb/pkg/parser"
	"github.com/radimsem/remindb/pkg/relations"
	"github.com/radimsem/remindb/pkg/store"
	"github.com/radimsem/remindb/pkg/transformer"
)

type Result struct {
	Added    int
	Modified int
	Removed  int
	Total    int
}

type Option func(*options)

type options struct {
	paths       []string
	message     string
	compileRoot string
	temps       map[string]*float64
	logger      *slog.Logger
	ignore      *ignore.Matcher
	redactor    *redaction.Redactor
	maxFileSize int64
	maxParallel int
	timeout     time.Duration
	ignoreSet   bool
	fullRescan  bool
	reseedTemps bool
}

func WithPaths(p []string) Option {
	return func(o *options) { o.paths = p }
}

func WithMessage(m string) Option {
	return func(o *options) { o.message = m }
}

func WithCompileRoot(r string) Option {
	return func(o *options) { o.compileRoot = r }
}

func WithTemps(t map[string]*float64) Option {
	return func(o *options) { o.temps = t }
}

func WithLogger(l *slog.Logger) Option {
	return func(o *options) { o.logger = l }
}

func WithIgnore(m *ignore.Matcher) Option {
	return func(o *options) {
		o.ignore = m
		o.ignoreSet = true
	}
}

func WithFullRescan() Option {
	return func(o *options) { o.fullRescan = true }
}

func WithReseedTemperatures() Option {
	return func(o *options) { o.reseedTemps = true }
}

func WithRedactor(r *redaction.Redactor) Option {
	return func(o *options) { o.redactor = r }
}

func WithMaxFileSize(n int64) Option {
	return func(o *options) { o.maxFileSize = n }
}

func WithMaxParallelism(n int) Option {
	return func(o *options) { o.maxParallel = n }
}

func WithWallClockTimeout(d time.Duration) Option {
	return func(o *options) { o.timeout = d }
}

// Translate the workspace compile block into options; absent fields keep engine defaults.
func ConfigOptions(cc config.CompileConfig) []Option {
	var opts []Option

	if cc.MaxFileSize != nil {
		opts = append(opts, WithMaxFileSize(int64(*cc.MaxFileSize)))
	}
	if cc.MaxParallelism != nil {
		opts = append(opts, WithMaxParallelism(*cc.MaxParallelism))
	}
	if cc.WallClockTimeout != nil {
		opts = append(opts, WithWallClockTimeout(time.Duration(*cc.WallClockTimeout)))
	}

	return opts
}

func withTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if d <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, d)
}

func Compile(ctx context.Context, st *store.Store, opts ...Option) (*Result, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}

	logger := o.logger
	if logger == nil {
		logger = slog.Default()
	}

	ctx, cancel := withTimeout(ctx, o.timeout)
	defer cancel()

	results := make([][]*parser.ContextNode, len(o.paths))

	limit := o.maxParallel
	if limit <= 0 {
		limit = runtime.GOMAXPROCS(0)
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(limit)

	for i, p := range o.paths {
		g.Go(func() error {
			if err := gctx.Err(); err != nil {
				return err
			}

			if o.maxFileSize > 0 {
				fi, err := os.Stat(p)
				if err != nil {
					return fmt.Errorf("failed to stat: %s: %w", p, err)
				}

				if fi.Size() > o.maxFileSize {
					logger.Warn("compile: skipping oversize file", "path", p, "size_bytes", fi.Size(), "max_bytes", o.maxFileSize)
					return nil
				}
			}

			data, err := os.ReadFile(p)
			if err != nil {
				return fmt.Errorf("failed to read: %w", err)
			}

			nodes, err := parser.ParseBytes(p, data)
			if err != nil {
				if errors.Is(err, parser.ErrUnsupportedExt) {
					logger.Warn("compile: skipping unsupported file", "path", p, "err", err)
					return nil
				}
				return fmt.Errorf("failed to parse: %s: %w", p, err)
			}

			if t := o.temps[p]; t != nil {
				seedTemp(nodes, t)
			}

			results[i] = nodes
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		if o.timeout > 0 && errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("failed to compile: wall-clock timeout exceeded after %s: %w", o.timeout, err)
		}
		return nil, err
	}

	total := 0
	for _, nodes := range results {
		total += len(nodes)
	}

	roots := make([]*parser.ContextNode, 0, total)
	for _, nodes := range results {
		roots = append(roots, nodes...)
	}

	if err := transformer.Transform(ctx, roots, o.compileRoot, o.redactor); err != nil {
		return nil, fmt.Errorf("failed to transform: %w", err)
	}

	flat := parser.Flatten(roots)

	prev, err := buildPrevState(ctx, st, flat, o.fullRescan, o.compileRoot)
	if err != nil {
		return nil, err
	}

	deltas := diff.DiffFlat(flat, prev)
	cursorHash := diff.CursorHashFlat(flat)

	err = emitter.Emit(ctx, st,
		emitter.WithRoots(roots),
		emitter.WithDeltas(deltas),
		emitter.WithCursorHash(cursorHash),
		emitter.WithMessage(o.message),
		emitter.WithCompileRoot(o.compileRoot),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to emit: %w", err)
	}

	if err := relations.Run(ctx, st, flat); err != nil {
		return nil, fmt.Errorf("failed to resolve relations: %w", err)
	}

	return countResult(deltas), nil
}

func seedTemp(nodes []*parser.ContextNode, t *float64) {
	for _, n := range nodes {
		n.Temperature = t
		seedTemp(n.Children, t)
	}
}

func CompileDir(ctx context.Context, st *store.Store, dir, message string, opts ...Option) (*Result, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}

	// Bound the whole compile, the directory walk included, not just Compile.
	ctx, cancel := withTimeout(ctx, o.timeout)
	defer cancel()

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve: %s: %w", dir, err)
	}

	matcher := o.ignore
	if !o.ignoreSet {
		m, err := ignore.Load(absDir)
		if err != nil {
			return nil, fmt.Errorf("failed to load: %s: %w", ignore.Path, err)
		}

		matcher = m
	}

	var paths []string
	err = filepath.WalkDir(absDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, _ := filepath.Rel(absDir, path)
		rel = filepath.ToSlash(rel)

		if d.IsDir() {
			name := d.Name()
			if path != absDir && (fileext.ShouldSkipDir(name) || name == config.DirName) {
				return filepath.SkipDir
			}

			if matcher.Match(rel, true) {
				return filepath.SkipDir
			}
			return nil
		}

		if !fileext.Supported(path) {
			return nil
		}
		if matcher.Match(rel, false) {
			return nil
		}

		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk: %s: %w", absDir, err)
	}

	if len(paths) == 0 {
		return &Result{}, nil
	}

	temps, err := resolveTemps(absDir, paths)
	if err != nil {
		return nil, err
	}

	all := append([]Option{}, opts...)
	all = append(all,
		WithPaths(paths),
		WithMessage(message),
		WithCompileRoot(absDir),
		WithTemps(temps),
		WithFullRescan(),
	)

	result, err := Compile(ctx, st, all...)
	if err != nil {
		return nil, err
	}

	if o.reseedTemps && len(temps) > 0 {
		if err := reseedTemperatures(ctx, st, absDir, temps); err != nil {
			return nil, fmt.Errorf("failed to reseed temperatures: %w", err)
		}
	}
	return result, nil
}

// Bypasses the emitter so the temperature update does not create a new snapshot.
func reseedTemperatures(ctx context.Context, st *store.Store, compileRoot string, temps map[string]*float64) error {
	byTemp := make(map[float64][]string, len(temps))

	for path, t := range temps {
		if t == nil {
			continue
		}

		rel, err := filepath.Rel(compileRoot, path)
		if err != nil {
			return fmt.Errorf("failed to resolve: relative path for %s: %w", path, err)
		}
		byTemp[*t] = append(byTemp[*t], rel)
	}

	for temp, paths := range byTemp {
		if err := st.ResetTemperaturesByFiles(ctx, paths, temp); err != nil {
			return err
		}
	}
	return nil
}

// Compile a single file; compile root anchors at the file's parent directory.
func CompileFile(ctx context.Context, st *store.Store, path, message string, opts ...Option) (*Result, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve: %s: %w", path, err)
	}

	if !fileext.Supported(path) {
		return nil, fmt.Errorf("%w: %q", parser.ErrUnsupportedExt, filepath.Ext(path))
	}

	all := append([]Option{}, opts...)
	all = append(all,
		WithPaths([]string{path}),
		WithMessage(message),
		WithCompileRoot(filepath.Dir(absPath)),
	)
	return Compile(ctx, st, all...)
}

func resolveTemps(dir string, paths []string) (map[string]*float64, error) {
	resolver, err := tempfile.Load(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to load: %s: %w", tempfile.Path, err)
	}
	if resolver == nil {
		return nil, nil
	}

	temps := make(map[string]*float64, len(paths))
	for _, p := range paths {
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve: relative path for %s: %w", p, err)
		}

		if t, ok := resolver.Resolve(rel); ok {
			temps[p] = &t
		}
	}
	return temps, nil
}

func buildPrevState(ctx context.Context, st *store.Store, flat []*parser.ContextNode, fullRescan bool, compileRoot string) (map[string]diff.NodeState, error) {
	existing, err := loadPrevNodes(ctx, st, flat, fullRescan, compileRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to get nodes: %w", err)
	}

	prev := make(map[string]diff.NodeState, len(existing))
	for _, n := range existing {
		prev[n.ID] = diff.NodeState{Hash: n.ContentHash, Content: n.Content}
	}
	return prev, nil
}

func loadPrevNodes(ctx context.Context, st *store.Store, flat []*parser.ContextNode, fullRescan bool, compileRoot string) ([]*store.Node, error) {
	if fullRescan && compileRoot != "" {
		return st.GetNodesByCompileRoot(ctx, compileRoot)
	}
	return st.GetNodesByFiles(ctx, uniqueFilesFlat(flat))
}

func uniqueFilesFlat(flat []*parser.ContextNode) []string {
	seen := make(map[string]bool, len(flat))
	out := make([]string, 0, len(flat))

	for _, n := range flat {
		if !seen[n.SourceFile] {
			seen[n.SourceFile] = true
			out = append(out, n.SourceFile)
		}
	}
	return out
}

func countResult(deltas []diff.Delta) *Result {
	r := &Result{}

	for _, d := range deltas {
		switch d.Op {
		case diff.OpAdd:
			r.Added++
		case diff.OpMod:
			r.Modified++
		case diff.OpRem:
			r.Removed++
		}
	}

	r.Total = r.Added + r.Modified + r.Removed
	return r
}
