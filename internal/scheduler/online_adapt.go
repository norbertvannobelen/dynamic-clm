package scheduler

import "math"

// OnlineAdapter updates local C_fixed/k_d with a small learning rate (§4.4).
type OnlineAdapter struct {
	Prior      DeviceCosts
	Estimate   DeviceCosts
	LR         float64
	Divergence float64 // relative threshold to shift policy
	Updates    int
}

// NewOnlineAdapter starts from an offline prior.
func NewOnlineAdapter(prior DeviceCosts, lr, divergence float64) *OnlineAdapter {
	if lr <= 0 {
		lr = 0.1
	}
	if divergence <= 0 {
		divergence = 0.5
	}
	return &OnlineAdapter{
		Prior:      prior,
		Estimate:   prior,
		LR:         lr,
		Divergence: divergence,
	}
}

// ObserveRewrite folds a realized rewrite observation into the estimate.
func (a *OnlineAdapter) ObserveRewrite(observedCFixed, observedSuffix, r float64) {
	if r <= 0 {
		r = a.Estimate.R
	}
	// Blend C_fixed; keep r/kd unless separately measured.
	a.Estimate.CFixed = (1-a.LR)*a.Estimate.CFixed + a.LR*observedCFixed
	if observedSuffix > 0 && r > 0 {
		// mild update on r from suffix timing is left to calibration; noop here
		_ = observedSuffix
	}
	a.Updates++
}

// RhoDiverged reports whether estimate diverged from prior beyond threshold.
func (a *OnlineAdapter) RhoDiverged() bool {
	p := a.Prior.Rho()
	e := a.Estimate.Rho()
	if math.IsInf(p, 0) || math.IsInf(e, 0) || p == 0 {
		return false
	}
	rel := math.Abs(e-p) / math.Abs(p)
	return rel > a.Divergence
}

// ShiftedPolicy nudges β/γ when rho diverges (higher rho → defer more → higher gamma).
func (a *OnlineAdapter) ShiftedPolicy(base Policy) Policy {
	if !a.RhoDiverged() {
		return base
	}
	p := base
	ratio := a.Estimate.Rho() / a.Prior.Rho()
	if ratio > 1 {
		// C_fixed relatively larger → defer more
		p.Gamma = math.Min(0.8, p.Gamma*1.1)
		p.Delta = int(math.Min(20, float64(p.Delta)+1))
	} else {
		p.Gamma = math.Max(0.2, p.Gamma*0.9)
		p.Delta = int(math.Max(1, float64(p.Delta)-1))
	}
	p.Name = base.Name + "-adapted"
	return p
}
