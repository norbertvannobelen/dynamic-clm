package swedistill

import (
	"path/filepath"
	"testing"

	"github.com/norbertvannobelen/dynamic-clm/internal/scheduler"
)

func TestLoadFixtures(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "testdata", "swe_distill")
	fx, err := LoadFixtures(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(fx) < 6 {
		t.Fatalf("want >=6 fixtures, got %d", len(fx))
	}
}

func TestOfflineSuite(t *testing.T) {
	dir := t.TempDir()
	fxDir := filepath.Join("..", "..", "..", "testdata", "swe_distill")
	rep, err := Run(Config{
		ModelID:      "offline-test",
		ResultsDir:   dir,
		FixtureDir:   fxDir,
		UseLiveModel: false,
		DeviceCosts:  scheduler.DefaultM5MaxPrior("x").ToDeviceCosts(),
		MaxContext:   2048,
		MaxTurns:     12,
		// leave NoisePad 0 → auto-size to ≥1.4× max_context
	})
	if err != nil {
		t.Fatal(err)
	}
	bc := rep.Arms["baseline-compact"]
	dyn := rep.Arms["dynclm"]
	imm := rep.Arms["immediate-rewrite"]
	if bc.N != 6 || dyn.N != 6 || imm.N != 6 {
		t.Fatalf("arm counts: bc=%d dyn=%d imm=%d", bc.N, dyn.N, imm.N)
	}
	if bc.AvgCompacts < 1 {
		t.Fatalf("expected baseline-compact to fire compaction, got avg=%.2f", bc.AvgCompacts)
	}
	// Canary design: offline baseline loses SPEC → ~0%; marker arms keep canary → 100%.
	if bc.AccuracyPct > 0 {
		t.Fatalf("expected baseline-compact ~0%% on canary suite (got %.1f) — parametric leak?", bc.AccuracyPct)
	}
	if dyn.AccuracyPct < 100 || imm.AccuracyPct < 100 {
		t.Fatalf("expected marker arms 100%% offline: dyn=%.1f imm=%.1f", dyn.AccuracyPct, imm.AccuracyPct)
	}
}
