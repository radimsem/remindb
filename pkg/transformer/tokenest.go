package transformer

import (
	"math"

	"github.com/radimsem/remindb/pkg/parser"
)

// Conservative tokens-per-byte ratio for mixed content (English + structured data).
const tokensPerByte = 0.75

func setTokenCount(n *parser.ContextNode) {
	n.TokenCount = estimateTokens(n.Content)
}

func estimateTokens(s string) int {
	if len(s) == 0 {
		return 0
	}
	return int(math.Ceil(float64(len(s)) * tokensPerByte))
}
