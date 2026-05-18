package resources

import (
	"context"
	"encoding/json"
	"fmt"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/pkg/doctor"
)

const DoctorURI = "remindb://doctor"

func (d *Deps) HandleDoctor(ctx context.Context, _ *gomcp.ReadResourceRequest) (*gomcp.ReadResourceResult, error) {
	report := doctor.Run(ctx, d.Store)

	body, err := json.Marshal(report)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal: doctor: %w", err)
	}

	return &gomcp.ReadResourceResult{
		Contents: []*gomcp.ResourceContents{{
			URI:      DoctorURI,
			MIMEType: mimeJSON,
			Text:     string(body),
		}},
	}, nil
}
