package tokens

import "math"

// Approximates BPE output for mixed prose/structured content (~1 token per 4 chars).
const perByte = 0.25

func Estimate(s string) int {
	if len(s) == 0 {
		return 0
	}
	return int(math.Ceil(float64(len(s)) * perByte))
}
