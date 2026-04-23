package temperature

import "testing"

func BenchmarkDecayFactor(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		decayFactor(0.05, 24.0)
	}
}

func BenchmarkScore(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		Score(2.5, 0.75)
	}
}
