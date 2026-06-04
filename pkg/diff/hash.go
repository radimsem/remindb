package diff

import (
	"encoding/binary"
	"encoding/hex"
	"sort"

	"github.com/cespare/xxhash/v2"
	"github.com/radimsem/remindb/pkg/parser"
)

func CursorHash(roots []*parser.ContextNode) string {
	return CursorHashFlat(parser.Flatten(roots))
}

func CursorHashFlat(flat []*parser.ContextNode) string {
	h := xxhash.New()
	writeFlatPairs(h, flat)
	return sumHex(h)
}

// Hash a compiled post-state against its parent snapshot so a compile that lands
// on a prior content state (edit, compile, revert, compile) still yields a unique
// digest. prevHeadID is the HEAD snapshot id (monotonic, never reused).
func CursorHashForCompile(prevHeadID int64, flat []*parser.ContextNode) string {
	h := xxhash.New()
	writeID(h, prevHeadID)
	writeFlatPairs(h, flat)
	return sumHex(h)
}

// Hash a change against its parent snapshot so reverting to a prior post-state
// still yields a unique digest. prevHeadID is the HEAD snapshot id (monotonic,
// never reused), so the result is unique per snapshot even when the deltas repeat.
func CursorHashForChange(prevHeadID int64, deltas []Delta) string {
	h := xxhash.New()
	writeID(h, prevHeadID)
	writeDeltaPairs(h, deltas)
	return sumHex(h)
}

func CursorHashForRollback(prevHeadID, targetID int64, deltas []Delta) string {
	h := xxhash.New()
	writeID(h, prevHeadID)
	writeID(h, targetID)
	writeDeltaPairs(h, deltas)
	return sumHex(h)
}

// Fold a monotonic snapshot id into the digest.
func writeID(h *xxhash.Digest, id int64) {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(id))
	_, _ = h.Write(buf[:])
}

// Write the sorted id:contentHash pairs that characterize a flat post-state.
func writeFlatPairs(h *xxhash.Digest, flat []*parser.ContextNode) {
	pairs := make([]string, len(flat))
	for i, n := range flat {
		pairs[i] = n.ID + ":" + n.ContentHash
	}
	sort.Strings(pairs)

	for _, s := range pairs {
		_, _ = h.WriteString(s)
	}
}

// Write the sorted op:id:old:new pairs that characterize a delta set.
func writeDeltaPairs(h *xxhash.Digest, deltas []Delta) {
	pairs := make([]string, len(deltas))
	for i, d := range deltas {
		pairs[i] = string(d.Op) + ":" + d.NodeID + ":" + d.OldHash + ":" + d.NewHash
	}
	sort.Strings(pairs)

	for _, p := range pairs {
		_, _ = h.WriteString(p)
	}
}

func sumHex(h *xxhash.Digest) string {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], h.Sum64())
	return hex.EncodeToString(buf[:])
}
