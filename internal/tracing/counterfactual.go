package tracing

import "github.com/norbertvannobelen/dynamic-clm/internal/scheduler"

// EstimateDeferVsRewrite computes counterfactual costs at a decision point.
func EstimateDeferVsRewrite(s scheduler.State, dc scheduler.DeviceCosts) (wouldRewrite bool, rewriteCost, deferCost float64) {
	tRem := float64(s.EstimatedRemainingTurns)
	if tRem <= 0 {
		tRem = 1
	}
	rewriteCost = scheduler.RewriteCost(dc, float64(s.EstimatedSuffix))
	deferCost = scheduler.DeferCost(dc, tRem, float64(s.DeadTokens))
	wouldRewrite = rewriteCost < deferCost && s.DeadTokens > 0
	return wouldRewrite, rewriteCost, deferCost
}
