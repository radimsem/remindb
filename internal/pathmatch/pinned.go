package pathmatch

import "github.com/radimsem/remindb/pkg/config"

const (
	PinnedFileName = "pinned"
	PinnedPath     = config.DirName + "/" + PinnedFileName
)

// Read <dir>/.remindb/pinned; (nil, nil) if absent.
func LoadPinned(dir string) (*Matcher, error) {
	return loadMatcher(dir, PinnedFileName, PinnedPath)
}
