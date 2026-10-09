package metrics

import "testing"

func TestCompareToBaseline(t *testing.T) {
	base := Snapshot{Arm: "baseline", AccuracyPct: 90, AvgWallClockS: 2, AvgPrefillTokens: 0, N: 10, Correct: 9}
	base.FinalizeDerived()
	arm := Snapshot{Arm: "dynclm", AccuracyPct: 100, AvgWallClockS: 1.5, AvgPrefillTokens: 100, N: 10, Correct: 10}
	arm.FinalizeDerived()
	rows := CompareToBaseline(arm, base)
	if len(rows) == 0 {
		t.Fatal("empty")
	}
	found := false
	for _, r := range rows {
		if r.Metric == "accuracy_pct" {
			found = true
			if r.Delta != 10 {
				t.Fatalf("delta=%v", r.Delta)
			}
		}
	}
	if !found {
		t.Fatal("missing accuracy_pct")
	}
}
