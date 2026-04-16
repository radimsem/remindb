// Package diff computes minimal deltas between the current enriched AST and a
// previous snapshot.
package diff

type Op string

const (
	OpAdd Op = "add"
	OpMod Op = "mod"
	OpRem Op = "rem"
)

type Delta struct {
	NodeID     string
	OldHash    string
	NewHash    string
	OldContent string
	NewContent string
	Op
}

// Node's hash+content pair from the previous snapshot.
type NodeState struct {
	Hash    string
	Content string
}
