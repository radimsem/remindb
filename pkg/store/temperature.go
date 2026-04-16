package store

import (
	"context"
	"time"
)

func (s *Store) UpdateTemperature(ctx context.Context, id string, temp float64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE nodes SET temperature = ?, updated_at = unixepoch() WHERE id = ?`,
		temp, id)
	return err
}

func (s *Store) IncrementAccess(ctx context.Context, id string) error {
	now := time.Now().Unix()
	_, err := s.db.ExecContext(ctx,
		`UPDATE nodes SET access_count = access_count + 1, last_accessed_at = ?, updated_at = unixepoch()
		WHERE id = ?`, now, id)
	return err
}

func (s *Store) BoostTemperature(ctx context.Context, id string, boost float64) error {
	now := time.Now().Unix()
	_, err := s.db.ExecContext(ctx,
		`UPDATE nodes SET temperature = min(1.0, temperature + ?),
		access_count = access_count + 1, last_accessed_at = ?, updated_at = unixepoch()
		WHERE id = ?`, boost, now, id)

	return err
}

func (s *Store) DecayTemperatures(ctx context.Context, factor float64) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE nodes SET temperature = temperature * ?, updated_at = unixepoch()
		WHERE temperature > 0`, factor)
	if err != nil {
		return 0, err
	}

	return res.RowsAffected()
}

func (s *Store) GetColdNodes(ctx context.Context, threshold float64) ([]*Node, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+nodeColumns+` FROM nodes WHERE temperature < ? ORDER BY temperature ASC`,
		threshold)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()
	return collectRows(rows)
}
