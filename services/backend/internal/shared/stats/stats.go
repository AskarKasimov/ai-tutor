// Package stats holds plain arithmetic over shares of a whole.
package stats

import "math"

// Mean is the average share; an empty list has none.
func Mean(shares []float64) float64 {
	if len(shares) == 0 {
		return 0
	}
	sum := 0.0
	for _, share := range shares {
		sum += share
	}
	return sum / float64(len(shares))
}

// Percent rounds a share down, so 89.9% stays 89. The small addition keeps a
// whole percent that floats store just under it.
func Percent(share float64) int {
	return int(math.Floor(share*100 + 1e-9))
}
