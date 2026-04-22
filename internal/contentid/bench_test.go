package contentid

import (
	"strings"
	"testing"
)

func BenchmarkContentHash(b *testing.B) {
	short := "User prefers verbose explanations."
	medium := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 25)
	long := strings.Repeat("Lorem ipsum dolor sit amet, consectetur adipiscing elit. ", 200)

	for _, tc := range []struct {
		name    string
		content string
	}{
		{"short/34B", short},
		{"medium/1KB", medium},
		{"long/11KB", long},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.SetBytes(int64(len(tc.content)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				ContentHash(tc.content)
			}
		})
	}
}

func BenchmarkIdentifyNode(b *testing.B) {
	source := "vault/notes/example.md"
	parent := "AbCdEf12345"

	b.SetBytes(int64(len(source) + len(parent)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		IdentifyNode(source, parent, i&0xff)
	}
}
