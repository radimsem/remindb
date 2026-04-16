package store

import (
	"context"
	"database/sql"
)

type Snapshot struct {
	ID         int64
	CursorHash string
	ParentID   sql.NullInt64
	Message    string
	CreatedAt  int64
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

func (s *Store) CreateSnapshotTx(ctx context.Context, tx *sql.Tx, cursorHash, message string) (int64, error) {
	var parentID sql.NullInt64

	row := tx.QueryRowContext(ctx,
		`SELECT snapshot_id FROM cursors WHERE id = 'HEAD'`)
	if err := row.Scan(&parentID); err != nil && err != sql.ErrNoRows {
		return 0, err
	}

	res, err := tx.ExecContext(ctx,
		`INSERT INTO snapshots (cursor_hash, parent_id, message) VALUES (?, ?, ?)`,
		cursorHash, parentID, message)
	if err != nil {
		return 0, err
	}

	return res.LastInsertId()
}

func (s *Store) InsertDiffTx(ctx context.Context, tx *sql.Tx, d *DiffRecord) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO diffs (snapshot_id, node_id, op, old_hash, new_hash, old_content, new_content)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		d.SnapshotID, d.NodeID, d.Op, d.OldHash, d.NewHash, d.OldContent, d.NewContent)
	return err
}

func (s *Store) AdvanceCursorTx(ctx context.Context, tx *sql.Tx, snapshotID int64) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO cursors (id, snapshot_id) VALUES ('HEAD', ?)
		ON CONFLICT(id) DO UPDATE SET snapshot_id = excluded.snapshot_id, updated_at = unixepoch()`,
		snapshotID)
	return err
}

func (s *Store) GetHeadCursorHash(ctx context.Context) (string, error) {
	var hash string
	err := s.db.QueryRowContext(ctx,
		`SELECT s.cursor_hash FROM cursors c
		JOIN snapshots s ON s.id = c.snapshot_id
		WHERE c.id = 'HEAD'`).Scan(&hash)

	if err == sql.ErrNoRows {
		return "", nil
	}
	return hash, err
}

func (s *Store) GetSnapshot(ctx context.Context, id int) (*Snapshot, error) {
	var snap Snapshot
	err := s.db.QueryRowContext(ctx,
		`SELECT id, cursor_hash, parent_id, message, created_at
		FROM snapshots WHERE id = ?`, id).
		Scan(&snap.ID, &snap.CursorHash, &snap.ParentID, &snap.Message, &snap.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &snap, nil
}

func (s *Store) ListSnapshots(ctx context.Context, limit int) ([]*Snapshot, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, cursor_hash, parent_id, message, created_at
		FROM snapshots ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []*Snapshot
	for rows.Next() {
		var snap Snapshot
		if err := rows.Scan(&snap.ID, &snap.CursorHash, &snap.ParentID, &snap.Message, &snap.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &snap)
	}
	return out, rows.Err()
}

func (s *Store) GetDiffsBySnapshot(ctx context.Context, snapshotID int64) ([]*DiffRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT snapshot_id, node_id, op, old_hash, new_hash, old_content, new_content
		FROM diffs WHERE snapshot_id = ?
		ORDER BY id`, snapshotID)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()
	return collectDiffRows(rows)
}

func (s *Store) GetDiffsSince(ctx context.Context, sinceSnapshotID int64) ([]*DiffRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT snapshot_id, node_id, op, old_hash, new_hash, old_content, new_content
		FROM diffs WHERE snapshot_id > ?
		ORDER BY snapshot_id, id`, sinceSnapshotID)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()
	return collectDiffRows(rows)
}

func (s *Store) GetDiffsForNode(ctx context.Context, nodeID string) ([]*DiffRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT snapshot_id, node_id, op, old_hash, new_hash, old_content, new_content
		FROM diffs WHERE node_id = ?
		ORDER BY snapshot_id`, nodeID)
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
