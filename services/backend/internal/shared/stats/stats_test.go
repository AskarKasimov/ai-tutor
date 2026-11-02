package stats

import "testing"

func TestMean(t *testing.T) {
	if got := Mean([]float64{1, 0.5}); got != 0.75 {
		t.Fatalf("mean = %v", got)
	}
	if got := Mean(nil); got != 0 {
		t.Fatalf("empty mean = %v", got)
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
