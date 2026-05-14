package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/pkg/mcp/tools"
	"github.com/radimsem/remindb/pkg/query"
	"github.com/radimsem/remindb/pkg/relations"
	"github.com/radimsem/remindb/pkg/store"
	"github.com/radimsem/remindb/pkg/temperature"
	"github.com/radimsem/remindb/pkg/version"
)

const (
	TransportStdio = "stdio"
	TransportHttp  = "http"

	DefaultListenAddr = "127.0.0.1:7474"
)

type Server struct {
	mcp             *mcp.Server
	logger          *slog.Logger
	notifyThreshold float64
	transport       string
	listen          string
	listener        net.Listener
}

type Option func(*options)

type options struct {
	sourceDir string
	logger    *slog.Logger
	transport string
	listen    string
	listener  net.Listener
}

func WithSourceDir(dir string) Option {
	return func(o *options) { o.sourceDir = dir }
}

func WithLogger(l *slog.Logger) Option {
	return func(o *options) { o.logger = l }
}

func WithTransport(t string) Option {
	return func(o *options) { o.transport = t }
}

func WithListen(addr string) Option {
	return func(o *options) { o.listen = addr }
}

func WithListener(l net.Listener) Option {
	return func(o *options) { o.listener = l }
}

func NewServer(st *store.Store, tracker *temperature.Tracker, cfg temperature.Config, opts ...Option) *Server {
	var o options
	for _, opt := range opts {
		opt(&o)
	}

	logger := o.logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	transport := o.transport
	if transport == "" {
		transport = TransportStdio
	}

	listen := o.listen
	if listen == "" {
		listen = DefaultListenAddr
	}

	s := &Server{
		mcp: mcp.NewServer(&mcp.Implementation{
			Name:    "remindb",
			Version: version.Get(),
		}, nil),
		logger:          logger,
		notifyThreshold: cfg.NotifyThreshold,
		transport:       transport,
		listen:          listen,
		listener:        o.listener,
	}

	deps := &tools.Deps{
		Store:            st,
		Engine:           query.NewEngine(st),
		Resolver:         relations.New(st),
		Tracker:          tracker,
		Logger:           logger,
		SourceDir:        o.sourceDir,
		SummarizeRebound: cfg.SummarizeRebound,
	}

	registerTools(s.mcp, deps)
	return s
}

func (s *Server) Run(ctx context.Context) error {
	switch s.transport {
	case TransportStdio:
		return s.mcp.Run(ctx, &mcp.StdioTransport{})
	case TransportHttp:
		return s.runHttp(ctx)
	default:
		return fmt.Errorf("unsupported transport %q", s.transport)
	}
}

func (s *Server) Connect(ctx context.Context, t mcp.Transport) (*mcp.ServerSession, error) {
	return s.mcp.Connect(ctx, t, nil)
}

// Send a cold-node warning and return the IDs that reached at least one session.
func (s *Server) NotifyColdNodes(ctx context.Context, cold []*store.Node) []string {
	toNotify := make([]*store.Node, 0, len(cold))
	for _, n := range cold {
		if n.Temperature < s.notifyThreshold {
			toNotify = append(toNotify, n)
		}
	}
	if len(toNotify) == 0 {
		return nil
	}

	if sent := s.sendColdLogging(ctx, toNotify); sent == 0 {
		return nil
	}

	ids := make([]string, len(toNotify))
	for i, n := range toNotify {
		ids[i] = n.ID
	}
	return ids
}

type coldNodeEntry struct {
	ID          string  `json:"id"`
	Label       string  `json:"label"`
	SourceFile  string  `json:"file"`
	Temperature float64 `json:"temperature"`
}

type coldNodePayload struct {
	Message         string          `json:"message"`
	SuggestedAction string          `json:"suggested_action"`
	Nodes           []coldNodeEntry `json:"nodes"`
}

func (s *Server) sendColdLogging(ctx context.Context, nodes []*store.Node) int {
	entries := make([]coldNodeEntry, len(nodes))
	for i, n := range nodes {
		entries[i] = coldNodeEntry{
			ID:          n.ID,
			Label:       n.Label,
			SourceFile:  n.SourceFile,
			Temperature: n.Temperature,
		}
	}

	params := &mcp.LoggingMessageParams{
		Level:  "warning",
		Logger: "remindb.temperature",
		Data: coldNodePayload{
			Message:         "Cold nodes detected; consider summarizing via MemorySummarize",
			SuggestedAction: "MemorySummarize",
			Nodes:           entries,
		},
	}

	sent := 0
	for ss := range s.mcp.Sessions() {
		if err := ss.Log(ctx, params); err != nil {
			s.logger.Warn("failed to send: cold-node notification", "err", err)
			continue
		}
		sent++
	}

	s.logger.Debug("cold-node notification dispatched", "nodes", len(nodes), "sessions", sent)
	return sent
}

func registerTools(srv *mcp.Server, d *tools.Deps) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "MemoryFetch",
		Description: "Retrieve context around an anchor node within a token budget",
	}, d.HandleFetch)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "MemoryFetchBatch",
		Description: "Retrieve content for a list of node IDs in one call (shared token budget; missing IDs and over-budget IDs surfaced inline)",
	}, d.HandleFetchBatch)

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

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "MemoryRelated",
		Description: "Traverse the relations graph from an anchor node",
	}, d.HandleRelated)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "MemoryRelate",
		Description: "Create a manual edge from one node to another (does not snapshot)",
	}, d.HandleRelate)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "MemoryPin",
		Description: "Protect a node from temperature decay and cold-set selection (does not snapshot)",
	}, d.HandlePin)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "MemoryUnpin",
		Description: "Release a previously pinned node back into the temperature lifecycle (does not snapshot)",
	}, d.HandleUnpin)
}
