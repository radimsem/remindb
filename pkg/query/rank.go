package query

import (
	"sort"
	"time"

	"github.com/radimsem/remindb/pkg/store"
	"github.com/radimsem/remindb/pkg/temperature"
)

type ScoredNode struct {
	Node  *store.Node
	Score float64
}

const recencyNormHours = 24.0

func rankNodes(nodes []*store.Node, now time.Time) []ScoredNode {
	scored := make([]ScoredNode, 0, len(nodes))
	for _, n := range nodes {
		s := scoreNode(n, 1.0, now)
		scored = append(scored, ScoredNode{Node: n, Score: s})
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})
	return scored
}

func rankSearchResults(results []*store.RankedNode, now time.Time) []ScoredNode {
	scored := make([]ScoredNode, 0, len(results))
	for _, r := range results {
		// BM25 returns negative; negate so higher = better
		ftsScore := -r.Rank
		s := scoreNode(r.Node, ftsScore, now)
		scored = append(scored, ScoredNode{Node: r.Node, Score: s})
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})
	return scored
}

func scoreNode(n *store.Node, ftsRank float64, now time.Time) float64 {
	tempScore := temperature.Score(1.0, n.Temperature)
	recency := recencyBoost(n, now)
	return ftsRank * tempScore * recency
}

func recencyBoost(n *store.Node, now time.Time) float64 {
	if !n.LastAccessed.Valid {
		return 1.0
	}

	hours := now.Sub(time.Unix(n.LastAccessed.Int64, 0)).Hours()
	if hours < 0 {
		hours = 0
	}
	return 1.0 / (1.0 + hours/recencyNormHours)
}
