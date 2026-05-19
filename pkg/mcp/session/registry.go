// Package session tracks active MCP client sessions for the bound database.
package session

import (
	"context"
	"log/slog"
	"sort"
	"strconv"
	"sync"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/internal/contentid"
	"github.com/radimsem/remindb/internal/loghelper"
	"github.com/radimsem/remindb/pkg/mcp/ledger"
	"github.com/radimsem/remindb/pkg/mcp/sessionlog"
)

const (
	methodPing     = "ping"
	methodCallTool = "tools/call"
	transportHttp  = "http"
)

type SessionInfo struct {
	ID             string     `json:"id"`
	Client         ClientMeta `json:"client_meta"`
	Transport      string     `json:"transport"`
	Listen         string     `json:"listen,omitempty"`
	ConnectedAt    int64      `json:"connected_at"`
	LastActivity   int64      `json:"last_activity"`
	CountToolCalls int64      `json:"count_tool_calls"`
}

type ClientMeta struct {
	Name     string `json:"name"`
	Title    string `json:"title,omitempty"`
	Version  string `json:"version"`
	Protocol string `json:"protocol"`
}

type meta struct {
	id           string
	client       ClientMeta
	connectedAt  int64
	lastActivity int64
	toolCalls    int64
}

type Registry struct {
	srv       *gomcp.Server
	transport string
	listen    string
	ledger    *ledger.Ledger
	logger    *slog.Logger

	mu       sync.Mutex
	sessions map[*gomcp.ServerSession]*meta
}

type Option func(*options)

type options struct {
	transport string
	listen    string
	ledger    *ledger.Ledger
	logger    *slog.Logger
}

func WithTransport(t string) Option {
	return func(o *options) { o.transport = t }
}

func WithListen(addr string) Option {
	return func(o *options) { o.listen = addr }
}

func WithLedger(l *ledger.Ledger) Option {
	return func(o *options) { o.ledger = l }
}

func WithLogger(l *slog.Logger) Option {
	return func(o *options) { o.logger = l }
}

// NewRegistry tracks live sessions; the ledger (may be nil) persists their metrics.
func NewRegistry(srv *gomcp.Server, opts ...Option) *Registry {
	o := options{transport: "stdio"}
	for _, opt := range opts {
		opt(&o)
	}

	return &Registry{
		srv:       srv,
		transport: o.transport,
		listen:    o.listen,
		ledger:    o.ledger,
		logger:    loghelper.OrDiscard(o.logger),
		sessions:  make(map[*gomcp.ServerSession]*meta),
	}
}

// Middleware records connect/activity/tool-call signals on each inbound request.
func (r *Registry) Middleware(next gomcp.MethodHandler) gomcp.MethodHandler {
	return func(ctx context.Context, method string, req gomcp.Request) (gomcp.Result, error) {
		if ss, ok := req.GetSession().(*gomcp.ServerSession); ok {
			ctx = sessionlog.NewContext(ctx, r.observe(ss, method))
		}
		return next(ctx, method, req)
	}
}

// observe records connect/activity/tool-call signals and returns the canonical session id.
func (r *Registry) observe(ss *gomcp.ServerSession, method string) string {
	now := time.Now().Unix()

	r.mu.Lock()
	defer r.mu.Unlock()

	m := r.sessions[ss]
	if m == nil {
		m = &meta{connectedAt: now, lastActivity: now}
		r.sessions[ss] = m
	}

	// Refresh each call: client metadata is only populated after initialize.
	m.id = sessionID(ss.ID(), r.transport, m.connectedAt)
	m.client = clientMetaOf(ss)

	if method != methodPing {
		m.lastActivity = now
	}
	if method == methodCallTool {
		m.toolCalls++
	}

	return m.id
}

// Snapshot returns one SessionInfo per live SDK session, pruning stale entries.
func (r *Registry) Snapshot() []SessionInfo {
	now := time.Now().Unix()

	r.mu.Lock()
	defer r.mu.Unlock()

	var out []SessionInfo

	// Report only live sessions.
	for ss := range r.srv.Sessions() {
		m := r.sessions[ss]
		if m == nil {
			m = &meta{connectedAt: now, lastActivity: now}
			r.sessions[ss] = m
		}

		out = append(out, r.toInfo(ss.ID(), m, clientMetaOf(ss)))
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].ConnectedAt != out[j].ConnectedAt {
			return out[i].ConnectedAt < out[j].ConnectedAt
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func (r *Registry) toInfo(rawID string, m *meta, c ClientMeta) SessionInfo {
	info := SessionInfo{
		ID:             sessionID(rawID, r.transport, m.connectedAt),
		Client:         c,
		Transport:      r.transport,
		ConnectedAt:    m.connectedAt,
		LastActivity:   m.lastActivity,
		CountToolCalls: m.toolCalls,
	}
	if r.transport == transportHttp {
		info.Listen = r.listen
	}
	return info
}

func clientMetaOf(ss *gomcp.ServerSession) ClientMeta {
	ip := ss.InitializeParams()
	if ip == nil {
		return ClientMeta{}
	}

	c := ClientMeta{Protocol: ip.ProtocolVersion}
	if ip.ClientInfo != nil {
		c.Name = ip.ClientInfo.Name
		c.Title = ip.ClientInfo.Title
		c.Version = ip.ClientInfo.Version
	}
	return c
}

func sessionID(raw, transport string, connectedAt int64) string {
	if raw != "" {
		return raw
	}
	return contentid.IdentifyPayload("session", transport+strconv.FormatInt(connectedAt, 10))
}

// Run flushes the ledger on a ticker and once more on shutdown.
func (r *Registry) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			r.Flush(time.Now().Unix())
			return
		case <-t.C:
			r.Flush(time.Now().Unix())
		}
	}
}

// Flush appends a checkpoint line for every live session.
func (r *Registry) Flush(now int64) {
	r.mu.Lock()

	live := make(map[*gomcp.ServerSession]bool)
	for ss := range r.srv.Sessions() {
		live[ss] = true

		m := r.sessions[ss]
		if m == nil {
			m = &meta{connectedAt: now, lastActivity: now}
			r.sessions[ss] = m
		}

		m.id = sessionID(ss.ID(), r.transport, m.connectedAt)
		m.client = clientMetaOf(ss)
	}

	var records []ledger.Record
	for ss, m := range r.sessions {
		if live[ss] {
			records = append(records, r.record(m, 0))
			continue
		}

		if m.id != "" {
			records = append(records, r.record(m, m.lastActivity))
		}
		delete(r.sessions, ss)
	}

	r.mu.Unlock()

	if r.ledger == nil {
		return
	}
	for _, rec := range records {
		if err := r.ledger.Append(rec); err != nil {
			r.logger.Warn("failed to append: session ledger", "err", err)
		}
	}
}

func (r *Registry) record(m *meta, disconnectedAt int64) ledger.Record {
	return ledger.Record{
		SessionID:      m.id,
		Client:         ledger.ClientMeta(m.client),
		Transport:      r.transport,
		ConnectedAt:    m.connectedAt,
		LastSeen:       m.lastActivity,
		DisconnectedAt: disconnectedAt,
		ToolCalls:      m.toolCalls,
	}
}
