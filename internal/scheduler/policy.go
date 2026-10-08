package scheduler

// Policy is the 3-knob trigger policy (§4.1).
type Policy struct {
	Name  string  `json:"name" yaml:"name"`
	Beta  float64 `json:"beta" yaml:"beta"`   // rewrite when C > β · max_context
	Gamma float64 `json:"gamma" yaml:"gamma"` // rewrite when dead/C > γ
	Delta int     `json:"delta" yaml:"delta"` // min turns between rewrites
}

// DefaultDynCLM is the gate default (β=0.6, γ=0.5, Δ=3).
func DefaultDynCLM() Policy {
	return Policy{Name: "dynclm-default", Beta: 0.6, Gamma: 0.5, Delta: 3}
}

// ImmediateRewrite is the CLM-like arm: rewrite on every edit (Δ=0, no β/γ gates).
func ImmediateRewrite() Policy {
	return Policy{Name: "immediate-rewrite", Beta: 0, Gamma: 0, Delta: 0}
}

// Baseline is a no-op policy (never rewrite).
func Baseline() Policy {
	return Policy{Name: "baseline", Beta: 2, Gamma: 2, Delta: 1_000_000}
}

// State is the observable session state for the decision rule.
type State struct {
	ContextLength         int
	MaxContext            int
	DeadTokens            int
	TurnsSinceLastRewrite int
	EstimatedSuffix       int
	EstimatedRemainingTurns int
}

// ShouldRewrite implements §4.2.
func ShouldRewrite(s State, p Policy, dc DeviceCosts) bool {
	// Immediate-rewrite arm: fire whenever there is garbage.
	if p.Name == "immediate-rewrite" {
		return s.DeadTokens > 0
	}
	if p.Delta > 0 && s.TurnsSinceLastRewrite < p.Delta {
		return false
	}
	if p.Beta > 0 && float64(s.ContextLength) < p.Beta*float64(s.MaxContext) {
		return false
	}
	if p.Gamma > 0 && s.ContextLength > 0 {
		if float64(s.DeadTokens)/float64(s.ContextLength) < p.Gamma {
			return false
		}
	}
	if s.DeadTokens == 0 {
		return false
	}
	// Floor T so late-session turns still account for post-session carry risk.
	tRem := float64(s.EstimatedRemainingTurns)
	if tRem < 10 {
		tRem = 10
	}
	cRewrite := RewriteCost(dc, float64(s.EstimatedSuffix))
	cDefer := DeferCost(dc, tRem, float64(s.DeadTokens))
	if cRewrite < cDefer {
		return true
	}
	// Also fire when accumulated garbage exceeds the optimal deferral window.
	opt := OptimalGarbage(dc, 200, tRem)
	return float64(s.DeadTokens) >= opt && opt > 0
}

// GatePolicies returns the five learning-set policies from §6.3.
func GatePolicies() []Policy {
	return []Policy{
		{Name: "beta04", Beta: 0.4, Gamma: 0.5, Delta: 3},
		{Name: "beta06", Beta: 0.6, Gamma: 0.5, Delta: 3},
		{Name: "beta08", Beta: 0.8, Gamma: 0.5, Delta: 3},
		{Name: "gamma03", Beta: 0.7, Gamma: 0.3, Delta: 3},
		{Name: "gamma06", Beta: 0.7, Gamma: 0.6, Delta: 3},
		{Name: "delta10", Beta: 0.6, Gamma: 0.5, Delta: 10},
	}
}
