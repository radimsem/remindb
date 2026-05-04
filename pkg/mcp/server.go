package mcp

import (
	"context"
	"log/slog"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/pkg/mcp/tools"
	"github.com/radimsem/remindb/pkg/query"
	"github.com/radimsem/remindb/pkg/store"
	"github.com/radimsem/remindb/pkg/temperature"
)

type Server struct {
	mcp             *mcp.Server
	logger          *slog.Logger
	coldThreshold   float64
	notifyThreshold float64

	mu       sync.Mutex
	notified map[string]struct{}
}

func NewServer(st *store.Store, tracker *temperature.Tracker, cfg temperature.Config, sourceDir string, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	s := &Server{
		mcp: mcp.NewServer(&mcp.Implementation{
			Name:    "remindb",
			Version: "0.1.0",
		}, nil),
		logger:          logger,
		coldThreshold:   cfg.ColdThreshold,
		notifyThreshold: cfg.NotifyThreshold,
		notified:        make(map[string]struct{}),
	}

	deps := &tools.Deps{
		Store:     st,
		Engine:    query.NewEngine(st),
		Tracker:   tracker,
		Logger:    logger,
		SourceDir: sourceDir,
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

func (s *Server) NotifyColdNodes(ctx context.Context, cold []*store.Node) {
	toNotify := s.coldCandidates(cold)
	if len(toNotify) == 0 {
		return
	}

	if sent := s.sendColdLogging(ctx, toNotify); sent > 0 {
		s.markNotified(toNotify)
	}
}

// Garbage-collect notified IDs no longer cold, then return cold nodes pending notification.
func (s *Server) coldCandidates(cold []*store.Node) []*store.Node {
	s.mu.Lock()
	defer s.mu.Unlock()

	stillCold := make(map[string]struct{}, len(cold))
	for _, n := range cold {
		stillCold[n.ID] = struct{}{}
	}
	for id := range s.notified {
		if _, ok := stillCold[id]; !ok {
			delete(s.notified, id)
		}
	}

	out := make([]*store.Node, 0, len(cold))
	for _, n := range cold {
		if n.Temperature >= s.notifyThreshold {
			continue
		}
		if _, seen := s.notified[n.ID]; seen {
			continue
		}
		out = append(out, n)
	}
	return out
}

func (s *Server) markNotified(nodes []*store.Node) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, n := range nodes {
		s.notified[n.ID] = struct{}{}
	}
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
