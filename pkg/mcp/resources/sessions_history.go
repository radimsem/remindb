package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/pkg/mcp/ledger"
)

const (
	SessionsHistoryURI            = "remindb://sessions/history"
	SessionsHistoryByHashTemplate = "remindb://sessions/history/{hash}"
)

type sessionsHistoryEnvelope struct {
	DBPath  string                `json:"db_path"`
	Clients []ledger.ClientLedger `json:"clients"`
}

func (d *Deps) HandleSessionsHistory(ctx context.Context, _ *gomcp.ReadResourceRequest) (*gomcp.ReadResourceResult, error) {
	env := sessionsHistoryEnvelope{Clients: []ledger.ClientLedger{}}
	if d.Store != nil {
		env.DBPath = d.Store.Path
	}

	if d.Ledger != nil {
		clients, err := d.Ledger.Clients()
		if err != nil {
			return nil, fmt.Errorf("failed to read: session ledger: %w", err)
		}

		if clients != nil {
			env.Clients = clients
		}
	}

	body, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal: sessions history: %w", err)
	}

	return &gomcp.ReadResourceResult{
		Contents: []*gomcp.ResourceContents{{
			URI:      SessionsHistoryURI,
			MIMEType: mimeJSON,
			Text:     string(body),
		}},
	}, nil
}

func (d *Deps) HandleSessionsHistoryByHash(ctx context.Context, req *gomcp.ReadResourceRequest) (*gomcp.ReadResourceResult, error) {
	u, err := url.Parse(req.Params.URI)
	if err != nil {
		return nil, fmt.Errorf("failed to parse: sessions history uri: %w", err)
	}

	hash := strings.TrimPrefix(u.Path, "/history/")
	if hash == "" || strings.Contains(hash, "/") {
		return nil, fmt.Errorf("sessions history uri missing client hash")
	}

	if d.Ledger == nil {
		return nil, fmt.Errorf("unknown client %q", hash)
	}

	cl, err := d.Ledger.Client(hash)
	if err != nil {
		return nil, fmt.Errorf("failed to read: session ledger: %w", err)
	}

	body, err := json.Marshal(cl)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal: sessions history: %w", err)
	}

	return &gomcp.ReadResourceResult{
		Contents: []*gomcp.ResourceContents{{
			URI:      req.Params.URI,
			MIMEType: mimeJSON,
			Text:     string(body),
		}},
	}, nil
}
