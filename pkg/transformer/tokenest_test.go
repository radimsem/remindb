package transformer

import (
	"testing"

	"github.com/radimsem/remindb/pkg/parser"
)

func TestSetTokenCount(t *testing.T) {
	// 11 * 0.75 = 8.25 → 9
	n := &parser.ContextNode{Content: "hello world"}
	setTokenCount(n)
	if n.TokenCount != 9 {
		t.Errorf("TokenCount = %d, want 9", n.TokenCount)
	}
}
