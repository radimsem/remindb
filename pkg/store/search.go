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
	rows, err := s.queryFTS(ctx, qSearchFTS, query, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return collectRows(rows)
}

func (s *Store) SearchRanked(ctx context.Context, query string, limit int) ([]*RankedNode, error) {
	rows, err := s.queryFTS(ctx, qSearchRanked, query, limit)
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
			&n.CreatedAt, &n.UpdatedAt, &n.Pinned, &rank,
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

	// Quote each term so internal punctuation (hyphens, dots) matches as a
	// literal phrase instead of leaking into FTS5 as an operator.
	terms := strings.Fields(q)
	quoted := make([]string, len(terms))
	for i, t := range terms {
		quoted[i] = `"` + t + `"`
	}

	return strings.Join(quoted, " OR ")
}

// Quote each term as an FTS5 phrase, escaping internal " as "" per FTS5 syntax.
func quoteAllTerms(q string) string {
	q = strings.TrimSpace(q)
	if q == "" {
		return q
	}

	terms := strings.Fields(q)
	quoted := make([]string, len(terms))
	for i, t := range terms {
		quoted[i] = `"` + strings.ReplaceAll(t, `"`, `""`) + `"`
	}

	return strings.Join(quoted, " OR ")
}

// Match the SQLite logic-error class FTS5 parse failures arrive under in modernc.
func isFTS5SyntaxError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "SQL logic error:")
}

// Retry with safely-quoted terms on FTS5 parse error so adversarial input degrades to zero results.
func (s *Store) queryFTS(ctx context.Context, stmt, q string, limit int) (*sql.Rows, error) {
	rows, err := s.db.QueryContext(ctx, stmt, rewriteQuery(q), limit)
	if err == nil {
		return rows, nil
	}
	if !isFTS5SyntaxError(err) {
		return nil, err
	}
	return s.db.QueryContext(ctx, stmt, quoteAllTerms(q), limit)
}
