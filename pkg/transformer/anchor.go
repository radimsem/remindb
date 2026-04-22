package transformer

import (
	"github.com/radimsem/remindb/internal/contentid"
	"github.com/radimsem/remindb/pkg/parser"
)

func setContentHash(n *parser.ContextNode) {
	n.ContentHash = contentid.ContentHash(n.Content)
}

// Set n.ID from structural position and wire n.ParentID.
func setIdentity(n *parser.ContextNode, parentID string, siblingIndex int) {
	n.ID = contentid.IdentifyNode(n.SourceFile, parentID, siblingIndex)
	n.ParentID = parentID
}
