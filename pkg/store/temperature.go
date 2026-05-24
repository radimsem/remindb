package store

import (
	"context"
	"database/sql"
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

	clause, idArgs := bindStrings(ids)
	args := append([]any{boost, time.Now().Unix()}, idArgs...)
	query := qBoostTemperatureBatchPrefix + clause + `)`

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
	return s.Tx(ctx, func(tx *sql.Tx) error {
		return s.ResetTemperaturesByFilesTx(ctx, tx, paths, temp)
	})
}

func (s *Store) ResetTemperaturesByFilesTx(ctx context.Context, tx *sql.Tx, paths []string, temp float64) error {
	if len(paths) == 0 {
		return nil
	}

	clause, pathArgs := bindStrings(paths)
	args := append([]any{temp}, pathArgs...)
	query := qResetTemperaturesByFilesPrefix + clause + `)`

	_, err := tx.ExecContext(ctx, query, args...)
	return err
}

func (s *Store) ResetPinnedByFiles(ctx context.Context, paths []string) error {
	return s.Tx(ctx, func(tx *sql.Tx) error {
		return s.ResetPinnedByFilesTx(ctx, tx, paths)
	})
}

func (s *Store) ResetPinnedByFilesTx(ctx context.Context, tx *sql.Tx, paths []string) error {
	if len(paths) == 0 {
		return nil
	}

	clause, args := bindStrings(paths)
	query := qResetPinnedByFilesPrefix + clause + `)`

	_, err := tx.ExecContext(ctx, query, args...)
	return err
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
