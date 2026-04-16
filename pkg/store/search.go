package store

import "context"

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
