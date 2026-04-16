package contentid

import (
	"encoding/binary"
	"encoding/hex"

	"github.com/cespare/xxhash/v2"
)

const base62Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// Hash returns a 16-char hex content hash and an 8-char base62 node ID.
func Hash(content string) (contentHash, id string) {
	h := xxhash.Sum64String(content)

	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], h)

	contentHash = hex.EncodeToString(buf[:])
	id = encodeBase62(h)[:8]
	return contentHash, id
}

func encodeBase62(v uint64) string {
	var buf [11]byte
	for i := 10; i >= 0; i-- {
		buf[i] = base62Alphabet[v%62]
		v /= 62
	}
	return string(buf[:])
}
