package resources

import (
	"context"
	"encoding/json"
	"fmt"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/radimsem/remindb/pkg/inspect"
)

const OverviewURI = "remindb://overview"

type nodesEnvelope struct {
	Total  int            `json:"total"`
	ByType map[string]int `json:"by_type"`
	Tokens int64          `json:"tokens"`
}

type snapshotsEnvelope struct {
	Count         int    `json:"count"`
	HeadID        int64  `json:"head_id"`
	CursorHash    string `json:"cursor_hash"`
	LatestMessage string `json:"latest_message"`
	LatestAgeS    int64  `json:"latest_age_s"`
}

type temperatureEnvelope struct {
	Avg    float64 `json:"avg"`
	Median float64 `json:"median"`
	Hot    int     `json:"hot"`
	Cold   int     `json:"cold"`
	Pinned int     `json:"pinned"`
}

type relationsEnvelope struct {
	Total    int            `json:"total"`
	ByOrigin map[string]int `json:"by_origin"`
	Pending  int            `json:"pending"`
}

type overviewEnvelope struct {
	DBPath      string              `json:"db_path"`
	DBBytes     int64               `json:"db_bytes"`
	Nodes       nodesEnvelope       `json:"nodes"`
	Snapshots   snapshotsEnvelope   `json:"snapshots"`
	Temperature temperatureEnvelope `json:"temperature"`
	Relations   relationsEnvelope   `json:"relations"`
	FTSRows     int                 `json:"fts_rows"`
}

// Convert collected stats into the locked overview JSON envelope.
func newOverviewEnvelope(s *inspect.Stats) overviewEnvelope {
	relTotal := 0
	for _, v := range s.RelationsByOrigin {
		relTotal += v
	}

	env := overviewEnvelope{
		DBPath:  s.DBPath,
		DBBytes: s.DBSizeBytes,
		Nodes: nodesEnvelope{
			Total:  s.NodeCount,
			ByType: s.NodeCountsByType,
			Tokens: s.TokenCountTotal,
		},
		Snapshots: snapshotsEnvelope{Count: s.SnapshotCount},
		Temperature: temperatureEnvelope{
			Avg:    s.AvgTemp,
			Median: s.MedianTemp,
			Hot:    s.HotCount,
			Cold:   s.ColdCount,
			Pinned: s.PinnedCount,
		},
		Relations: relationsEnvelope{
			Total:    relTotal,
			ByOrigin: s.RelationsByOrigin,
			Pending:  s.PendingRelationCount,
		},
		FTSRows: s.FTSRowCount,
	}

	if s.Latest != nil {
		env.Snapshots.HeadID = s.Latest.ID
		env.Snapshots.CursorHash = s.Latest.CursorHash
		env.Snapshots.LatestMessage = s.Latest.Message
		env.Snapshots.LatestAgeS = s.Latest.AgeSeconds
	}

	return env
}

func (d *Deps) HandleOverview(ctx context.Context, _ *gomcp.ReadResourceRequest) (*gomcp.ReadResourceResult, error) {
	stats, err := inspect.Collect(ctx, d.Store)
	if err != nil {
		return nil, fmt.Errorf("failed to collect: stats: %w", err)
	}

	body, err := json.Marshal(newOverviewEnvelope(stats))
	if err != nil {
		return nil, fmt.Errorf("failed to marshal: overview: %w", err)
	}

	return &gomcp.ReadResourceResult{
		Contents: []*gomcp.ResourceContents{{
			URI:      OverviewURI,
			MIMEType: mimeJSON,
			Text:     string(body),
		}},
	}, nil
}
