package transformer

import (
	"github.com/radimsem/remindb/internal/contentid"
	"github.com/radimsem/remindb/pkg/parser"
)

func setAnchor(n *parser.ContextNode) {
	n.ContentHash, n.ID = contentid.Hash(n.Content)
}
