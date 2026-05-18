package resources

import (
	"context"
	"encoding/json"
	"fmt"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/pkg/mcp/session"
)

const SessionsURI = "remindb://sessions"

type sessionsEnvelope struct {
	DBPath   string                `json:"db_path"`
	Sessions []session.SessionInfo `json:"sessions"`
}

func (d *Deps) HandleSessions(ctx context.Context, _ *gomcp.ReadResourceRequest) (*gomcp.ReadResourceResult, error) {
	env := sessionsEnvelope{Sessions: []session.SessionInfo{}}
	if d.Store != nil {
		env.DBPath = d.Store.Path
	}
	if d.Sessions != nil {
		if s := d.Sessions.Snapshot(); s != nil {
			env.Sessions = s
		}
	}

	body, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal: sessions: %w", err)
	}

	return &gomcp.ReadResourceResult{
		Contents: []*gomcp.ResourceContents{{
			URI:      SessionsURI,
			MIMEType: mimeJSON,
			Text:     string(body),
		}},
	}, nil
}
