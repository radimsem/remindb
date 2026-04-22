package mcp

import (
	"context"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/pkg/mcp/tools"
	"github.com/radimsem/remindb/pkg/query"
	"github.com/radimsem/remindb/pkg/store"
	"github.com/radimsem/remindb/pkg/temperature"
)

type Server struct {
	mcp *mcp.Server
}

func NewServer(st *store.Store, tracker *temperature.Tracker, logger *slog.Logger) *Server {
	s := &Server{
		mcp: mcp.NewServer(&mcp.Implementation{
			Name:    "remindb",
			Version: "0.1.0",
		}, nil),
	}

	deps := &tools.Deps{
		Store:   st,
		Engine:  query.NewEngine(st),
		Tracker: tracker,
		Logger:  logger,
	}

	registerTools(s.mcp, deps)
	return s
}

func (s *Server) Run(ctx context.Context) error {
	return s.mcp.Run(ctx, &mcp.StdioTransport{})
}

func (s *Server) Connect(ctx context.Context, t mcp.Transport) (*mcp.ServerSession, error) {
	return s.mcp.Connect(ctx, t, nil)
}

func registerTools(srv *mcp.Server, d *tools.Deps) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "MemoryFetch",
		Description: "Retrieve context around an anchor node within a token budget",
	}, d.HandleFetch)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "MemorySearch",
		Description: "Full-text search for nodes within a token budget",
	}, d.HandleSearch)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "MemoryWrite",
		Description: "Write or update content at an anchor node, creating a snapshot",
	}, d.HandleWrite)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "MemoryCompile",
		Description: "Compile source files or a directory into the memory database",
	}, d.HandleCompile)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "MemoryDelta",
		Description: "Return changes since a given snapshot",
	}, d.HandleDelta)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "MemorySummarize",
		Description: "Replace a node's content with a provided summary",
	}, d.HandleSummarize)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "MemoryHistory",
		Description: "Browse version history for a specific node",
	}, d.HandleHistory)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "MemoryTree",
		Description: "Return the node tree structure with labels",
	}, d.HandleTree)
}
