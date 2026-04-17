package transformer

import (
	"github.com/radimsem/remindb/internal/contentid"
	"github.com/radimsem/remindb/pkg/parser"
)

func setContentHash(n *parser.ContextNode) {
	n.ContentHash = contentid.ContentHash(n.Content)
}

// Set n.ID from structural context and wire n.ParentID.
func setIdentity(n *parser.ContextNode, parentID string) {
	n.ID = contentid.Identify(n.SourceFile, parentID, n.Content)
	n.ParentID = parentID
}
