package store

import "context"

type Stats struct {
	NodeCount     int
	SnapshotCount int
	AvgTemp       float64
	HotCount      int
	ColdCount     int
}

func (s *Store) GetStats(ctx context.Context) (*Stats, error) {
	var st Stats

	err := s.db.QueryRowContext(ctx,
		`SELECT count(*), coalesce(avg(temperature), 0),
			coalesce(sum(temperature >= 0.5), 0),
			coalesce(sum(temperature < 0.1), 0)
		FROM nodes`).
		Scan(&st.NodeCount, &st.AvgTemp, &st.HotCount, &st.ColdCount)
	if err != nil {
		return nil, err
	}

	err = s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM snapshots`).
		Scan(&st.SnapshotCount)
	if err != nil {
		return nil, err
	}

	return &st, nil
}
