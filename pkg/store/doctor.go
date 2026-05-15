package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/radimsem/remindb/migrations"
)

func (s *Store) CountNodes(ctx context.Context) (int, error) {
	return s.scanCount(ctx, qCountNodes)
}

func (s *Store) CheckFTSIntegrity(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, qFTSIntegrityCheck)
	return err
}

func (s *Store) CountOrphanParents(ctx context.Context) (int, error) {
	return s.scanCount(ctx, qCountOrphanParents)
}

func (s *Store) CountDanglingDiffs(ctx context.Context) (int, error) {
	return s.scanCount(ctx, qCountDanglingDiffs)
}

func (s *Store) CountSnapshots(ctx context.Context) (int, error) {
	return s.scanCount(ctx, qCountSnapshots)
}

func (s *Store) scanCount(ctx context.Context, query string) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, query).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func (s *Store) HeadCursorRef(ctx context.Context) (snapshotID int64, exists bool, err error) {
	var id int64

	err = s.db.QueryRowContext(ctx, qSelectHeadCursorSnapID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}

	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}

func (s *Store) SnapshotExists(ctx context.Context, id int64) (bool, error) {
	var one int

	err := s.db.QueryRowContext(ctx, qSelectSnapshotExists, id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}

	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) MaxSnapshotID(ctx context.Context) (id int64, hasAny bool, err error) {
	err = s.db.QueryRowContext(ctx, qSelectMaxSnapshotID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}

	return id, true, nil
}

func (s *Store) AppliedMigrationVersions(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, qSelectAppliedMigrationVersions)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []string
	for rows.Next() {
		var v string

		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func EmbeddedMigrationVersions() ([]string, error) {
	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		return nil, fmt.Errorf("failed to read: embedded migrations: %w", err)
	}

	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		out = append(out, e.Name())
	}

	sort.Strings(out)
	return out, nil
}

func (s *Store) DistinctCompileRoots(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, qSelectDistinctCompileRoots)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []string
	for rows.Next() {
		var r string

		if err := rows.Scan(&r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) RebuildFTS(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, qFixRebuildFTS)
	return err
}

func (s *Store) PromoteOrphansToRoots(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, qFixPromoteOrphansToRoots)
	return err
}

func (s *Store) DeleteDanglingDiffs(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, qFixDeleteDanglingDiffs)
	return err
}

func (s *Store) RepointHeadCursor(ctx context.Context) error {
	return s.Tx(ctx, func(tx *sql.Tx) error {
		var maxID int64

		err := tx.QueryRowContext(ctx, qSelectMaxSnapshotID).Scan(&maxID)
		if errors.Is(err, sql.ErrNoRows) {
			_, err = tx.ExecContext(ctx, qFixDeleteHeadCursor)
			return err
		}
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, qUpsertHeadCursor, maxID)
		return err
	})
}

func (s *Store) BackupTo(ctx context.Context, dst string) error {
	_, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, dst)
	if err != nil {
		return fmt.Errorf("failed to back up: %s: %w", dst, err)
	}
	return nil
}
