package version

import "runtime/debug"

// Override is the build-time injected version (takes precedence over BuildInfo).
var Override = ""

func Get() string {
	if Override != "" {
		return Override
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return "dev"
}
