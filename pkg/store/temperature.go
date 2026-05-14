package store

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

func (s *Store) UpdateTemperature(ctx context.Context, id string, temp float64) error {
	return s.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, qUpdateTemperature, temp, id)
		return err
	})
}

func (s *Store) IncrementAccess(ctx context.Context, id string) error {
	return s.Tx(ctx, func(tx *sql.Tx) error {
		now := time.Now().Unix()
		_, err := tx.ExecContext(ctx, qIncrementAccess, now, id)
		return err
	})
}

func (s *Store) BoostTemperature(ctx context.Context, id string, boost float64) error {
	return s.Tx(ctx, func(tx *sql.Tx) error {
		now := time.Now().Unix()
		_, err := tx.ExecContext(ctx, qBoostTemperature, boost, now, id)
		return err
	})
}

func (s *Store) BoostTemperatureBatch(ctx context.Context, ids []string, boost float64) error {
	if len(ids) == 0 {
		return nil
	}

	placeholders := make([]string, len(ids))
	args := make([]any, 0, len(ids)+2)
	args = append(args, boost, time.Now().Unix())

	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}

	query := qBoostTemperatureBatchPrefix + strings.Join(placeholders, ",") + `)`
	return s.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, query, args...)
		return err
	})
}

func (s *Store) DecayTemperatures(ctx context.Context, factor float64) (int64, error) {
	var affected int64
	err := s.Tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, qDecayTemperatures, factor)
		if err != nil {
			return err
		}
		affected, err = res.RowsAffected()
		return err
	})
	return affected, err
}

func (s *Store) ResetTemperaturesByFiles(ctx context.Context, paths []string, temp float64) error {
	if len(paths) == 0 {
		return nil
	}

	placeholders := make([]string, len(paths))
	args := make([]any, 0, len(paths)+1)
	args = append(args, temp)

	for i, p := range paths {
		placeholders[i] = "?"
		args = append(args, p)
	}

	query := qResetTemperaturesByFilesPrefix + strings.Join(placeholders, ",") + `)`
	return s.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, query, args...)
		return err
	})
}

func (s *Store) GetColdNodes(ctx context.Context, threshold float64, limit int) ([]*Node, error) {
	rows, err := s.db.QueryContext(ctx, qSelectColdNodes, threshold, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return collectRows(rows)
}

func (s *Store) SetPinned(ctx context.Context, id string, pinned bool, temp *float64) error {
	return s.Tx(ctx, func(tx *sql.Tx) error {
		if temp != nil {
			if _, err := tx.ExecContext(ctx, qUpdateTemperature, *temp, id); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, qSetPinned, pinned, id)
		return err
	})
}
