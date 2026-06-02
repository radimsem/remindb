package compiler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/radimsem/remindb/internal/fileext"
	"github.com/radimsem/remindb/internal/loghelper"
	"github.com/radimsem/remindb/internal/pathmatch"
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
	pins        map[string]bool
	logger      *slog.Logger
	ignore      *pathmatch.Matcher
	pinned      *pathmatch.Matcher
	redactor    *redaction.Redactor
	maxFileSize int64
	maxParallel int
	timeout     time.Duration
	ignoreSet   bool
	pinnedSet   bool
	fullRescan  bool
	reseedTemps bool
	reseedPins  bool
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

func WithPins(p map[string]bool) Option {
	return func(o *options) { o.pins = p }
}

func WithLogger(l *slog.Logger) Option {
	return func(o *options) { o.logger = l }
}

func WithIgnore(m *pathmatch.Matcher) Option {
	return func(o *options) {
		o.ignore = m
		o.ignoreSet = true
	}
}

func WithPinned(m *pathmatch.Matcher) Option {
	return func(o *options) {
		o.pinned = m
		o.pinnedSet = true
	}
}

func WithFullRescan() Option {
	return func(o *options) { o.fullRescan = true }
}

func WithReseedTemperatures() Option {
	return func(o *options) { o.reseedTemps = true }
}

