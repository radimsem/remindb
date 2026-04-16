package query

import (
	"database/sql"
	"math"
	"testing"
	"time"

	"github.com/radimsem/remindb/pkg/store"
)

func FuzzFillBudget(f *testing.F) {
	f.Add(100, 10, 20, 30)
	f.Add(0, 10, 20, 30)
	f.Add(-1, 10, 20, 30)
	f.Add(50, 0, 0, 0)
	f.Add(1, 100, 200, 300)
	f.Add(math.MaxInt, 1, 1, 1)
	f.Add(10, math.MaxInt, 1, 1)

	f.Fuzz(func(t *testing.T, budget, tok1, tok2, tok3 int) {
		scored := []ScoredNode{
			{Node: &store.Node{TokenCount: tok1}, Score: 3.0},
			{Node: &store.Node{TokenCount: tok2}, Score: 2.0},
			{Node: &store.Node{TokenCount: tok3}, Score: 1.0},
		}

		result := fillBudget(scored, budget)

		// Token counts can be negative (fuzzed), so only assert budget
		// invariants when all inputs are non-negative.
		if budget >= 0 && tok1 >= 0 && tok2 >= 0 && tok3 >= 0 {
			if result.TokensUsed > budget {
				t.Errorf("TokensUsed = %d exceeds budget %d", result.TokensUsed, budget)
			}
			if result.TokensUsed < 0 {
				t.Errorf("TokensUsed = %d, want >= 0", result.TokensUsed)
			}
		}
	})
}

func FuzzScoreNode(f *testing.F) {
	f.Add(1.0, 0.5, int64(0), true)
	f.Add(0.0, 0.0, int64(0), false)
	f.Add(-1.0, 0.5, int64(1744800000), true)
	f.Add(100.0, 2.0, int64(1744800000), true)
	f.Add(1.0, -1.0, int64(0), true)
	f.Add(math.Inf(1), 0.5, int64(0), false)
	f.Add(1.0, 0.5, int64(math.MaxInt64), true)
	f.Add(1.0, 0.5, int64(-1), true)

	f.Fuzz(func(t *testing.T, ftsRank, temp float64, lastAccessed int64, hasAccess bool) {
		n := &store.Node{Temperature: temp}
		if hasAccess {
			n.LastAccessed = sql.NullInt64{Int64: lastAccessed, Valid: true}
		}

		now := time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC)
		result := scoreNode(n, ftsRank, now)

		if math.IsNaN(result) {
			if !math.IsNaN(ftsRank) && !math.IsNaN(temp) {
				t.Errorf("scoreNode(fts=%g, temp=%g) = NaN with non-NaN inputs", ftsRank, temp)
			}
		}
	})
}
