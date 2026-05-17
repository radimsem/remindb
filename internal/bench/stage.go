package bench

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/radimsem/remindb/internal/fileext"
	"github.com/radimsem/remindb/internal/ignore"
	"github.com/radimsem/remindb/pkg/compiler"
	"github.com/radimsem/remindb/pkg/config"
	"github.com/radimsem/remindb/pkg/store"
)

type benchStage struct {
	dbPath  string
	srcDir  string
	tmpRoot string
}

func (s *benchStage) cleanup() {
	if s.tmpRoot == "" {
		return
	}
	_ = os.RemoveAll(s.tmpRoot)
}

// Copy the source tree into /tmp and fresh-compile it into an ephemeral DB so the baseline always matches the staged source.
func stageBench(ctx context.Context, sourceDir string) (*benchStage, error) {
	userDir, err := filepath.Abs(sourceDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve: %s: %w", sourceDir, err)
	}

	matcher, err := ignore.Load(userDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load: %s: %w", ignore.Path, err)
	}

	tmpRoot, err := os.MkdirTemp("", "remindb-bench-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create: tmp dir: %w", err)
	}
	stage := &benchStage{
		tmpRoot: tmpRoot,
		dbPath:  filepath.Join(tmpRoot, "memory.db"),
		srcDir:  filepath.Join(tmpRoot, "src"),
	}

	if err := copySourceTree(userDir, stage.srcDir, matcher); err != nil {
		stage.cleanup()
		return nil, err
	}

	if err := compileBaseline(ctx, stage.dbPath, stage.srcDir); err != nil {
		stage.cleanup()
		return nil, err
	}

	return stage, nil
}

// Compile with default options to match the bench MCP server (no --source → default config, no redactor) so the synthetic-change recompile diffs cleanly.
func compileBaseline(ctx context.Context, dbPath, srcDir string) error {
	st, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open: %s: %w", dbPath, err)
	}
	defer func() { _ = st.Close() }()

	if err := st.Migrate(ctx); err != nil {
		return fmt.Errorf("failed to migrate: %w", err)
	}
	if _, err := compiler.CompileDir(ctx, st, srcDir, "bench-baseline"); err != nil {
		return fmt.Errorf("failed to compile: baseline: %w", err)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	_, err = io.Copy(out, in)
	return err
}

// Mirror every parsable file from source dir into dst.
func copySourceTree(src, dst string, matcher *ignore.Matcher) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("failed to resolve: relative path for %s: %w", path, err)
		}
		relSlash := filepath.ToSlash(rel)

		if d.IsDir() {
			name := d.Name()
			if path != src && (fileext.ShouldSkipDir(name) || name == config.DirName) {
				return filepath.SkipDir
			}
			if matcher.Match(relSlash, true) {
				return filepath.SkipDir
			}
			return nil
		}

		if !fileext.Supported(path) {
			return nil
		}
		if matcher.Match(relSlash, false) {
			return nil
		}

		target := filepath.Join(dst, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("failed to create: %s: %w", filepath.Dir(target), err)
		}

		return copyFile(path, target)
	})
}
