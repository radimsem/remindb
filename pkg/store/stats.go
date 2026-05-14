package store

import "context"

const (
	hotThreshold  = 0.5
	coldThreshold = 0.1
)

type Stats struct {
	NodeCount            int
	SnapshotCount        int
	AvgTemp              float64
	MedianTemp           float64
	HotCount             int
	ColdCount            int
	PinnedCount          int
	TokenCountTotal      int64
	FTSRowCount          int
	PendingRelationCount int
}

func (s *Store) GetStats(ctx context.Context) (*Stats, error) {
	var st Stats
	err := s.db.QueryRowContext(ctx, qSelectStats, hotThreshold, coldThreshold).
		Scan(
			&st.NodeCount, &st.AvgTemp, &st.MedianTemp,
			&st.HotCount, &st.ColdCount, &st.PinnedCount,
			&st.TokenCountTotal,
			&st.SnapshotCount, &st.FTSRowCount, &st.PendingRelationCount,
		)

	if err != nil {
		return nil, err
	}
	return &st, nil
}

// Return node counts grouped by node_type (e.g. heading, list, kv, table).
func (s *Store) GetNodeCountsByType(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, qSelectNodeCountsByType)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string]int)
	for rows.Next() {
		var kind string
		var count int

		if err := rows.Scan(&kind, &count); err != nil {
			return nil, err
		}
		out[kind] = count
	}
	return out, rows.Err()
}

// Return relation counts grouped by origin (parsed, manual).
func (s *Store) GetRelationCountsByOrigin(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, qSelectRelationCountsByOrigin)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string]int)
	for rows.Next() {
		var origin string
		var count int

		if err := rows.Scan(&origin, &count); err != nil {
			return nil, err
		}
		out[origin] = count
	}
	return out, rows.Err()
}
