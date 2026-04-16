package temperature

import (
	"math"
	"testing"
)

func TestDecayFactor(t *testing.T) {
	// No time elapsed → factor = 1.0 (no decay).
	if f := decayFactor(0.05, 0); f != 1.0 {
		t.Errorf("decayFactor(0.05, 0) = %f, want 1.0", f)
	}

	// 1 hour at rate 0.05 → e^(-0.05) ≈ 0.9512.
	f := decayFactor(0.05, 1.0)
	want := math.Exp(-0.05)
	if math.Abs(f-want) > 1e-10 {
		t.Errorf("decayFactor(0.05, 1) = %f, want %f", f, want)
	}

	// 24 hours → e^(-1.2) ≈ 0.3012.
	f = decayFactor(0.05, 24.0)
	want = math.Exp(-1.2)
	if math.Abs(f-want) > 1e-10 {
		t.Errorf("decayFactor(0.05, 24) = %f, want %f", f, want)
	}
}

func TestScore(t *testing.T) {
	tests := []struct {
		relevance, temp, want float64
	}{
		{0.8, 1.0, 0.8},  // hot: score = relevance × 1.0
		{1.0, 0.0, 0.3},  // cold: score = relevance × 0.3
		{1.0, 0.5, 0.65}, // mid: score = relevance × 0.65
	}
	for _, tt := range tests {
		got := Score(tt.relevance, tt.temp)
		if math.Abs(got-tt.want) > 1e-10 {
			t.Errorf("Score(%g, %g) = %g, want %g", tt.relevance, tt.temp, got, tt.want)
		}
	}
}
