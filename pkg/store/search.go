package store

import (
	"context"
	"database/sql"
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
		)`, query, limit)
	if err != nil {
		return nil, err
	}

	defer func() { _ = rows.Close() }()
	return collectRows(rows)
}

func (s *Store) SearchRanked(ctx context.Context, query string, limit int) ([]*RankedNode, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT n.id, n.parent_id, n.source_file, n.node_type, n.depth,
			n.label, n.content, n.format, n.token_count, n.content_hash,
			n.temperature, n.access_count, n.last_accessed_at,
			n.created_at, n.updated_at, nodes_fts.rank
		FROM nodes_fts
		JOIN nodes n ON n.rowid = nodes_fts.rowid
		WHERE nodes_fts MATCH ?
		ORDER BY nodes_fts.rank
		LIMIT ?`, query, limit)
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
