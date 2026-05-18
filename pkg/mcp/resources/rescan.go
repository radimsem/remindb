package resources

import (
	"context"
	"encoding/json"
	"fmt"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/pkg/mcp/rescanstat"
)

const RescanURI = "remindb://rescan"

type rescanEnvelope struct {
	IntervalS int64               `json:"interval_s"`
	LastMeta  rescanstat.Snapshot `json:"last_meta"`
}

func (d *Deps) HandleRescan(ctx context.Context, _ *gomcp.ReadResourceRequest) (*gomcp.ReadResourceResult, error) {
	var env rescanEnvelope
	if d.RescanStatus != nil {
		env.IntervalS, env.LastMeta = d.RescanStatus.Get()
	}
	if env.LastMeta.PurgedFiles == nil {
		env.LastMeta.PurgedFiles = []rescanstat.PurgedFile{}
	}

	body, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal: rescan: %w", err)
	}

	return &gomcp.ReadResourceResult{
		Contents: []*gomcp.ResourceContents{{
			URI:      RescanURI,
			MIMEType: mimeJSON,
			Text:     string(body),
		}},
	}, nil
}
