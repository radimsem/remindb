package bench

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/radimsem/remindb/internal/fileext"
	"github.com/radimsem/remindb/internal/ignore"
	"github.com/radimsem/remindb/pkg/store"
)

type benchStage struct {
	dbPath  string
	srcDir  string
	userDir string
	tmpRoot string
}

func (s *benchStage) cleanup() {
	if s.tmpRoot == "" {
		return
	}
	_ = os.RemoveAll(s.tmpRoot)
}

// Copy the live DB and source directory into /tmp and rewrite the source file paths
func stageBench(ctx context.Context, userDBPath, overrideDir string) (*benchStage, error) {
	userDir, err := resolveSourceDir(ctx, userDBPath, overrideDir)
	if err != nil {
		return nil, err
	}

	matcher, err := ignore.Load(userDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load: %s: %w", ignore.FileName, err)
	}

	tmpRoot, err := os.MkdirTemp("", "remindb-bench-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create: tmp dir: %w", err)
	}
	stage := &benchStage{
		tmpRoot: tmpRoot,
		userDir: userDir,
		dbPath:  filepath.Join(tmpRoot, "memory.db"),
		srcDir:  filepath.Join(tmpRoot, "src"),
	}

	if err := copyDBFiles(userDBPath, stage.dbPath); err != nil {
		stage.cleanup()
		return nil, err
	}

	if err := copySourceTree(userDir, stage.srcDir, matcher); err != nil {
		stage.cleanup()
		return nil, err
	}

	if err := rewriteSourcePaths(ctx, stage.dbPath, userDir, stage.srcDir); err != nil {
		stage.cleanup()
		return nil, err
	}

	return stage, nil
}

// Pick the source directory to bench against.
func resolveSourceDir(ctx context.Context, userDBPath, override string) (string, error) {
	if override != "" {
		abs, err := filepath.Abs(override)
		if err != nil {
			return "", fmt.Errorf("failed to resolve: %s: %w", override, err)
		}
		return abs, nil
	}

	st, err := store.Open(userDBPath)
	if err != nil {
		return "", fmt.Errorf("failed to open: %s: %w", userDBPath, err)
	}
	defer func() { _ = st.Close() }()

	root, err := st.GetLatestCompileRoot(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to read: compile_root: %w", err)
	}
	if root == "" {
		return "", fmt.Errorf("no compile_root recorded in db (pre-migration snapshot or write-only history); pass --dir")
	}
	return root, nil
}

// Copy the main DB file plus any adjacent WAL/SHM companions.
func copyDBFiles(src, dst string) error {
	if err := copyFile(src, dst); err != nil {
		return fmt.Errorf("failed to copy: %s: %w", src, err)
	}

	for _, suffix := range []string{"-wal", "-shm"} {
		from := src + suffix
		if _, err := os.Stat(from); err != nil {
			continue
		}
		if err := copyFile(from, dst+suffix); err != nil {
			return fmt.Errorf("failed to copy: %s: %w", from, err)
		}
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
			if path != src && fileext.ShouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			if matcher.Match(relSlash, true) {
				return filepath.SkipDir
			}
			return nil
		}
		if relSlash == ignore.FileName {
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

// Update every source file row so the staged DB matches the staged source directory.
func rewriteSourcePaths(ctx context.Context, dbPath, userDir, stagedDir string) error {
	st, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open: %s: %w", dbPath, err)
	}
	defer func() { _ = st.Close() }()

	if err := st.ExecRewriteSourcePaths(ctx, userDir, stagedDir); err != nil {
		return fmt.Errorf("failed to rewrite: source paths: %w", err)
	}
	return nil
}
