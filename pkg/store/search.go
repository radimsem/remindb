package store

import (
	"context"
	"database/sql"
	"strings"
)

type RankedNode struct {
	Node *Node
	Rank float64
}

func (s *Store) Search(ctx context.Context, query string, limit int) ([]*Node, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+nodeColumns+` FROM nodes
		WHERE rowid IN (
			SELECT rowid FROM nodes_fts WHERE nodes_fts MATCH ?
			ORDER BY rank
			LIMIT ?
		)`, rewriteQuery(query), limit)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()
	return collectRows(rows)
}

func (s *Store) SearchRanked(ctx context.Context, query string, limit int) ([]*RankedNode, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+nodeColumnsAliased+`, nodes_fts.rank
		FROM nodes_fts
		JOIN nodes n ON n.rowid = nodes_fts.rowid
		WHERE nodes_fts MATCH ?
		ORDER BY nodes_fts.rank
		LIMIT ?`, rewriteQuery(query), limit)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()

	var out []*RankedNode
	for rows.Next() {
		var n Node
		var parentID sql.NullString
		var rank float64

		err := rows.Scan(
			&n.ID, &parentID, &n.SourceFile, &n.NodeType, &n.Depth,
			&n.Label, &n.Content, &n.Format, &n.TokenCount, &n.ContentHash,
			&n.Temperature, &n.AccessCount, &n.LastAccessed,
			&n.CreatedAt, &n.UpdatedAt, &rank,
		)
		if err != nil {
			return nil, err
		}

		n.ParentID = parentID.String
		out = append(out, &RankedNode{Node: &n, Rank: rank})
	}
	return out, rows.Err()
}

var ftsOperators = []string{" OR ", " AND ", " NOT ", "NEAR(", "\"", ":", "*", "("}

// Convert a natural-language query into FTS5 OR syntax.
func rewriteQuery(q string) string {
	q = strings.TrimSpace(q)
	if q == "" {
		return q
	}

	for _, op := range ftsOperators {
		if strings.Contains(q, op) {
			return q
		}
	}

	terms := strings.Fields(q)
	if len(terms) <= 1 {
		return q
	}

	return strings.Join(terms, " OR ")
}
