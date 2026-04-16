package temperature

import "testing"

func BenchmarkDecayFactor(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		decayFactor(0.05, 24.0)
	}
}

func BenchmarkScore(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Score(2.5, 0.75)
	}
}
