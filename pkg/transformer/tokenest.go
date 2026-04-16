package transformer

import (
	"github.com/radimsem/remindb/internal/tokens"
	"github.com/radimsem/remindb/pkg/parser"
)

func setTokenCount(n *parser.ContextNode) {
	n.TokenCount = tokens.Estimate(n.Content)
}
