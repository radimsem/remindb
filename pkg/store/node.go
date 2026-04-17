package store

import (
	"context"
	"database/sql"
	"strings"
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
	SeedTemp     *float64
}

type RowScanner interface {
	Scan(...any) error
}

const nodeColumns = `id, parent_id, source_file, node_type, depth,
	label, content, format, token_count, content_hash,
	temperature, access_count, last_accessed_at,
	created_at, updated_at`

const nodeColumnsAliased = `n.id, n.parent_id, n.source_file, n.node_type, n.depth,
	n.label, n.content, n.format, n.token_count, n.content_hash,
	n.temperature, n.access_count, n.last_accessed_at,
	n.created_at, n.updated_at`

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

func (s *Store) GetNodesByFiles(ctx context.Context, paths []string) ([]*Node, error) {
	if len(paths) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(paths))
	args := make([]any, len(paths))
	for i, p := range paths {
		placeholders[i] = "?"
		args[i] = p
	}

	query := `SELECT ` + nodeColumns + ` FROM nodes WHERE source_file IN (` + strings.Join(placeholders, ",") + `)`
	rows, err := s.db.QueryContext(ctx, query, args...)
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
		label, content, format, token_count, content_hash, temperature)
VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, COALESCE(?11, 0.5))
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
	temperature = CASE WHEN ?11 IS NOT NULL THEN excluded.temperature ELSE temperature END,
	updated_at = unixepoch()`

func upsertArgs(n *Node) []any {
	var seedTemp any
	if n.SeedTemp != nil {
		seedTemp = *n.SeedTemp
	}
	return []any{
		n.ID, parentIDParam(n.ParentID), n.SourceFile, n.NodeType, n.Depth,
		n.Label, n.Content, n.Format, n.TokenCount, n.ContentHash, seedTemp,
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

func (s *Store) GetDescendants(ctx context.Context, id string, maxDepth int) ([]*Node, error) {
	rows, err := s.db.QueryContext(ctx, `
		WITH RECURSIVE desc_cte(nid, lvl) AS (
			SELECT id, 1 FROM nodes WHERE parent_id = ?
			UNION ALL
			SELECT n.id, d.lvl + 1 FROM nodes n
			JOIN desc_cte d ON n.parent_id = d.nid
			WHERE d.lvl < ?
		)
		SELECT `+nodeColumns+` FROM nodes WHERE id IN (SELECT nid FROM desc_cte)`, id, maxDepth)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()
	return collectRows(rows)
}

func (s *Store) GetSiblings(ctx context.Context, id string) ([]*Node, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+nodeColumns+` FROM nodes
		WHERE parent_id = (SELECT parent_id FROM nodes WHERE id = ?)
		AND id != ?`, id, id)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()
	return collectRows(rows)
}

func (s *Store) GetRootNodes(ctx context.Context) ([]*Node, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+nodeColumns+` FROM nodes WHERE parent_id IS NULL ORDER BY source_file, depth`)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()
	return collectRows(rows)
}

func (s *Store) GetAllNodes(ctx context.Context) ([]*Node, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+nodeColumns+` FROM nodes ORDER BY source_file, depth`)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()
	return collectRows(rows)
}

func BuildTree(nodes []*Node) (roots []*Node, children map[string][]*Node) {
	children = make(map[string][]*Node, len(nodes))

	for _, n := range nodes {
		if n.ParentID == "" {
			roots = append(roots, n)
		} else {
			children[n.ParentID] = append(children[n.ParentID], n)
		}
	}
	return roots, children
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
