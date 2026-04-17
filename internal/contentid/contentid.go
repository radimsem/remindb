package contentid

import (
	"encoding/binary"
	"encoding/hex"

	"github.com/cespare/xxhash/v2"
)

const (
	base62Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	idLen          = 11
	sep            = "\x00"
)

// Return a 16-char hex content hash used for change detection.
func ContentHash(content string) string {
	h := xxhash.Sum64String(content)

	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], h)

	return hex.EncodeToString(buf[:])
}

// Return an 11-char base62 node ID derived from structural context.
func Identify(source, parent, content string) string {
	input := source + sep + parent + sep + content
	h := xxhash.Sum64String(input)

	return encodeBase62(h)[:idLen]
}

func encodeBase62(v uint64) string {
	var buf [11]byte
	for i := 10; i >= 0; i-- {
		buf[i] = base62Alphabet[v%62]
		v /= 62
	}
	return string(buf[:])
}
