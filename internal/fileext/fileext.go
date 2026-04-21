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

func Supported(path string) bool {
	return supported[strings.ToLower(filepath.Ext(path))]
}
