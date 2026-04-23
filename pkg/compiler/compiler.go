package compiler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/radimsem/remindb/internal/fileext"
	"github.com/radimsem/remindb/internal/tempfile"
	"github.com/radimsem/remindb/pkg/diff"
	"github.com/radimsem/remindb/pkg/emitter"
	"github.com/radimsem/remindb/pkg/parser"
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

func Compile(ctx context.Context, st *store.Store, opts ...Option) (*Result, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}

	logger := o.logger
	if logger == nil {
		logger = slog.Default()
	}

	var roots []*parser.ContextNode
	for _, p := range o.paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("failed to read: %s: %w", p, err)
		}

		nodes, err := parser.ParseBytes(p, data)
		if err != nil {
			if errors.Is(err, parser.ErrUnsupportedExt) {
				logger.Warn("compile: skipping unsupported file", "path", p, "err", err)
				continue
			}
			return nil, fmt.Errorf("failed to parse: %s: %w", p, err)
		}

		if t := o.temps[p]; t != nil {
			seedTemp(nodes, t)
		}

		roots = append(roots, nodes...)
	}

	if err := transformer.Transform(ctx, roots); err != nil {
		return nil, fmt.Errorf("failed to transform: %w", err)
	}

	flat := parser.Flatten(roots)

	prev, err := buildPrevState(ctx, st, flat)
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

	return countResult(deltas), nil
}

func seedTemp(nodes []*parser.ContextNode, t *float64) {
	for _, n := range nodes {
		n.Temperature = t
		seedTemp(n.Children, t)
	}
}

func CompileDir(ctx context.Context, st *store.Store, dir, message string, opts ...Option) (*Result, error) {
	var paths []string

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != dir && fileext.ShouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if fileext.Supported(path) {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk: %s: %w", dir, err)
	}

	if len(paths) == 0 {
		return &Result{}, nil
	}

	temps, err := resolveTemps(dir, paths)
	if err != nil {
		return nil, err
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve: %s: %w", dir, err)
	}

	// Caller-supplied opts come first so WithPaths/WithCompileRoot/WithTemps below win.
	all := append([]Option{}, opts...)
	all = append(all,
		WithPaths(paths),
		WithMessage(message),
		WithCompileRoot(absDir),
		WithTemps(temps),
	)
	return Compile(ctx, st, all...)
}

func resolveTemps(dir string, paths []string) (map[string]*float64, error) {
	resolver, err := tempfile.Load(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to load: %s: %w", tempfile.FileName, err)
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

func buildPrevState(ctx context.Context, st *store.Store, flat []*parser.ContextNode) (map[string]diff.NodeState, error) {
	files := uniqueFilesFlat(flat)

	existing, err := st.GetNodesByFiles(ctx, files)
	if err != nil {
		return nil, fmt.Errorf("failed to get nodes: %w", err)
	}

	prev := make(map[string]diff.NodeState, len(existing))
	for _, n := range existing {
		prev[n.ID] = diff.NodeState{Hash: n.ContentHash, Content: n.Content}
	}
	return prev, nil
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
