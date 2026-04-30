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
