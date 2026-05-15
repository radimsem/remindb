package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type DeleteMode int

const (
	DeleteStrict DeleteMode = iota
	DeleteCascade
	DeleteReparent
)

func (m DeleteMode) String() string {
	switch m {
	case DeleteStrict:
		return "strict"
	case DeleteCascade:
		return "cascade"
	case DeleteReparent:
		return "reparent"
	default:
		return ""
	}
}

func ParseDeleteMode(s string) (DeleteMode, error) {
	switch s {
	case "", "strict":
		return DeleteStrict, nil
	case "cascade":
		return DeleteCascade, nil
	case "reparent":
		return DeleteReparent, nil
	default:
		return 0, fmt.Errorf("unknown delete mode %q (want strict, cascade, or reparent)", s)
	}
}

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
	Pinned       bool
	SeedTemp     *float64
}

type FileSummary struct {
	Path        string
	NodeCount   int
	TokenCount  int
	CompileRoot string
}

type RowScanner interface {
	Scan(...any) error
}

func scanNode(r RowScanner) (*Node, error) {
	var n Node
	var parentID sql.NullString

	err := r.Scan(
		&n.ID, &parentID, &n.SourceFile, &n.NodeType, &n.Depth,
		&n.Label, &n.Content, &n.Format, &n.TokenCount, &n.ContentHash,
		&n.Temperature, &n.AccessCount, &n.LastAccessed,
		&n.CreatedAt, &n.UpdatedAt, &n.Pinned,
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
	row := s.db.QueryRowContext(ctx, qSelectNodeByID, id)
	return scanNode(row)
}

func (s *Store) GetNodesByFile(ctx context.Context, path string) ([]*Node, error) {
	rows, err := s.db.QueryContext(ctx, qSelectNodesByFile, path)
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

	query := qSelectNodesByFilesPrefix + strings.Join(placeholders, ",") + `)`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return collectRows(rows)
}

func (s *Store) GetNodesByIDs(ctx context.Context, ids []string) ([]*Node, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))

	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := qSelectNodesByIDsPrefix + strings.Join(placeholders, ",") + `)`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return collectRows(rows)
}

func (s *Store) GetNodesByCompileRoot(ctx context.Context, root string) ([]*Node, error) {
	rows, err := s.db.QueryContext(ctx, qSelectNodesByCompileRoot, root)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return collectRows(rows)
}

