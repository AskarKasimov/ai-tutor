// Package scoring normalizes results so that a competency weighs the same
// however many constituents and tasks it took (decision of 01.11.2026).
package scoring

import "math"

// CompetencyPoints is what a competency weighs in the overall score.
const CompetencyPoints = 5

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

// Overall is the score of all competencies, rounded to one decimal, its maximum
// and the percent. Without competencies there is no percent.
func Overall(competencies []float64) (score float64, maximum int, percent *int) {
	if len(competencies) == 0 {
		return 0, 0, nil
	}
	share := Mean(competencies)
	maximum = CompetencyPoints * len(competencies)
	p := Percent(share)
	return math.Round(share*float64(maximum)*10) / 10, maximum, &p
}
