package scoring

import "testing"

func TestOverallIsTheMeanOfCompetencies(t *testing.T) {
	score, maximum, percent := Overall([]float64{1, 7.0 / 9})
	if score != 8.9 || maximum != 10 || *percent != 88 {
		t.Fatalf("overall = %v/%d %d%%", score, maximum, *percent)
	}
	score, maximum, percent = Overall([]float64{1, 1, 1})
	if score != 15 || maximum != 15 || *percent != 100 {
		t.Fatalf("overall = %v/%d %d%%", score, maximum, *percent)
	}
	if _, maximum, percent = Overall(nil); maximum != 0 || percent != nil {
		t.Fatalf("empty overall = %d %v", maximum, percent)
	}
}

func TestPercentRoundsDown(t *testing.T) {
	if got := Percent(0.899); got != 89 {
		t.Fatalf("percent = %d", got)
	}
	if got := Percent(0.9); got != 90 {
		t.Fatalf("percent = %d", got)
	}
}
