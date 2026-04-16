package query

import (
	"testing"

	"github.com/radimsem/remindb/pkg/store"
)

func TestFillBudget_FitsAll(t *testing.T) {
	scored := []ScoredNode{
		{Node: &store.Node{TokenCount: 10}, Score: 3.0},
		{Node: &store.Node{TokenCount: 20}, Score: 2.0},
	}

	result := fillBudget(scored, 100)
	if len(result.Nodes) != 2 {
		t.Errorf("len = %d, want 2", len(result.Nodes))
	}
	if result.TokensUsed != 30 {
		t.Errorf("used = %d, want 30", result.TokensUsed)
	}
}

func TestFillBudget_SkipsLargeNode(t *testing.T) {
	scored := []ScoredNode{
		{Node: &store.Node{ID: "big00001", TokenCount: 100}, Score: 3.0},
		{Node: &store.Node{ID: "sml00001", TokenCount: 10}, Score: 2.0},
		{Node: &store.Node{ID: "sml00002", TokenCount: 10}, Score: 1.0},
	}

	result := fillBudget(scored, 25)
	if len(result.Nodes) != 2 {
		t.Errorf("len = %d, want 2 (skip big, fit both small)", len(result.Nodes))
	}
	if result.TokensUsed != 20 {
		t.Errorf("used = %d, want 20", result.TokensUsed)
	}
}

func TestFillBudget_ZeroBudget(t *testing.T) {
	scored := []ScoredNode{
		{Node: &store.Node{TokenCount: 10}, Score: 1.0},
	}

	result := fillBudget(scored, 0)
	if len(result.Nodes) != 0 {
		t.Errorf("len = %d, want 0", len(result.Nodes))
	}
}

func TestFillBudget_Empty(t *testing.T) {
	result := fillBudget(nil, 1000)
	if len(result.Nodes) != 0 {
		t.Errorf("len = %d, want 0", len(result.Nodes))
	}
}
