package compiler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/radimsem/remindb/internal/fileext"
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

	flat := parser.Flatten(roots)

	prev, err := buildPrevState(ctx, st, flat)
	if err != nil {
		return nil, err
	}

	deltas := diff.DiffFlat(flat, prev)
	cursorHash := diff.CursorHashFlat(flat)

	if err := emitter.Emit(ctx, st, roots, deltas, cursorHash, message); err != nil {
		return nil, fmt.Errorf("failed to emit: %w", err)
	}

	return countResult(flat, deltas), nil
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
	return Compile(ctx, st, paths, message)
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

func countResult(flat []*parser.ContextNode, deltas []diff.Delta) *Result {
	r := &Result{Total: len(flat)}

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
	return r
}
