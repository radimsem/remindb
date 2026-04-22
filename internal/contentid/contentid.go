package contentid

import (
	"encoding/binary"
	"encoding/hex"
	"strconv"

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

// Return an 11-char base62 node ID from structural position (stable across content edits).
func IdentifyNode(source, parent string, siblingIndex int) string {
	input := source + sep + parent + sep + strconv.Itoa(siblingIndex)
	h := xxhash.Sum64String(input)

	return encodeBase62(h)[:idLen]
}

// Return an 11-char base62 node ID from a payload under a namespace (content-addressed; identical payloads dedupe).
func IdentifyPayload(namespace, payload string) string {
	input := namespace + sep + payload
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
