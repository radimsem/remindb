package store

import (
	"context"
	"database/sql"
)

type Snapshot struct {
	ID          int64
	CursorHash  string
	ParentID    sql.NullInt64
	Message     string
	CompileRoot string
	CreatedAt   int64
}

type DiffRecord struct {
	SnapshotID int64
	NodeID     string
	Op         string
	OldHash    string
	NewHash    string
	OldContent string
	NewContent string
}

func (s *Store) CreateSnapshotTx(ctx context.Context, tx *sql.Tx, cursorHash, message, compileRoot string) (int64, error) {
	var parentID sql.NullInt64
	row := tx.QueryRowContext(ctx, qSelectHeadCursorSnapID)
	if err := row.Scan(&parentID); err != nil && err != sql.ErrNoRows {
		return 0, err
	}

	res, err := tx.ExecContext(ctx, qInsertSnapshot, cursorHash, parentID, message, compileRoot)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Return the compile root of the most recent snapshot created via a directory compile.
func (s *Store) GetLatestCompileRoot(ctx context.Context) (string, error) {
	var root string
	err := s.db.QueryRowContext(ctx, qSelectLatestCompileRoot).Scan(&root)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return root, err
}

func (s *Store) InsertDiffTx(ctx context.Context, tx *sql.Tx, d *DiffRecord) error {
	_, err := tx.ExecContext(ctx, qInsertDiff,
		d.SnapshotID, d.NodeID, d.Op, d.OldHash, d.NewHash, d.OldContent, d.NewContent)
	return err
}

func (s *Store) AdvanceCursorTx(ctx context.Context, tx *sql.Tx, snapshotID int64) error {
	_, err := tx.ExecContext(ctx, qUpsertHeadCursor, snapshotID)
	return err
}

func (s *Store) GetHeadCursorHash(ctx context.Context) (string, error) {
	var hash string
	err := s.db.QueryRowContext(ctx, qSelectHeadCursorHash).Scan(&hash)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return hash, err
}

func (s *Store) GetSnapshot(ctx context.Context, id int) (*Snapshot, error) {
	var snap Snapshot
	err := s.db.QueryRowContext(ctx, qSelectSnapshotByID, id).
		Scan(&snap.ID, &snap.CursorHash, &snap.ParentID, &snap.Message, &snap.CompileRoot, &snap.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &snap, nil
}

func (s *Store) ListSnapshots(ctx context.Context, limit int) ([]*Snapshot, error) {
	rows, err := s.db.QueryContext(ctx, qListSnapshots, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []*Snapshot
	for rows.Next() {
		var snap Snapshot
		if err := rows.Scan(&snap.ID, &snap.CursorHash, &snap.ParentID, &snap.Message, &snap.CompileRoot, &snap.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &snap)
	}
	return out, rows.Err()
}

func (s *Store) GetDiffsBySnapshot(ctx context.Context, snapshotID int64) ([]*DiffRecord, error) {
	rows, err := s.db.QueryContext(ctx, qSelectDiffsBySnapshot, snapshotID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return collectDiffRows(rows)
}

func (s *Store) GetDiffsSince(ctx context.Context, sinceSnapshotID int64) ([]*DiffRecord, error) {
	rows, err := s.db.QueryContext(ctx, qSelectDiffsSince, sinceSnapshotID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return collectDiffRows(rows)
}

func (s *Store) GetDiffsForNode(ctx context.Context, nodeID string) ([]*DiffRecord, error) {
	rows, err := s.db.QueryContext(ctx, qSelectDiffsForNode, nodeID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return collectDiffRows(rows)
}

func collectDiffRows(rows *sql.Rows) ([]*DiffRecord, error) {
	var out []*DiffRecord
	for rows.Next() {
		var d DiffRecord
		if err := rows.Scan(&d.SnapshotID, &d.NodeID, &d.Op, &d.OldHash, &d.NewHash, &d.OldContent, &d.NewContent); err != nil {
			return nil, err
		}
		out = append(out, &d)
	}
	return out, rows.Err()
}
