package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

const (
	OriginParsed = "parsed"
	OriginManual = "manual"

	DirectionOut  = "out"
	DirectionIn   = "in"
	DirectionBoth = "both"
)

type Relation struct {
	ID           int64
	SourceNodeID string
	TargetNodeID string
	Weight       float64
	Origin       string
	CreatedAt    int64
}

type PendingRelation struct {
	ID           int64
	SourceNodeID string
	TargetLabel  string
	TargetSource string
	TargetIDHint string
	Weight       float64
	Origin       string
	CreatedAt    int64
}

type RelatedNode struct {
	Node   *Node
	Weight float64
	Hop    int
}

func nullableParam(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (s *Store) UpsertRelation(ctx context.Context, r *Relation) error {
	_, err := s.db.ExecContext(ctx, qUpsertRelation, r.SourceNodeID, r.TargetNodeID, r.Weight, r.Origin)
	return err
}

func (s *Store) UpsertRelationTx(ctx context.Context, tx *sql.Tx, r *Relation) error {
	_, err := tx.ExecContext(ctx, qUpsertRelation, r.SourceNodeID, r.TargetNodeID, r.Weight, r.Origin)
	return err
}

func (s *Store) InsertPendingRelation(ctx context.Context, p *PendingRelation) error {
	_, err := s.db.ExecContext(ctx, qInsertPendingRelation,
		p.SourceNodeID,
		nullableParam(p.TargetLabel),
		nullableParam(p.TargetSource),
		nullableParam(p.TargetIDHint),
		p.Weight, p.Origin,
	)
	return err
}

func (s *Store) InsertPendingRelationTx(ctx context.Context, tx *sql.Tx, p *PendingRelation) error {
	_, err := tx.ExecContext(ctx, qInsertPendingRelation,
		p.SourceNodeID,
		nullableParam(p.TargetLabel),
		nullableParam(p.TargetSource),
		nullableParam(p.TargetIDHint),
		p.Weight, p.Origin,
	)
	return err
}

func (s *Store) DeletePendingByID(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, qDeletePendingByID, id)
	return err
}

func (s *Store) DeletePendingByIDTx(ctx context.Context, tx *sql.Tx, id int64) error {
	_, err := tx.ExecContext(ctx, qDeletePendingByID, id)
	return err
}

func (s *Store) DeleteParsedPendingForSource(ctx context.Context, sourceID string) error {
	_, err := s.db.ExecContext(ctx, qDeleteParsedPendingForSource, sourceID)
	return err
}

func (s *Store) DeleteParsedPendingForSourceTx(ctx context.Context, tx *sql.Tx, sourceID string) error {
	_, err := tx.ExecContext(ctx, qDeleteParsedPendingForSource, sourceID)
	return err
}

// Look up a heading node ID by case-insensitive trimmed label match across all files.
func (s *Store) FindHeadingByLabel(ctx context.Context, label string) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx, qFindHeadingByLabel, label).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}

// Look up a heading node ID by label scoped to a source file.
func (s *Store) FindHeadingByLabelInFile(ctx context.Context, sourceFile, label string) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx, qFindHeadingByLabelInFile, sourceFile, sourceFile, label).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}

func (s *Store) GetAllRelations(ctx context.Context) ([]*Relation, error) {
	rows, err := s.db.QueryContext(ctx, qSelectAllRelations)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return collectRelationRows(rows)
}

func (s *Store) GetAllPendingRelations(ctx context.Context) ([]*PendingRelation, error) {
	rows, err := s.db.QueryContext(ctx, qSelectAllPendingRelations)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return collectPendingRows(rows)
}

func (s *Store) GetPendingBySource(ctx context.Context, sourceID string) ([]*PendingRelation, error) {
	rows, err := s.db.QueryContext(ctx, qSelectPendingBySource, sourceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	return collectPendingRows(rows)
}

// Return nodes reachable from anchor via relations edges, up to maxDepth hops, filtered by weightMin.
func (s *Store) GetRelatedNodes(ctx context.Context, anchorID, direction string, maxDepth int, weightMin float64, limit int) ([]*RelatedNode, error) {
	if maxDepth < 1 {
		maxDepth = 1
	}
	if limit < 1 {
		limit = 100
	}

	query, args := relatedQueryArgs(anchorID, direction, maxDepth, weightMin, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []*RelatedNode
	for rows.Next() {
		rn, err := scanRelatedNode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rn)
	}
	return out, rows.Err()
}

func relatedQueryArgs(anchorID, direction string, maxDepth int, weightMin float64, limit int) (string, []any) {
	switch direction {
	case DirectionIn:
		return qRelatedIn, []any{anchorID, weightMin, maxDepth, weightMin, anchorID, limit}
	case DirectionOut:
		return qRelatedOut, []any{anchorID, weightMin, maxDepth, weightMin, anchorID, limit}
	default:
		return qRelatedBoth, []any{
			anchorID, weightMin, maxDepth, weightMin,
			anchorID, weightMin, maxDepth, weightMin,
			anchorID, limit,
		}
	}
}

func scanRelatedNode(r RowScanner) (*RelatedNode, error) {
	var n Node
	var parentID sql.NullString
	var hop int
	var weight float64

	err := r.Scan(
		&n.ID, &parentID, &n.SourceFile, &n.NodeType, &n.Depth,
		&n.Label, &n.Content, &n.Format, &n.TokenCount, &n.ContentHash,
		&n.Temperature, &n.AccessCount, &n.LastAccessed,
		&n.CreatedAt, &n.UpdatedAt, &n.Pinned,
		&hop, &weight,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan: related node: %w", err)
	}

	n.ParentID = parentID.String
	return &RelatedNode{Node: &n, Weight: weight, Hop: hop}, nil
}

func scanRelation(r RowScanner) (*Relation, error) {
	var rel Relation

	err := r.Scan(
		&rel.ID, &rel.SourceNodeID, &rel.TargetNodeID,
		&rel.Weight, &rel.Origin, &rel.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &rel, nil
}

func collectRelationRows(rows *sql.Rows) ([]*Relation, error) {
	var out []*Relation
	for rows.Next() {
		rel, err := scanRelation(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, rel)
	}
	return out, rows.Err()
}

func scanPending(r RowScanner) (*PendingRelation, error) {
	var p PendingRelation
	var label, source, idHint sql.NullString

	err := r.Scan(
		&p.ID, &p.SourceNodeID, &label, &source, &idHint,
		&p.Weight, &p.Origin, &p.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	p.TargetLabel = label.String
	p.TargetSource = source.String
	p.TargetIDHint = idHint.String
	return &p, nil
}

func collectPendingRows(rows *sql.Rows) ([]*PendingRelation, error) {
	var out []*PendingRelation
	for rows.Next() {
		p, err := scanPending(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, p)
	}
	return out, rows.Err()
}
