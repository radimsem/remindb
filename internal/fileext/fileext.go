package fileext

import (
	"path/filepath"
	"strings"
)

var supported = map[string]bool{
	".md": true, ".markdown": true,
	".yaml": true, ".yml": true,
	".json":   true,
	".jsonl":  true,
	".ndjson": true,
	".toon":   true,
}

var skipDirs = map[string]bool{
	"node_modules": true,
}

func Supported(path string) bool {
	return supported[strings.ToLower(filepath.Ext(path))]
}

// Report whether a directory name should be skipped during recursive walks.
// Dotfiles (names starting with ".") are always skipped, except "." and "..".
func ShouldSkipDir(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	if strings.HasPrefix(name, ".") {
		return true
	}
	return skipDirs[name]
}
