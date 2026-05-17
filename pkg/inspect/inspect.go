// Package inspect aggregates database statistics for the CLI and MCP surfaces.
package inspect

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/radimsem/remindb/pkg/store"
)

type LatestSnapshot struct {
	ID         int64
	CursorHash string
	Message    string
	AgeSeconds int64
}

type Stats struct {
	DBPath               string
	DBSizeBytes          int64
	NodeCount            int
	SnapshotCount        int
	AvgTemp              float64
	MedianTemp           float64
	HotCount             int
	ColdCount            int
	PinnedCount          int
	TokenCountTotal      int64
	FTSRowCount          int
	NodeCountsByType     map[string]int
	RelationCount        int
	RelationsByOrigin    map[string]int
	PendingRelationCount int
	Latest               *LatestSnapshot
}

// Collect every stat surfaced by inspect and MemoryStats from the given store.
func Collect(ctx context.Context, st *store.Store) (*Stats, error) {
	core, err := st.GetStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get: stats: %w", err)
	}

	byType, err := st.GetNodeCountsByType(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get: node counts: %w", err)
	}

	byOrigin, err := st.GetRelationCountsByOrigin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get: relation counts: %w", err)
	}

	relTotal := 0
	for _, v := range byOrigin {
		relTotal += v
	}

	s := &Stats{
		DBPath:               st.Path,
		DBSizeBytes:          dbSize(st.Path),
		NodeCount:            core.NodeCount,
		SnapshotCount:        core.SnapshotCount,
		AvgTemp:              core.AvgTemp,
		MedianTemp:           core.MedianTemp,
		HotCount:             core.HotCount,
		ColdCount:            core.ColdCount,
		PinnedCount:          core.PinnedCount,
		TokenCountTotal:      core.TokenCountTotal,
		FTSRowCount:          core.FTSRowCount,
		NodeCountsByType:     byType,
		RelationCount:        relTotal + core.PendingRelationCount,
		RelationsByOrigin:    byOrigin,
		PendingRelationCount: core.PendingRelationCount,
	}

	snaps, err := st.ListSnapshots(ctx, 1)
	if err != nil {
		return nil, fmt.Errorf("failed to list: snapshots: %w", err)
	}
	if len(snaps) > 0 {
		latest := snaps[0]
		s.Latest = &LatestSnapshot{
			ID:         latest.ID,
			CursorHash: latest.CursorHash,
			Message:    latest.Message,
			AgeSeconds: time.Now().Unix() - latest.CreatedAt,
		}
	}
	return s, nil
}

func dbSize(path string) int64 {
	if path == "" || path == ":memory:" {
		return 0
	}

	fi, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return fi.Size()
}
