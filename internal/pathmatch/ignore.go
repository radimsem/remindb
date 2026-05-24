package pathmatch

import "github.com/radimsem/remindb/pkg/config"

const (
	IgnoreFileName = "ignore"
	IgnorePath     = config.DirName + "/" + IgnoreFileName
)

// Read <dir>/.remindb/ignore; (nil, nil) if absent.
func LoadIgnore(dir string) (*Matcher, error) {
	return loadMatcher(dir, IgnoreFileName, IgnorePath)
}
