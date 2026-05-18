// Package session tracks active MCP client sessions for the bound database.
package session

import (
	"context"
	"sort"
	"strconv"
	"sync"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/internal/contentid"
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
	connectedAt  int64
	lastActivity int64
	toolCalls    int64
}

type Registry struct {
	srv       *gomcp.Server
	transport string
	listen    string

	mu       sync.Mutex
	sessions map[*gomcp.ServerSession]*meta
}

func NewRegistry(srv *gomcp.Server, transport, listen string) *Registry {
	return &Registry{
		srv:       srv,
		transport: transport,
		listen:    listen,
		sessions:  make(map[*gomcp.ServerSession]*meta),
	}
}

// Middleware records connect/activity/tool-call signals on each inbound request.
func (r *Registry) Middleware(next gomcp.MethodHandler) gomcp.MethodHandler {
	return func(ctx context.Context, method string, req gomcp.Request) (gomcp.Result, error) {
		if ss, ok := req.GetSession().(*gomcp.ServerSession); ok {
			r.observe(ss, method)
		}
		return next(ctx, method, req)
	}
}

func (r *Registry) observe(ss *gomcp.ServerSession, method string) {
	now := time.Now().Unix()

	r.mu.Lock()
	defer r.mu.Unlock()

	m := r.sessions[ss]
	if m == nil {
		m = &meta{connectedAt: now, lastActivity: now}
		r.sessions[ss] = m
	}

	if method != methodPing {
		m.lastActivity = now
	}
	if method == methodCallTool {
		m.toolCalls++
	}
}

// Snapshot returns one SessionInfo per live SDK session, pruning stale entries.
func (r *Registry) Snapshot() []SessionInfo {
	now := time.Now().Unix()

	r.mu.Lock()
	defer r.mu.Unlock()

	seen := make(map[*gomcp.ServerSession]bool)
	var out []SessionInfo

	for ss := range r.srv.Sessions() {
		seen[ss] = true

		m := r.sessions[ss]
		if m == nil {
			m = &meta{connectedAt: now, lastActivity: now}
			r.sessions[ss] = m
		}

		out = append(out, r.toInfo(ss.ID(), m, clientMetaOf(ss)))
	}

	for ss := range r.sessions {
		if !seen[ss] {
			delete(r.sessions, ss)
		}
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
