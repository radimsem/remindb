package query

import (
	"context"
	"slices"
	"time"

	"github.com/radimsem/remindb/pkg/store"
)

type QueryStore interface {
	GetNode(ctx context.Context, id string) (*store.Node, error)

	GetNodesByIDs(ctx context.Context, ids []string) ([]*store.Node, error)

	GetAncestors(ctx context.Context, id string) ([]*store.Node, error)

	GetDescendants(ctx context.Context, id string, maxDepth int) ([]*store.Node, error)

	GetSiblings(ctx context.Context, id string) ([]*store.Node, error)

	SearchRanked(ctx context.Context, query string, limit int) ([]*store.RankedNode, error)

	GetDiffsSince(ctx context.Context, sinceSnapshotID int64) ([]*store.DiffRecord, error)

	GetDiffsBetween(ctx context.Context, fromSnapshotID, toSnapshotID int64) ([]*store.DiffRecord, error)
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

	// Ascending score so the anchor lands at the prompt tail where LLM attention peaks.
	slices.Reverse(filled.Nodes)

	anchorScored := ScoredNode{Node: node, Score: scoreNode(node, 1.0, now)}
	filled.Nodes = append(filled.Nodes, anchorScored)
	filled.TokensUsed += node.TokenCount

	return &filled, nil
}

func (e *Engine) FetchBatch(ctx context.Context, ids []string, budget int) (*Result, []string, error) {
	if len(ids) == 0 {
		return &Result{}, nil, nil
	}

	nodes, err := e.store.GetNodesByIDs(ctx, ids)
	if err != nil {
		return nil, nil, err
	}

	byID := make(map[string]*store.Node, len(nodes))
	for _, n := range nodes {
		byID[n.ID] = n
	}

	now := time.Now()
	ordered := make([]ScoredNode, 0, len(ids))
	var missing []string
	seen := make(map[string]bool, len(ids))

	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true

		n, ok := byID[id]
		if !ok {
			missing = append(missing, id)
			continue
		}
		ordered = append(ordered, ScoredNode{Node: n, Score: scoreNode(n, 1.0, now)})
	}

	if budget <= 0 {
		used := 0
		for _, sn := range ordered {
			used += sn.Node.TokenCount
		}
		return &Result{Nodes: ordered, TokensUsed: used}, missing, nil
	}

	filled := fillBudget(ordered, budget)
	return &filled, missing, nil
}

func (e *Engine) Search(ctx context.Context, query string, budget int) (*Result, error) {
	ranked, err := e.store.SearchRanked(ctx, query, searchLimit)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	scored := rankSearchResults(ranked, now)
	filled := fillBudget(scored, budget)

	// Ascending score so the top hit lands at the prompt tail where LLM attention peaks.
	slices.Reverse(filled.Nodes)

	return &filled, nil
}

func (e *Engine) Delta(ctx context.Context, sinceSnapshot int64) ([]*store.DiffRecord, error) {
	return e.store.GetDiffsSince(ctx, sinceSnapshot)
}

func (e *Engine) Diff(ctx context.Context, fromSnapshot, toSnapshot int64) ([]*store.DiffRecord, error) {
	raw, err := e.store.GetDiffsBetween(ctx, fromSnapshot, toSnapshot)
	if err != nil {
		return nil, err
	}
	return consolidateDiffs(raw), nil
}
