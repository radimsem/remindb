package store

import (
	"context"
	"database/sql"
)

type Node struct {
	ID           string
	ParentID     string
	SourceFile   string
	NodeType     string
	Depth        int
	Label        string
	Content      string
	Format       string
	TokenCount   int
	ContentHash  string
	Temperature  float64
	AccessCount  int
	LastAccessed sql.NullInt64
	CreatedAt    int64
	UpdatedAt    int64
}

type RowScanner interface {
	Scan(...any) error
}

const nodeColumns = `id, parent_id, source_file, node_type, depth,
	label, content, format, token_count, content_hash,
	temperature, access_count, last_accessed_at,
	created_at, updated_at`

func scanNode(r RowScanner) (*Node, error) {
	var n Node
	var parentID sql.NullString

	err := r.Scan(
		&n.ID, &parentID, &n.SourceFile, &n.NodeType, &n.Depth,
		&n.Label, &n.Content, &n.Format, &n.TokenCount, &n.ContentHash,
		&n.Temperature, &n.AccessCount, &n.LastAccessed,
		&n.CreatedAt, &n.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	n.ParentID = parentID.String
	return &n, nil
}

func parentIDParam(id string) any {
	if id == "" {
		return nil
	}
	return id
}

func (s *Store) GetNode(ctx context.Context, id string) (*Node, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+nodeColumns+` FROM nodes WHERE id = ?`, id)
	return scanNode(row)
}

func (s *Store) GetNodesByFile(ctx context.Context, path string) ([]*Node, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+nodeColumns+` FROM nodes WHERE source_file = ?`, path)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()
	return collectRows(rows)
}

func (s *Store) GetChildren(ctx context.Context, parentID string) ([]*Node, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+nodeColumns+` FROM nodes WHERE parent_id = ?`, parentID)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()
	return collectRows(rows)
}

func (s *Store) GetAncestors(ctx context.Context, id string) ([]*Node, error) {
	rows, err := s.db.QueryContext(ctx, `
		WITH RECURSIVE anc AS (
			SELECT * FROM nodes WHERE id = ?
			UNION ALL
			SELECT n.* FROM nodes n
			JOIN anc a ON n.id = a.parent_id
			WHERE a.parent_id IS NOT NULL
		)
		SELECT * FROM anc`, id)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()
	return collectRows(rows)
}

const upsertSQL = `
INSERT INTO nodes (id, parent_id, source_file, node_type, depth,
		label, content, format, token_count, content_hash)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	parent_id = excluded.parent_id,
	source_file = excluded.source_file,
	node_type = excluded.node_type,
	depth = excluded.depth,
	label = excluded.label,
	content = excluded.content,
	format = excluded.format,
	token_count = excluded.token_count,
	content_hash = excluded.content_hash,
	updated_at = unixepoch()`

func upsertArgs(n *Node) []any {
	return []any{
		n.ID, parentIDParam(n.ParentID), n.SourceFile, n.NodeType, n.Depth,
		n.Label, n.Content, n.Format, n.TokenCount, n.ContentHash,
	}
}

func (s *Store) UpsertNode(ctx context.Context, n *Node) error {
	_, err := s.db.ExecContext(ctx, upsertSQL, upsertArgs(n)...)
	return err
}

func (s *Store) UpsertNodeTx(ctx context.Context, tx *sql.Tx, n *Node) error {
	_, err := tx.ExecContext(ctx, upsertSQL, upsertArgs(n)...)
	return err
}

func (s *Store) DeleteNode(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM nodes WHERE id = ?`, id)
	return err
}

func (s *Store) DeleteNodeTx(ctx context.Context, tx *sql.Tx, id string) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM nodes WHERE id = ?`, id)
	return err
}

func collectRows(rows *sql.Rows) ([]*Node, error) {
	var out []*Node
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}
