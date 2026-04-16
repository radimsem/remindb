package tokens

import "math"

// Conservative tokens-per-byte ratio for mixed content.
const perByte = 0.75

func Estimate(s string) int {
	if len(s) == 0 {
		return 0
	}
	return int(math.Ceil(float64(len(s)) * perByte))
}
