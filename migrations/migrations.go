// Hold the embed.FS handle so other packages can read the migrations at runtime.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
