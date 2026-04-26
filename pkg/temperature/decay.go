package temperature

import "math"

func decayFactor(rate, hours float64) float64 {
	if rate == 0 || hours == 0 {
		return 1.0
	}
	return math.Exp(-rate * hours)
}

const (
	// Minimum relevance weight for coldest nodes (temperature = 0).
	coldFloor = 0.3
	// Additional weight scaled by temperature.
	tempWeight = 0.7
)

// Temperature-weighted relevance: cold nodes deprioritized but still findable.
func Score(relevance, temperature float64) float64 {
	weight := coldFloor + tempWeight*temperature

	if relevance == 0 || weight == 0 {
		return 0
	}
	return relevance * weight
}
