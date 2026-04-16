package transformer

import (
	"encoding/binary"
	"encoding/hex"

	"github.com/cespare/xxhash/v2"
	"github.com/radimsem/remindb/pkg/parser"
)

const base62Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// Hash Content with xxhash64, set ContentHash (16 hex chars) and ID (8 base62 chars).
func setAnchor(n *parser.ContextNode) {
	h := xxhash.Sum64String(n.Content)

	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], h)
	n.ContentHash = hex.EncodeToString(buf[:])
	n.ID = encodeBase62(h)[:8]
}

func encodeBase62(v uint64) string {
	var buf [11]byte
	for i := 10; i >= 0; i-- {
		buf[i] = base62Alphabet[v%62]
		v /= 62
	}
	return string(buf[:])
}
