package pathmatch

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/radimsem/remindb/pkg/config"
)

const (
	IgnoreFileName = "ignore"
	IgnorePath     = config.DirName + "/" + IgnoreFileName
)

// Read <dir>/.remindb/ignore; (nil, nil) if absent.
func LoadIgnore(dir string) (*Matcher, error) {
	f, err := os.Open(filepath.Join(dir, config.DirName, IgnoreFileName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read: %s: %w", IgnorePath, err)
	}
	defer func() { _ = f.Close() }()

	var patterns []pattern
	scanner := bufio.NewScanner(f)
	line := 0

	for scanner.Scan() {
		line++

		raw := strings.TrimSpace(scanner.Text())
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}

		p, err := parsePattern(raw)
		if err != nil {
			return nil, fmt.Errorf("unsupported pattern at line %d: %w", line, err)
		}
		patterns = append(patterns, p)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read: %s: %w", IgnorePath, err)
	}

	return &Matcher{patterns: patterns}, nil
}
