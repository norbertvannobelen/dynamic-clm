package ablations

import "github.com/norbertvannobelen/dynamic-clm/internal/scheduler"

// KnobSweep returns independent β/γ/Δ variants for ablation runs.
func KnobSweep() []scheduler.Policy {
	var out []scheduler.Policy
	for _, b := range []float64{0.4, 0.6, 0.8} {
		out = append(out, scheduler.Policy{Name: "ablate-beta", Beta: b, Gamma: 0.5, Delta: 3})
	}
	for _, g := range []float64{0.3, 0.5, 0.6} {
		out = append(out, scheduler.Policy{Name: "ablate-gamma", Beta: 0.7, Gamma: g, Delta: 3})
	}
	for _, d := range []int{3, 5, 10} {
		out = append(out, scheduler.Policy{Name: "ablate-delta", Beta: 0.6, Gamma: 0.5, Delta: d})
	}
	return out
}

// MarkerPositions for position ablation.
func MarkerPositions() []string { return []string{"tail", "middle"} }

// MarkerSimilarities for poisoning ablation.
func MarkerSimilarities() []string { return []string{"identical", "distinct"} }
