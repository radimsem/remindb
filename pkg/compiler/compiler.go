package compiler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

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

var supportedExts = map[string]bool{
	".md": true, ".yaml": true, ".yml": true, ".json": true, ".toon": true,
}

func Compile(ctx context.Context, st *store.Store, paths []string, message string) (*Result, error) {
	var roots []*parser.ContextNode
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("failed to read: %s: %w", p, err)
		}

		nodes, err := parser.ParseBytes(p, data)
		if err != nil {
			if errors.Is(err, parser.ErrUnsupportedExt) {
				log.Printf("skipping %s: %v", p, err)
				continue
			}
			return nil, fmt.Errorf("failed to parse: %s: %w", p, err)
		}
		roots = append(roots, nodes...)
	}

	if err := transformer.Transform(ctx, roots); err != nil {
		return nil, fmt.Errorf("failed to transform: %w", err)
	}

	prev, err := buildPrevState(ctx, st, roots)
	if err != nil {
		return nil, err
	}

	deltas := diff.Diff(roots, prev)
	cursorHash := diff.CursorHash(roots)

	if err := emitter.Emit(ctx, st, roots, deltas, cursorHash, message); err != nil {
		return nil, fmt.Errorf("failed to emit: %w", err)
	}

	return countStats(roots, deltas), nil
}

func CompileDir(ctx context.Context, st *store.Store, dir, message string) (*Result, error) {
	var paths []string

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if supportedExts[filepath.Ext(path)] {
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
	return Compile(ctx, st, paths, message)
}

func buildPrevState(ctx context.Context, st *store.Store, roots []*parser.ContextNode) (map[string]diff.NodeState, error) {
	files := uniqueFiles(roots)
	prev := make(map[string]diff.NodeState)

	for _, f := range files {
		existing, err := st.GetNodesByFile(ctx, f)
		if err != nil {
			return nil, fmt.Errorf("failed to get nodes: %s: %w", f, err)
		}
		for _, n := range existing {
			prev[n.ID] = diff.NodeState{Hash: n.ContentHash, Content: n.Content}
		}
	}
	return prev, nil
}

func uniqueFiles(roots []*parser.ContextNode) []string {
	seen := make(map[string]bool)
	var out []string

	var walk func([]*parser.ContextNode)
	walk = func(nodes []*parser.ContextNode) {
		for _, n := range nodes {
			if !seen[n.SourceFile] {
				seen[n.SourceFile] = true
				out = append(out, n.SourceFile)
			}
			walk(n.Children)
		}
	}
	walk(roots)
	return out
}

func countStats(roots []*parser.ContextNode, deltas []diff.Delta) *Result {
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

	var count func([]*parser.ContextNode)
	count = func(nodes []*parser.ContextNode) {
		for _, n := range nodes {
			r.Total++
			count(n.Children)
		}
	}
	count(roots)
	return r
}
