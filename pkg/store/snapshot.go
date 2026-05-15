package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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

	OldParentID   sql.NullString
	OldSourceFile sql.NullString
	OldNodeType   sql.NullString
	OldDepth      sql.NullInt64
	OldLabel      sql.NullString
	OldFormat     sql.NullString
	OldTokenCount sql.NullInt64
}

// True iff the diff carries pre-state metadata.
func (d *DiffRecord) HasOldMetadata() bool {
	return d.OldSourceFile.Valid
}

// Populate the diff's old_* metadata fields from a node's pre-state.
func (d *DiffRecord) SetOldMetadata(n *Node) {
	d.OldParentID = sql.NullString{String: n.ParentID, Valid: n.ParentID != ""}
	d.OldSourceFile = sql.NullString{String: n.SourceFile, Valid: true}
	d.OldNodeType = sql.NullString{String: n.NodeType, Valid: true}
	d.OldDepth = sql.NullInt64{Int64: int64(n.Depth), Valid: true}
	d.OldLabel = sql.NullString{String: n.Label, Valid: true}
	d.OldFormat = sql.NullString{String: n.Format, Valid: true}
	d.OldTokenCount = sql.NullInt64{Int64: int64(n.TokenCount), Valid: true}
}

type SkippedRestore struct {
	NodeID string
	Reason string
}

type RestoreResult struct {
	Nodes   map[string]*Node
	Skipped []SkippedRestore
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

// Insert a snapshot with an explicit parent_id (parentID == 0 records NULL).
func (s *Store) CreateSnapshotWithParentTx(ctx context.Context, tx *sql.Tx, cursorHash, message, compileRoot string, parentID int64) (int64, error) {
	var parent sql.NullInt64
	if parentID > 0 {
		parent = sql.NullInt64{Int64: parentID, Valid: true}
	}

	res, err := tx.ExecContext(ctx, qInsertSnapshot, cursorHash, parent, message, compileRoot)
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
		d.SnapshotID, d.NodeID, d.Op, d.OldHash, d.NewHash, d.OldContent, d.NewContent,
		d.OldParentID, d.OldSourceFile, d.OldNodeType, d.OldDepth,
		d.OldLabel, d.OldFormat, d.OldTokenCount,
	)
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

// Return the snapshot id at HEAD, or 0 if no snapshot has been recorded yet.
func (s *Store) GetHeadSnapshotID(ctx context.Context) (int64, error) {
	var id sql.NullInt64
	err := s.db.QueryRowContext(ctx, qSelectHeadCursorSnapID).Scan(&id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}

	if !id.Valid {
		return 0, nil
	}
	return id.Int64, nil
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

func (s *Store) GetDiffsBetween(ctx context.Context, fromSnapshotID, toSnapshotID int64) ([]*DiffRecord, error) {
	rows, err := s.db.QueryContext(ctx, qSelectDiffsBetween, fromSnapshotID, toSnapshotID)
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

		err := rows.Scan(
			&d.SnapshotID, &d.NodeID, &d.Op, &d.OldHash, &d.NewHash, &d.OldContent, &d.NewContent,
			&d.OldParentID, &d.OldSourceFile, &d.OldNodeType, &d.OldDepth,
			&d.OldLabel, &d.OldFormat, &d.OldTokenCount,
		)
		if err != nil {
			return nil, err
		}

		out = append(out, &d)
	}
	return out, rows.Err()
}

// Compute the node set at targetID by reverse-walking diffs from HEAD. Pure read.
func (s *Store) RestoreToSnapshot(ctx context.Context, targetID int64) (*RestoreResult, error) {
	if _, err := s.GetSnapshot(ctx, int(targetID)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("snapshot %d not found", targetID)
		}
		return nil, fmt.Errorf("failed to fetch: snapshot %d: %w", targetID, err)
	}

	nodes, err := s.GetAllNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load: head nodes: %w", err)
	}

	state := make(map[string]*Node, len(nodes))
	for _, n := range nodes {
		c := *n
		state[n.ID] = &c
	}

	rows, err := s.db.QueryContext(ctx, qSelectDiffsAfter, targetID)
	if err != nil {
		return nil, fmt.Errorf("failed to load: diffs after %d: %w", targetID, err)
	}
	defer func() { _ = rows.Close() }()

	diffs, err := collectDiffRows(rows)
	if err != nil {
		return nil, fmt.Errorf("failed to scan: diffs after %d: %w", targetID, err)
	}

	var skipped []SkippedRestore
	for _, d := range diffs {
		reverseApplyDiff(d, state, &skipped)
	}

	return &RestoreResult{Nodes: state, Skipped: skipped}, nil
}

func reverseApplyDiff(d *DiffRecord, state map[string]*Node, skipped *[]SkippedRestore) {
	switch d.Op {
	case "add":
		delete(state, d.NodeID)

	case "mod":
		existing, ok := state[d.NodeID]
		if !ok {
			return
		}

		existing.Content = d.OldContent
		existing.ContentHash = d.OldHash
		if d.HasOldMetadata() {
			applyOldMetadata(existing, d)
		}

	case "rem":
		if !d.HasOldMetadata() {
			*skipped = append(*skipped, SkippedRestore{
				NodeID: d.NodeID,
				Reason: "pre-migration OpRem; node metadata unavailable",
			})
			return
		}

		state[d.NodeID] = &Node{
			ID:          d.NodeID,
			ParentID:    d.OldParentID.String,
			SourceFile:  d.OldSourceFile.String,
			NodeType:    d.OldNodeType.String,
			Depth:       int(d.OldDepth.Int64),
			Label:       d.OldLabel.String,
			Content:     d.OldContent,
			Format:      d.OldFormat.String,
			TokenCount:  int(d.OldTokenCount.Int64),
			ContentHash: d.OldHash,
		}
	}
}

func applyOldMetadata(n *Node, d *DiffRecord) {
	n.ParentID = d.OldParentID.String
	n.SourceFile = d.OldSourceFile.String
	n.NodeType = d.OldNodeType.String
	n.Depth = int(d.OldDepth.Int64)
	n.Label = d.OldLabel.String
	n.Format = d.OldFormat.String
	n.TokenCount = int(d.OldTokenCount.Int64)
}

// Hard-delete snapshots (and their diffs) with id > targetID, except excludeID.
func (s *Store) PruneSnapshotsAfterTx(ctx context.Context, tx *sql.Tx, targetID, excludeID int64) (int, error) {
	if _, err := tx.ExecContext(ctx, `PRAGMA defer_foreign_keys = 1`); err != nil {
		return 0, fmt.Errorf("failed to defer: foreign keys: %w", err)
	}

	if _, err := tx.ExecContext(ctx, qDeleteDiffsAfter, targetID, excludeID); err != nil {
		return 0, fmt.Errorf("failed to delete: diffs after %d: %w", targetID, err)
	}

	res, err := tx.ExecContext(ctx, qDeleteSnapshotsAfter, targetID, excludeID)
	if err != nil {
		return 0, fmt.Errorf("failed to delete: snapshots after %d: %w", targetID, err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(n), nil
}
