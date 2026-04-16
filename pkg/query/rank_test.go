package query

import (
	"database/sql"
	"math"
	"testing"
	"time"

	"github.com/radimsem/remindb/pkg/store"
)

func TestRecencyBoost_NeverAccessed(t *testing.T) {
	n := &store.Node{}
	if b := recencyBoost(n, time.Now()); b != 1.0 {
		t.Errorf("recencyBoost = %f, want 1.0", b)
	}
}

func TestRecencyBoost_JustAccessed(t *testing.T) {
	now := time.Now()
	n := &store.Node{LastAccessed: sql.NullInt64{Int64: now.Unix(), Valid: true}}

	b := recencyBoost(n, now)
	if math.Abs(b-1.0) > 0.01 {
		t.Errorf("recencyBoost = %f, want ~1.0", b)
	}
}

func TestRecencyBoost_24HoursAgo(t *testing.T) {
	now := time.Now()
	accessed := now.Add(-24 * time.Hour)
	n := &store.Node{LastAccessed: sql.NullInt64{Int64: accessed.Unix(), Valid: true}}

	b := recencyBoost(n, now)
	if math.Abs(b-0.5) > 0.01 {
		t.Errorf("recencyBoost = %f, want ~0.5", b)
	}
}

func TestRankNodes_SortsByScore(t *testing.T) {
	now := time.Now()
	nodes := []*store.Node{
		{ID: "cold0001", Temperature: 0.1, TokenCount: 10},
		{ID: "hot00001", Temperature: 0.9, TokenCount: 10},
	}

	scored := rankNodes(nodes, now)
	if scored[0].Node.ID != "hot00001" {
		t.Errorf("first = %q, want hot00001", scored[0].Node.ID)
	}
	if scored[1].Node.ID != "cold0001" {
		t.Errorf("second = %q, want cold0001", scored[1].Node.ID)
	}
}

func TestScoreNode_CombinesFactors(t *testing.T) {
	now := time.Now()
	n := &store.Node{Temperature: 1.0}

	s := scoreNode(n, 2.0, now)
	// ftsRank=2.0, tempScore=1.0*(0.3+0.7*1.0)=1.0, recency=1.0 → 2.0
	if math.Abs(s-2.0) > 1e-10 {
		t.Errorf("score = %g, want 2.0", s)
	}
}
