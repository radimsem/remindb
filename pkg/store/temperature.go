package store

import (
	"context"
	"strings"
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

func (s *Store) BoostTemperatureBatch(ctx context.Context, ids []string, boost float64) error {
	if len(ids) == 0 {
		return nil
	}

	now := time.Now().Unix()
	placeholders := make([]string, len(ids))
	args := make([]any, 0, len(ids)+2)
	args = append(args, boost, now)

	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}

	query := `UPDATE nodes SET temperature = min(1.0, temperature + ?),
		access_count = access_count + 1, last_accessed_at = ?, updated_at = unixepoch()
		WHERE id IN (` + strings.Join(placeholders, ",") + `)`

	_, err := s.db.ExecContext(ctx, query, args...)
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
