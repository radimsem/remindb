// Package resources serves read-only MCP resources.
package resources

import (
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/pkg/logbuf"
	"github.com/radimsem/remindb/pkg/store"
)

const mimeJSON = "application/json"

type Deps struct {
	Store         *store.Store
	ColdThreshold float64
	LogBuffer     *logbuf.Buffer
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

	srv.AddResource(&gomcp.Resource{
		Name:        "tree",
		URI:         TreeURI,
		MIMEType:    mimeJSON,
		Description: "Full parent/child node hierarchy as nested JSON — the structured twin of `MemoryTree`'s text. Passive read: does not boost temperature or create a snapshot.",
	}, d.HandleTree)

	srv.AddResource(&gomcp.Resource{
		Name:        "graph",
		URI:         GraphURI,
		MIMEType:    mimeJSON,
		Description: "Relations knowledge graph — resolved edges, pending/unresolved edges, and the referenced node set as stable JSON, for a UI drawing the brain graph. Passive read: does not boost temperature or create a snapshot.",
	}, d.HandleGraph)

	srv.AddResourceTemplate(&gomcp.ResourceTemplate{
		Name:        "tree-by-root",
		URITemplate: TreeByRootTemplate,
		MIMEType:    mimeJSON,
		Description: "Node hierarchy rooted at {rootId}, optionally depth-bounded via ?depth=N (omitted = full subtree). Passive read: does not boost temperature or create a snapshot.",
	}, d.HandleTreeByRoot)

	srv.AddResource(&gomcp.Resource{
		Name:        "snapshots",
		URI:         SnapshotsURI,
		MIMEType:    mimeJSON,
		Description: "Version history — every snapshot newest-first with parent links, compile root, and the HEAD marker, as stable JSON for an interactive timeline. Mirrors MemoryHistory. Passive read: does not boost temperature or create a snapshot.",
	}, d.HandleSnapshots)

	srv.AddResourceTemplate(&gomcp.ResourceTemplate{
		Name:        "snapshots-limited",
		URITemplate: SnapshotsLimitTemplate,
		MIMEType:    mimeJSON,
		Description: "Version history bounded to the newest ?limit=N snapshots (omitted = full history). Passive read: does not boost temperature or create a snapshot.",
	}, d.HandleSnapshotsLimited)

	srv.AddResource(&gomcp.Resource{
		Name:        "temperature",
		URI:         TemperatureURI,
		MIMEType:    mimeJSON,
		Description: "Per-node temperature for a heatmap — every node's id, label, temperature, and pinned flag in one array, plus an aggregate summary with the cold/hot cut points. Passive read: does not boost temperature or create a snapshot.",
	}, d.HandleTemperature)

	srv.AddResourceTemplate(&gomcp.ResourceTemplate{
		Name:        "snapshot-diffs",
		URITemplate: SnapshotDiffsTemplate,
		MIMEType:    mimeJSON,
		Description: "Per-snapshot diff records for snapshot {id} (op, node_id, old/new hash + content), the data behind MemoryDelta. Passive read: does not boost temperature or create a snapshot.",
	}, d.HandleSnapshotDiffs)

	srv.AddResource(&gomcp.Resource{
		Name:        "doctor",
		URI:         DoctorURI,
		MIMEType:    mimeJSON,
		Description: "Health-check report — overall worst-wins status header plus every check's name/status/detail as stable JSON, byte-equivalent to `remindb doctor --json`. Read-only (never applies `--fix`). Passive read: does not boost temperature or create a snapshot.",
	}, d.HandleDoctor)

	srv.AddResource(&gomcp.Resource{
		Name:        "logs",
		URI:         LogsURI,
		MIMEType:    mimeJSON,
		Description: "Recent server log records from the in-memory ring buffer, newest last, with an overflow count, as stable JSON for a log console. Passive read: does not boost temperature or create a snapshot.",
	}, d.HandleLogs)
}
