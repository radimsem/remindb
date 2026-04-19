package store

import (
	"context"
	"strings"
	"time"
)

func (s *Store) UpdateTemperature(ctx context.Context, id string, temp float64) error {
	_, err := s.db.ExecContext(ctx, qUpdateTemperature, temp, id)
	return err
}

func (s *Store) IncrementAccess(ctx context.Context, id string) error {
	now := time.Now().Unix()
	_, err := s.db.ExecContext(ctx, qIncrementAccess, now, id)
	return err
}

func (s *Store) BoostTemperature(ctx context.Context, id string, boost float64) error {
	now := time.Now().Unix()
	_, err := s.db.ExecContext(ctx, qBoostTemperature, boost, now, id)
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

	query := qBoostTemperatureBatchPrefix + strings.Join(placeholders, ",") + `)`
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *Store) DecayTemperatures(ctx context.Context, factor float64) (int64, error) {
	res, err := s.db.ExecContext(ctx, qDecayTemperatures, factor)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) GetColdNodes(ctx context.Context, threshold float64) ([]*Node, error) {
	rows, err := s.db.QueryContext(ctx, qSelectColdNodes, threshold)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return collectRows(rows)
}