func WithReseedPinned() Option {
	return func(o *options) { o.reseedPins = true }
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

	logger := loghelper.OrDefault(o.logger)

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
				if errors.Is(err, parser.ErrMalformed) {
					logger.Warn("compile: skipping malformed file", "path", p, "err", err)
					return nil
				}
				return fmt.Errorf("failed to parse: %s: %w", p, err)
			}

			if t := o.temps[p]; t != nil {
				seedTemp(nodes, t)
			}
			if o.pins[p] {
				seedPin(nodes)
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

	var skipped []string
	for i, nodes := range results {
		if len(nodes) == 0 {
			skipped = append(skipped, o.paths[i])
		}
	}

	prev, err := buildPrevState(ctx, st, flat, skipped, o.fullRescan, o.compileRoot)
	if err != nil {
		return nil, err
	}

	deltas := diff.DiffFlat(flat, prev)
	cursorHash := diff.CursorHashFlat(flat)

	// Emit and relations share one transaction so the snapshot, node mutations,
	// cursor advance, and relation writes commit or roll back together. A relations
	// failure must not leave HEAD ahead of the error returned to the caller.
	err = st.Tx(ctx, func(tx *sql.Tx) error {
		if err := emitter.EmitTx(ctx, st, tx,
			emitter.WithRoots(roots),
			emitter.WithDeltas(deltas),
			emitter.WithCursorHash(cursorHash),
			emitter.WithMessage(o.message),
			emitter.WithCompileRoot(o.compileRoot),
		); err != nil {
			return fmt.Errorf("failed to emit: %w", err)
		}

		if err := relations.RunTx(ctx, st, tx, flat); err != nil {
			return fmt.Errorf("failed to resolve relations: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return countResult(deltas), nil
}

func seedTemp(nodes []*parser.ContextNode, t *float64) {
	for _, n := range nodes {
		n.Temperature = t
		seedTemp(n.Children, t)
	}
}

func seedPin(nodes []*parser.ContextNode) {
	for _, n := range nodes {
		n.SeedPinned = true
		seedPin(n.Children)
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
		m, err := pathmatch.LoadIgnore(absDir)
		if err != nil {
			return nil, fmt.Errorf("failed to load: %s: %w", pathmatch.IgnorePath, err)
		}

		matcher = m
	}

	pinMatcher := o.pinned
	if !o.pinnedSet {
		m, err := pathmatch.LoadPinned(absDir)
		if err != nil {
			return nil, fmt.Errorf("failed to load: %s: %w", pathmatch.PinnedPath, err)
		}

		pinMatcher = m
	}

	// Resolve the root once so the containment check below compares walked
	// symlink targets against the root's real path, not a symlinked alias.
	rootReal, err := filepath.EvalSymlinks(absDir)
	if err != nil {
		rootReal = absDir
	}

	logger := loghelper.OrDefault(o.logger)

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
		if symlinkEscapesRoot(rootReal, path, d) {
			logger.Warn("compile: skipping symlink outside source root", "path", path)
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

	pins := ResolvePins(absDir, paths, pinMatcher)

	all := append([]Option{}, opts...)
	all = append(all,
		WithPaths(paths),
		WithMessage(message),
		WithCompileRoot(absDir),
		WithTemps(temps),
		WithPins(pins),
		WithFullRescan(),
	)

	result, err := Compile(ctx, st, all...)
	if err != nil {
		return nil, err
	}

	if err := reseed(ctx, st, absDir, temps, pins, o.reseedTemps, o.reseedPins); err != nil {
		return nil, err
	}
	return result, nil
}

// Containment applies to walked files too: a symlink in the tree must not pull in files outside the compile root.
func symlinkEscapesRoot(root, path string, entry os.DirEntry) bool {
	if entry.Type()&os.ModeSymlink == 0 {
		return false
	}

	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return true
	}

	rel, err := filepath.Rel(root, resolved)
	if err != nil {
		return true
	}
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// Apply reseed flags without emitting a snapshot; combined flags share one Tx.
func reseed(ctx context.Context, st *store.Store, compileRoot string, temps map[string]*float64, pins map[string]bool, reseedTemps, reseedPins bool) error {
	wantTemps := reseedTemps && len(temps) > 0
	wantPins := reseedPins && len(pins) > 0

	switch {
	case wantTemps && wantPins:
		return reseedBothTx(ctx, st, compileRoot, temps, pins)
	case wantTemps:
		return reseedTemperatures(ctx, st, compileRoot, temps)
	case wantPins:
		return reseedPinned(ctx, st, compileRoot, pins)
	}
	return nil
}

func reseedTemperatures(ctx context.Context, st *store.Store, compileRoot string, temps map[string]*float64) error {
	byTemp, err := groupByTemp(compileRoot, temps)
	if err != nil {
		return err
	}

	for temp, paths := range byTemp {
		if err := st.ResetTemperaturesByFiles(ctx, paths, temp); err != nil {
			return fmt.Errorf("failed to reseed temperatures: %w", err)
		}
	}
	return nil
}

func reseedPinned(ctx context.Context, st *store.Store, compileRoot string, pins map[string]bool) error {
	paths, err := pinPathsRel(compileRoot, pins)
	if err != nil {
		return err
	}

	if err := st.ResetPinnedByFiles(ctx, paths); err != nil {
		return fmt.Errorf("failed to reseed pinned: %w", err)
	}
	return nil
}

func reseedBothTx(ctx context.Context, st *store.Store, compileRoot string, temps map[string]*float64, pins map[string]bool) error {
	byTemp, err := groupByTemp(compileRoot, temps)
	if err != nil {
		return err
	}

	pinPaths, err := pinPathsRel(compileRoot, pins)
	if err != nil {
		return err
	}

	err = st.Tx(ctx, func(tx *sql.Tx) error {
		if err := st.ResetPinnedByFilesTx(ctx, tx, pinPaths); err != nil {
			return err
		}
		for temp, paths := range byTemp {
			if err := st.ResetTemperaturesByFilesTx(ctx, tx, paths, temp); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to reseed pinned + temperatures: %w", err)
	}
	return nil
}

func groupByTemp(compileRoot string, temps map[string]*float64) (map[float64][]string, error) {
	byTemp := make(map[float64][]string, len(temps))

	for path, t := range temps {
		if t == nil {
			continue
		}

		rel, err := filepath.Rel(compileRoot, path)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve: relative path for %s: %w", path, err)
		}

		byTemp[*t] = append(byTemp[*t], rel)
	}

	return byTemp, nil
}

func pinPathsRel(compileRoot string, pins map[string]bool) ([]string, error) {
	out := make([]string, 0, len(pins))

	for path := range pins {
		rel, err := filepath.Rel(compileRoot, path)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve: relative path for %s: %w", path, err)
		}
		out = append(out, rel)
	}
	return out, nil
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

	fileDir := filepath.Dir(absPath)

	var o options
	for _, opt := range opts {
		opt(&o)
	}

	pinMatcher := o.pinned
	if !o.pinnedSet {
		m, err := pathmatch.LoadPinned(fileDir)
		if err != nil {
			return nil, fmt.Errorf("failed to load: %s: %w", pathmatch.PinnedPath, err)
		}

		pinMatcher = m
	}

	pins := ResolvePins(fileDir, []string{absPath}, pinMatcher)

	all := append([]Option{}, opts...)
	all = append(all,
		WithPaths([]string{path}),
		WithMessage(message),
		WithCompileRoot(fileDir),
		WithPins(pins),
	)
	return Compile(ctx, st, all...)
}

func ResolvePins(dir string, paths []string, m *pathmatch.Matcher) map[string]bool {
	if m == nil {
		return nil
	}

	pins := make(map[string]bool, len(paths))
	for _, p := range paths {
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			continue
		}

		if m.Match(filepath.ToSlash(rel), false) {
			pins[p] = true
		}
	}
	return pins
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

func buildPrevState(ctx context.Context, st *store.Store, flat []*parser.ContextNode, skipped []string, fullRescan bool, compileRoot string) (map[string]diff.NodeState, error) {
	existing, err := loadPrevNodes(ctx, st, flat, skipped, fullRescan, compileRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to get nodes: %w", err)
	}

	prev := make(map[string]diff.NodeState, len(existing))
	for _, n := range existing {
		prev[n.ID] = diff.NodeState{Hash: n.ContentHash, Content: n.Content}
	}
	return prev, nil
}

func loadPrevNodes(ctx context.Context, st *store.Store, flat []*parser.ContextNode, skipped []string, fullRescan bool, compileRoot string) ([]*store.Node, error) {
	if fullRescan && compileRoot != "" {
		return st.GetNodesByCompileRoot(ctx, compileRoot)
	}

	files := appendSkippedKeys(uniqueFilesFlat(flat), skipped, compileRoot)
	return st.GetNodesByFiles(ctx, files)
}

// appendSkippedKeys unions the stored SourceFile key of each skipped path into files.
func appendSkippedKeys(files, skipped []string, compileRoot string) []string {
	if compileRoot == "" || len(skipped) == 0 {
		return files
	}

	seen := make(map[string]bool, len(files))
	for _, f := range files {
		seen[f] = true
	}

	for _, p := range skipped {
		key := transformer.StoredSourceFile(p, compileRoot)

		if !seen[key] {
			seen[key] = true
			files = append(files, key)
		}
	}
	return files
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
