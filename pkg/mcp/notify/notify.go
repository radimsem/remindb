// Package notify coalesces resource-change events into debounced notifications/resources/updated, keyed per resource URI.
package notify

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	mcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/pkg/config"
	"github.com/radimsem/remindb/pkg/mcp/resources"
)

const DefaultDebounce = 500 * time.Millisecond

// High-frequency streams get a longer floor than the global default unless the workspace config overrides them explicitly.
var builtinOverrides = map[string]time.Duration{
	"logs":        time.Second,
	"temperature": 2 * time.Second,
}

type Publisher struct {
	srv    *mcp.Server
	perURI map[string]time.Duration
	logger *slog.Logger

	mu     sync.Mutex
	timers map[string]*time.Timer
}

func NewPublisher(cfg config.ResourcesConfig, logger *slog.Logger) (*Publisher, error) {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	def := DefaultDebounce
	if cfg.Debounce != nil {
		def = time.Duration(*cfg.Debounce)
	}

	perURI := make(map[string]time.Duration, len(resources.Subscribable))
	for name, uri := range resources.Subscribable {
		d := def
		if b, ok := builtinOverrides[name]; ok {
			d = b
		}

		perURI[uri] = d
	}

	for name, d := range cfg.Overrides {
		uri, ok := resources.Subscribable[name]
		if !ok {
			return nil, fmt.Errorf("server.resources.overrides: unknown resource %q", name)
		}

		perURI[uri] = time.Duration(*d)
	}

	return &Publisher{
		perURI: perURI,
		logger: logger,
		timers: make(map[string]*time.Timer, len(perURI)),
	}, nil
}

// Attach binds the MCP server used to send notifications. Call once, before the first Touch.
func (p *Publisher) Attach(srv *mcp.Server) {
	p.srv = srv
}

// Touch records that uri changed.
func (p *Publisher) Touch(uri string) {
	d, ok := p.perURI[uri]
	if !ok {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if t := p.timers[uri]; t != nil {
		t.Reset(d)
		return
	}
	p.timers[uri] = time.AfterFunc(d, func() { p.fire(uri) })
}

func (p *Publisher) fire(uri string) {
	p.mu.Lock()
	delete(p.timers, uri)
	p.mu.Unlock()

	if p.srv == nil {
		return
	}

	params := &mcp.ResourceUpdatedNotificationParams{URI: uri}
	if err := p.srv.ResourceUpdated(context.Background(), params); err != nil {
		p.logger.Warn("failed to send: resource updated notification", "uri", uri, "err", err)
	}
}

func (p *Publisher) HandleSubscribe(_ context.Context, req *mcp.SubscribeRequest) error {
	if _, ok := p.perURI[req.Params.URI]; !ok {
		return fmt.Errorf("resource %q is not subscribable", req.Params.URI)
	}
	return nil
}

// HandleUnsubscribe is a no-op: the SDK owns subscription state and ResourceUpdated already skips unsubscribed sessions.
func (p *Publisher) HandleUnsubscribe(_ context.Context, _ *mcp.UnsubscribeRequest) error {
	return nil
}

func (p *Publisher) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for uri, t := range p.timers {
		t.Stop()
		delete(p.timers, uri)
	}
}
