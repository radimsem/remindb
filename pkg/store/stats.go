package store

import "context"

const (
	hotThreshold  = 0.5
	coldThreshold = 0.1
)

type Stats struct {
	NodeCount     int
	SnapshotCount int
	AvgTemp       float64
	HotCount      int
	ColdCount     int
}

func (s *Store) GetStats(ctx context.Context) (*Stats, error) {
	var st Stats
	err := s.db.QueryRowContext(ctx, qSelectStats, hotThreshold, coldThreshold).
		Scan(&st.NodeCount, &st.AvgTemp, &st.HotCount, &st.ColdCount, &st.SnapshotCount)

	if err != nil {
		return nil, err
	}
	return &st, nil
}
