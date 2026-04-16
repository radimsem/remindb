package temperature

import "math"

func decayFactor(rate, hours float64) float64 {
	return math.Exp(-rate * hours)
}

func boost(current, amount float64) float64 {
	return math.Min(1.0, current+amount)
}

const (
	// Minimum relevance weight for coldest nodes (temperature = 0).
	coldFloor = 0.3
	// Additional weight scaled by temperature.
	tempWeight = 0.7
)

// Temperature-weighted relevance: cold nodes deprioritized but still findable.
func Score(relevance, temperature float64) float64 {
	return relevance * (coldFloor + tempWeight*temperature)
}
