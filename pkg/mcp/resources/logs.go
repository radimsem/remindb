package resources

import (
	"context"
	"encoding/json"
	"fmt"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/pkg/logbuf"
)

const LogsURI = "remindb://logs"

type logsEnvelope struct {
	Records []logbuf.Record `json:"records"`
	Dropped int64           `json:"dropped"`
}

func (d *Deps) HandleLogs(ctx context.Context, _ *gomcp.ReadResourceRequest) (*gomcp.ReadResourceResult, error) {
	env := logsEnvelope{Records: []logbuf.Record{}}
	if d.LogBuffer != nil {
		env.Records = d.LogBuffer.Records()
		env.Dropped = d.LogBuffer.Dropped()
	}

	body, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal: logs: %w", err)
	}

	return &gomcp.ReadResourceResult{
		Contents: []*gomcp.ResourceContents{{
			URI:      LogsURI,
			MIMEType: mimeJSON,
			Text:     string(body),
		}},
	}, nil
}
