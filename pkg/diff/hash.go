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
	pairs := make([]string, len(flat))
	for i, n := range flat {
		pairs[i] = n.ID + ":" + n.ContentHash
	}
	sort.Strings(pairs)

	h := xxhash.New()
	for _, s := range pairs {
		_, _ = h.WriteString(s)
	}

	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], h.Sum64())
	return hex.EncodeToString(buf[:])
}

// Hash a delta set when roots alone don't characterize the post-state (e.g., deletions).
func CursorHashForDeltas(deltas []Delta) string {
	pairs := make([]string, len(deltas))
	for i, d := range deltas {
		pairs[i] = string(d.Op) + ":" + d.NodeID + ":" + d.OldHash + ":" + d.NewHash
	}

	sort.Strings(pairs)

	h := xxhash.New()
	for _, p := range pairs {
		_, _ = h.WriteString(p)
	}

	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], h.Sum64())
	return hex.EncodeToString(buf[:])
}

func CursorHashForRollback(prevHeadID, targetID int64, deltas []Delta) string {
	h := xxhash.New()

	var idBuf [8]byte
	binary.BigEndian.PutUint64(idBuf[:], uint64(prevHeadID))
	_, _ = h.Write(idBuf[:])

	binary.BigEndian.PutUint64(idBuf[:], uint64(targetID))
	_, _ = h.Write(idBuf[:])

	pairs := make([]string, len(deltas))
	for i, d := range deltas {
		pairs[i] = string(d.Op) + ":" + d.NodeID + ":" + d.OldHash + ":" + d.NewHash
	}

	sort.Strings(pairs)
	for _, p := range pairs {
		_, _ = h.WriteString(p)
	}

	var out [8]byte
	binary.BigEndian.PutUint64(out[:], h.Sum64())
	return hex.EncodeToString(out[:])
}
