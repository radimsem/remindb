package query

import (
	"context"
	"time"

	"github.com/radimsem/remindb/pkg/store"
)

type QueryStore interface {
	GetNode(ctx context.Context, id string) (*store.Node, error)

	GetAncestors(ctx context.Context, id string) ([]*store.Node, error)

	GetDescendants(ctx context.Context, id string, maxDepth int) ([]*store.Node, error)

	GetSiblings(ctx context.Context, id string) ([]*store.Node, error)

	SearchRanked(ctx context.Context, query string, limit int) ([]*store.RankedNode, error)

	GetDiffsSince(ctx context.Context, sinceSnapshotID int64) ([]*store.DiffRecord, error)
}

type Engine struct {
	store    QueryStore
	maxDepth int
}

func NewEngine(s QueryStore) *Engine {
	return &Engine{store: s, maxDepth: DefaultMaxDepth}
}

const searchLimit = 50

func (e *Engine) Fetch(ctx context.Context, anchor string, budget, depth int) (*Result, error) {
	node, err := e.store.GetNode(ctx, anchor)
	if err != nil {
		return nil, err
	}

	remaining := max(budget-node.TokenCount, 0)

	d := e.maxDepth
	if depth > 0 {
		d = clampDepth(depth)
	}

	context, err := e.collectContext(ctx, node, d)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	scored := rankNodes(context, now)
	filled := fillBudget(scored, remaining)

	anchorScored := ScoredNode{Node: node, Score: scoreNode(node, 1.0, now)}
	filled.Nodes = append([]ScoredNode{anchorScored}, filled.Nodes...)
	filled.TokensUsed += node.TokenCount

	return &filled, nil
}

func (e *Engine) Search(ctx context.Context, query string, budget int) (*Result, error) {
	ranked, err := e.store.SearchRanked(ctx, query, searchLimit)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	scored := rankSearchResults(ranked, now)
	filled := fillBudget(scored, budget)
	return &filled, nil
}

func (e *Engine) Delta(ctx context.Context, sinceSnapshot int64) ([]*store.DiffRecord, error) {
	return e.store.GetDiffsSince(ctx, sinceSnapshot)
}