func (s *Store) GetChildren(ctx context.Context, parentID string) ([]*Node, error) {
	rows, err := s.db.QueryContext(ctx, qSelectChildren, parentID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return collectRows(rows)
}

func (s *Store) GetAncestors(ctx context.Context, id string) ([]*Node, error) {
	rows, err := s.db.QueryContext(ctx, qSelectAncestors, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return collectRows(rows)
}

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
	_, err := s.db.ExecContext(ctx, qUpsertNode, upsertArgs(n)...)
	return err
}

func (s *Store) UpsertNodeTx(ctx context.Context, tx *sql.Tx, n *Node) error {
	_, err := tx.ExecContext(ctx, qUpsertNode, upsertArgs(n)...)
	return err
}

// Remove a node according to the mode; returns the IDs affected.
func (s *Store) DeleteNode(ctx context.Context, id string, mode DeleteMode) ([]string, error) {
	var affected []string
	err := s.Tx(ctx, func(tx *sql.Tx) error {
		var inner error

		affected, inner = s.deleteNodeTx(ctx, tx, id, mode)
		return inner
	})

	return affected, err
}

func (s *Store) deleteNodeTx(ctx context.Context, tx *sql.Tx, id string, mode DeleteMode) ([]string, error) {
	var parentID sql.NullString
	if err := tx.QueryRowContext(ctx, qSelectParentID, id).Scan(&parentID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("node %s not found", id)
		}
		return nil, fmt.Errorf("failed to load: parent of %s: %w", id, err)
	}

	childIDs, err := selectIDsTx(ctx, tx, qSelectChildIDs, id)
	if err != nil {
		return nil, fmt.Errorf("failed to load: children of %s: %w", id, err)
	}

	switch mode {
	case DeleteStrict:
		if len(childIDs) > 0 {
			return nil, fmt.Errorf("node %s has %d children; pass mode=cascade or mode=reparent", id, len(childIDs))
		}

		if _, err := tx.ExecContext(ctx, qDeleteNode, id); err != nil {
			return nil, fmt.Errorf("failed to delete: node %s: %w", id, err)
		}
		return []string{id}, nil

	case DeleteCascade:
		descIDs, err := selectIDsTx(ctx, tx, qSelectDescendantIDs, id)
		if err != nil {
			return nil, fmt.Errorf("failed to load: descendants of %s: %w", id, err)
		}

		if _, err := tx.ExecContext(ctx, qDeleteNode, id); err != nil {
			return nil, fmt.Errorf("failed to delete: node %s: %w", id, err)
		}
		return append([]string{id}, descIDs...), nil

	case DeleteReparent:
		if _, err := tx.ExecContext(ctx, qReparentChildren, parentIDParam(parentID.String), id); err != nil {
			return nil, fmt.Errorf("failed to reparent: children of %s: %w", id, err)
		}

		if _, err := tx.ExecContext(ctx, qDeleteNode, id); err != nil {
			return nil, fmt.Errorf("failed to delete: node %s: %w", id, err)
		}
		return append([]string{id}, childIDs...), nil

	default:
		return nil, fmt.Errorf("unknown delete mode: %d", mode)
	}
}

func (s *Store) DeleteNodeTx(ctx context.Context, tx *sql.Tx, id string) error {
	_, err := tx.ExecContext(ctx, qDeleteNode, id)
	return err
}

func selectIDsTx(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]string, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []string
	for rows.Next() {
		var id string

		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}

	return out, rows.Err()
}

func (s *Store) DeleteNodesByFiles(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return nil
	}

	placeholders := make([]string, len(paths))
	args := make([]any, len(paths))
	for i, p := range paths {
		placeholders[i] = "?"
		args[i] = p
	}

	query := qDeleteNodesByFilesPrefix + strings.Join(placeholders, ",") + `)`
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *Store) GetDescendants(ctx context.Context, id string, maxDepth int) ([]*Node, error) {
	rows, err := s.db.QueryContext(ctx, qSelectDescendants, id, maxDepth)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return collectRows(rows)
}

func (s *Store) GetSiblings(ctx context.Context, id string) ([]*Node, error) {
	rows, err := s.db.QueryContext(ctx, qSelectSiblings, id, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return collectRows(rows)
}

func (s *Store) GetRootNodes(ctx context.Context) ([]*Node, error) {
	rows, err := s.db.QueryContext(ctx, qSelectRootNodes)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return collectRows(rows)
}

func (s *Store) GetAllNodes(ctx context.Context) ([]*Node, error) {
	rows, err := s.db.QueryContext(ctx, qSelectAllNodes)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return collectRows(rows)
}

func (s *Store) ListFileSummaries(ctx context.Context) ([]FileSummary, error) {
	rows, err := s.db.QueryContext(ctx, qSelectFileSummaries)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []FileSummary
	for rows.Next() {
		var fs FileSummary
		if err := rows.Scan(&fs.Path, &fs.NodeCount, &fs.TokenCount, &fs.CompileRoot); err != nil {
			return nil, err
		}
		out = append(out, fs)
	}
	return out, rows.Err()
}

// Replace every source file prefix matching oldPrefix with newPrefix.
func (s *Store) ExecRewriteSourcePaths(ctx context.Context, oldPrefix, newPrefix string) error {
	_, err := s.db.ExecContext(ctx, qRewriteSourcePaths, newPrefix, oldPrefix, oldPrefix)
	return err
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
