package diff

import (
	"encoding/binary"
	"encoding/hex"
	"sort"

	"github.com/cespare/xxhash/v2"
	"github.com/radimsem/remindb/pkg/parser"
)

// Compute a fingerprint of the entire node set by hashing sorted
// content hashes.
func CursorHash(roots []*parser.ContextNode) string {
	flat := flatten(roots)
	hashes := make([]string, len(flat))
	for i, n := range flat {
		hashes[i] = n.ContentHash
	}
	sort.Strings(hashes)

	h := xxhash.New()
	for _, s := range hashes {
		_, _ = h.WriteString(s)
	}

	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], h.Sum64())
	return hex.EncodeToString(buf[:])
}
