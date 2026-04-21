package transformer

import (
	"testing"

	"github.com/radimsem/remindb/pkg/parser"
)

func TestSetTokenCount(t *testing.T) {
	// 11 * 0.25 = 2.75 → 3
	n := &parser.ContextNode{Content: "hello world"}
	setTokenCount(n)
	if n.TokenCount != 3 {
		t.Errorf("TokenCount = %d, want 3", n.TokenCount)
	}
}
