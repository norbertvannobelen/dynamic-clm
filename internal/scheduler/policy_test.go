package scheduler

import "testing"

func TestShouldRewriteImmediate(t *testing.T) {
	dc := DeviceCosts{R: 0.1, Kd: 0.001, CFixed: 10}
	st := State{ContextLength: 1000, MaxContext: 4000, DeadTokens: 100, TurnsSinceLastRewrite: 0, EstimatedSuffix: 500, EstimatedRemainingTurns: 20}
	if !ShouldRewrite(st, ImmediateRewrite(), dc) {
		t.Fatal("immediate should fire")
	}
	if ShouldRewrite(st, Baseline(), dc) {
		t.Fatal("baseline should not fire")
	}
}

func TestShouldRewriteDynCLM(t *testing.T) {
	dc := DeviceCosts{R: 0.1, Kd: 0.01, CFixed: 5}
	p := DefaultDynCLM()
	// Below beta
	st := State{ContextLength: 100, MaxContext: 4000, DeadTokens: 80, TurnsSinceLastRewrite: 10, EstimatedSuffix: 50, EstimatedRemainingTurns: 30}
	if ShouldRewrite(st, p, dc) {
		t.Fatal("should block on beta")
	}
	// Meet beta+gamma+delta; cost model should fire with high kd*dead*T
	st.ContextLength = 3000
	st.DeadTokens = 2000
	if !ShouldRewrite(st, p, dc) {
		t.Fatal("expected rewrite")
	}
}

func TestOptimalGarbage(t *testing.T) {
	dc := DeviceCosts{CFixed: 80, Kd: 0.00005}
	g := OptimalGarbage(dc, 200, 50)
	if g <= 0 {
		t.Fatalf("got %v", g)
	}
}

func TestOnlineAdapt(t *testing.T) {
	a := NewOnlineAdapter(DeviceCosts{R: 1, Kd: 1, CFixed: 10}, 0.5, 0.2)
	a.ObserveRewrite(100, 10, 1)
	if a.Estimate.CFixed <= 10 {
		t.Fatalf("expected CFixed to rise, got %v", a.Estimate.CFixed)
	}
	if !a.RhoDiverged() {
		t.Fatal("expected divergence")
	}
	p := a.ShiftedPolicy(DefaultDynCLM())
	if p.Name == DefaultDynCLM().Name {
		t.Fatal("expected adapted name")
	}
}
