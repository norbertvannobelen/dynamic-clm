package scheduler

import "math"

// DeviceCosts are measured per-device via microbenchmark (§4.3).
type DeviceCosts struct {
	R     float64 // prefill cost per token (ms or relative units)
	Kd    float64 // decode attention cost per token per turn
	CFixed float64 // fixed per-rewrite overhead
}

// Rho returns C_fixed / k_d.
func (d DeviceCosts) Rho() float64 {
	if d.Kd == 0 {
		return math.Inf(1)
	}
	return d.CFixed / d.Kd
}

// OptimalGarbage returns L - L' = sqrt(2 * C_fixed * g / (k_d * T)).
func OptimalGarbage(d DeviceCosts, g, tRemaining float64) float64 {
	if d.Kd <= 0 || tRemaining <= 0 {
		return 0
	}
	return math.Sqrt(2 * d.CFixed * g / (d.Kd * tRemaining))
}

// BreakEven reports whether a rewrite is worth it: r·S < T·d·(C - C').
func BreakEven(d DeviceCosts, suffix, deadTokens, tRemaining float64) bool {
	cRewrite := d.R * suffix
	cDefer := tRemaining * d.Kd * deadTokens
	return cRewrite < cDefer
}

// TotalSessionCost approximates Total ≈ N·C_fixed + k_d·T·avg_context + r·total_tokens.
func TotalSessionCost(d DeviceCosts, n, t, avgContext, totalTokens float64) float64 {
	return n*d.CFixed + d.Kd*t*avgContext + d.R*totalTokens
}

// RewriteCost is C_fixed + r·S.
func RewriteCost(d DeviceCosts, suffix float64) float64 {
	return d.CFixed + d.R*suffix
}

// DeferCost is T · k_d · dead_tokens.
func DeferCost(d DeviceCosts, tRemaining, deadTokens float64) float64 {
	return tRemaining * d.Kd * deadTokens
}
