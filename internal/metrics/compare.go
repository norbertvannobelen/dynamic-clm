package metrics

import "fmt"

// Snapshot is one arm's aggregated efficiency/quality numbers.
type Snapshot struct {
	Arm string `json:"arm"`

	// Quality
	AccuracyPct float64 `json:"accuracy_pct"`
	N           int     `json:"n"`
	Correct     int     `json:"correct"`

	// Context
	AvgContextTokens  float64 `json:"avg_context_tokens"`
	PeakContextTokens float64 `json:"peak_context_tokens"`
	FinalContextTokens float64 `json:"final_context_tokens"`
	AvgDeadTokens     float64 `json:"avg_dead_tokens"`
	AvgLiveTokens     float64 `json:"avg_live_tokens"`

	// Tokens / compute
	AvgPrefillTokens     float64 `json:"avg_prefill_tokens"`
	AvgPromptTokens      float64 `json:"avg_prompt_tokens"`
	AvgGeneratedTokens   float64 `json:"avg_generated_tokens"`
	AvgTotalTokens       float64 `json:"avg_total_tokens"` // prompt + generated
	AvgRewrites          float64 `json:"avg_rewrites"`
	AvgDeadCarriedTurnSum float64 `json:"avg_dead_carried_turn_sum"`

	// Time
	AvgWallClockS float64 `json:"avg_wall_clock_s"`
	TotalWallClockS float64 `json:"total_wall_clock_s"`

	// Derived efficiency
	WallClockPerCorrectS float64 `json:"wall_clock_per_correct_s"`
	GeneratedPerCorrect  float64 `json:"generated_tokens_per_correct"`
	PrefillPerCorrect    float64 `json:"prefill_tokens_per_correct"`
}

// VsBaseline is every metric as absolute value, delta vs baseline, and % change vs baseline.
type VsBaseline struct {
	Metric          string  `json:"metric"`
	Baseline        float64 `json:"baseline"`
	Value           float64 `json:"value"`
	Delta           float64 `json:"delta"`            // value - baseline
	PctChange       float64 `json:"pct_change"`       // 100*(value-baseline)/baseline; 0 if baseline==0
	LowerIsBetter   bool    `json:"lower_is_better"`
	BetterThanBase  *bool   `json:"better_than_baseline,omitempty"`
}

// CompareToBaseline builds a full comparison table for one arm vs baseline.
func CompareToBaseline(arm, baseline Snapshot) []VsBaseline {
	type row struct {
		name string
		val  float64
		base float64
		low  bool // lower is better
	}
	rows := []row{
		{"accuracy_pct", arm.AccuracyPct, baseline.AccuracyPct, false},
		{"avg_context_tokens", arm.AvgContextTokens, baseline.AvgContextTokens, true},
		{"peak_context_tokens", arm.PeakContextTokens, baseline.PeakContextTokens, true},
		{"final_context_tokens", arm.FinalContextTokens, baseline.FinalContextTokens, true},
		{"avg_dead_tokens", arm.AvgDeadTokens, baseline.AvgDeadTokens, true},
		{"avg_live_tokens", arm.AvgLiveTokens, baseline.AvgLiveTokens, true},
		{"avg_prefill_tokens", arm.AvgPrefillTokens, baseline.AvgPrefillTokens, true},
		{"avg_prompt_tokens", arm.AvgPromptTokens, baseline.AvgPromptTokens, true},
		{"avg_generated_tokens", arm.AvgGeneratedTokens, baseline.AvgGeneratedTokens, true},
		{"avg_total_tokens", arm.AvgTotalTokens, baseline.AvgTotalTokens, true},
		{"avg_rewrites", arm.AvgRewrites, baseline.AvgRewrites, true},
		{"avg_dead_carried_turn_sum", arm.AvgDeadCarriedTurnSum, baseline.AvgDeadCarriedTurnSum, true},
		{"avg_wall_clock_s", arm.AvgWallClockS, baseline.AvgWallClockS, true},
		{"total_wall_clock_s", arm.TotalWallClockS, baseline.TotalWallClockS, true},
		{"wall_clock_per_correct_s", arm.WallClockPerCorrectS, baseline.WallClockPerCorrectS, true},
		{"generated_tokens_per_correct", arm.GeneratedPerCorrect, baseline.GeneratedPerCorrect, true},
		{"prefill_tokens_per_correct", arm.PrefillPerCorrect, baseline.PrefillPerCorrect, true},
	}
	out := make([]VsBaseline, 0, len(rows))
	for _, r := range rows {
		pct := 0.0
		if r.base != 0 {
			pct = 100 * (r.val - r.base) / r.base
		} else if r.val != 0 {
			pct = 100 // baseline 0, arm nonzero
		}
		var better *bool
		if r.base != 0 || r.val != 0 {
			b := false
			if r.low {
				b = r.val < r.base || (r.base == 0 && r.val == 0)
				if r.base == 0 && r.val > 0 {
					b = false // more cost than baseline's zero
				}
				if r.base == 0 && r.val == 0 {
					b = true // tie at zero
				}
			} else {
				b = r.val > r.base
			}
			// tie
			if r.val == r.base {
				b = true // not worse
			}
			better = &b
		}
		out = append(out, VsBaseline{
			Metric:         r.name,
			Baseline:       r.base,
			Value:          r.val,
			Delta:          r.val - r.base,
			PctChange:      pct,
			LowerIsBetter:  r.low,
			BetterThanBase: better,
		})
	}
	return out
}

// FinalizeDerived fills per-correct ratios.
func (s *Snapshot) FinalizeDerived() {
	if s.Correct > 0 {
		s.WallClockPerCorrectS = s.TotalWallClockS / float64(s.Correct)
		s.GeneratedPerCorrect = (s.AvgGeneratedTokens * float64(s.N)) / float64(s.Correct)
		s.PrefillPerCorrect = (s.AvgPrefillTokens * float64(s.N)) / float64(s.Correct)
	}
	if s.N > 0 {
		s.AvgTotalTokens = s.AvgPromptTokens + s.AvgGeneratedTokens
	}
}

// FormatPct formats a signed percent for markdown.
func FormatPct(v float64) string {
	return fmt.Sprintf("%+.1f%%", v)
}

// FormatDelta formats a signed absolute delta.
func FormatDelta(v float64) string {
	return fmt.Sprintf("%+.2f", v)
}
