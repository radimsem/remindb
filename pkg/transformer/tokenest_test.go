package transformer

import (
	"testing"

	"github.com/radimsem/remindb/pkg/parser"
)

func TestEstimateTokens_Empty(t *testing.T) {
	if got := estimateTokens(""); got != 0 {
		t.Errorf("estimateTokens(\"\") = %d, want 0", got)
	}
}

func TestEstimateTokens_Short(t *testing.T) {
	// "hello" = 5 bytes * 0.75 = 3.75 → ceil → 4
	if got := estimateTokens("hello"); got != 4 {
		t.Errorf("estimateTokens(\"hello\") = %d, want 4", got)
	}
}

func TestSetTokenCount(t *testing.T) {
	// 11 * 0.75 = 8.25 → 9
	n := &parser.ContextNode{Content: "hello world"}
	setTokenCount(n)
	if n.TokenCount != 9 {
		t.Errorf("TokenCount = %d, want 9", n.TokenCount)
	}
}
