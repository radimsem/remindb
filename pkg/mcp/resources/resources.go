// Package resources serves read-only MCP resources.
package resources

import (
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/pkg/store"
)

const mimeJSON = "application/json"

type Deps struct {
	Store *store.Store
}

// Register every read-only resource on the server.
func Register(srv *gomcp.Server, d *Deps) {
	srv.AddResource(&gomcp.Resource{
		Name:        "overview",
		URI:         OverviewURI,
		MIMEType:    mimeJSON,
		Description: "Database introspection overview — node, snapshot, temperature, and relation counts as stable JSON. Passive read: does not boost temperature or create a snapshot.",
	}, d.HandleOverview)

	srv.AddResource(&gomcp.Resource{
		Name:        "files",
		URI:         FilesURI,
		MIMEType:    mimeJSON,
		Description: "Compiled source files grouped by compile root, with per-file node and token counts as stable JSON — the JSON twin of `remindb inspect --files`. Passive read: does not boost temperature or create a snapshot.",
	}, d.HandleFiles)
}
