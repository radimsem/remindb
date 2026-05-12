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
	"Pods":             true,
	"__pycache__":      true,
	"bower_components": true,
	"dist":             true,
	"node_modules":     true,
	"target":           true,
	"vendor":           true,
	"venv":             true,
}

func Supported(path string) bool {
	return supported[strings.ToLower(filepath.Ext(path))]
}

// Report whether a directory should be skipped during recursive walks; dotfiles are always skipped except "." and "..".
func ShouldSkipDir(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	if strings.HasPrefix(name, ".") {
		return true
	}
	return skipDirs[name]
}
