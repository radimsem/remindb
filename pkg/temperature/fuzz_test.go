package temperature

import (
	"math"
	"testing"
)

func FuzzDecayFactor(f *testing.F) {
	f.Add(0.1, 1.0)
	f.Add(0.0, 0.0)
	f.Add(0.1, 24.0)
	f.Add(-0.5, 1.0)
	f.Add(1.0, -10.0)
	f.Add(math.MaxFloat64, 1.0)
	f.Add(1.0, math.MaxFloat64)
	f.Add(math.SmallestNonzeroFloat64, math.SmallestNonzeroFloat64)

	f.Fuzz(func(t *testing.T, rate, hours float64) {
		result := decayFactor(rate, hours)

		if math.IsNaN(result) {
			// NaN only valid if an input is NaN.
			if !math.IsNaN(rate) && !math.IsNaN(hours) {
				t.Errorf("decayFactor(%g, %g) = NaN with non-NaN inputs", rate, hours)
			}
			return
		}

		// exp() never returns negative for finite inputs.
		if !math.IsInf(result, 0) && result < 0 {
			t.Errorf("decayFactor(%g, %g) = %g, want non-negative", rate, hours, result)
		}
	})
}

func FuzzScore(f *testing.F) {
	f.Add(1.0, 1.0)
	f.Add(1.0, 0.0)
	f.Add(1.0, 0.5)
	f.Add(0.0, 0.0)
	f.Add(-1.0, 0.5)
	f.Add(1.0, -1.0)
	f.Add(1.0, 2.0)
	f.Add(math.MaxFloat64, math.MaxFloat64)
	f.Add(math.Inf(1), 0.5)

	f.Fuzz(func(t *testing.T, relevance, temp float64) {
		result := Score(relevance, temp)

		if math.IsNaN(result) {
			if !math.IsNaN(relevance) && !math.IsNaN(temp) {
				t.Errorf("Score(%g, %g) = NaN with non-NaN inputs", relevance, temp)
			}
		}
	})
}
