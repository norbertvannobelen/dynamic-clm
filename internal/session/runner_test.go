package session

import (
	"path/filepath"
	"testing"

	"github.com/norbertvannobelen/dynamic-clm/internal/scheduler"
	"github.com/norbertvannobelen/dynamic-clm/internal/tracing"
)

func TestSyntheticArms(t *testing.T) {
	dir := t.TempDir()
	rec, err := tracing.OpenRecorder(filepath.Join(dir, "t.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Close()
	dc := scheduler.DeviceCosts{R: 0.1, Kd: 0.01, CFixed: 5}
	runner := NewRunner(nil, rec, dc)

	for _, arm := range []Arm{ArmBaseline, ArmImmediateRewrite, ArmDynCLM} {
		res, err := runner.RunCodingSession(Config{
			HardwareID:   "test",
			ModelID:      "synthetic",
			TaskType:     "coding",
			Arm:          arm,
			SessionID:    string(arm) + "-0",
			MaxTurns:     40,
			MaxContext:   2048,
			Policy:       scheduler.DefaultDynCLM(),
			DeviceCosts:  dc,
			ResultsDir:   dir,
			UseLiveModel: false,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !res.Success {
			t.Fatalf("%s expected success", arm)
		}
		if arm == ArmImmediateRewrite && res.RewriteCount == 0 {
			t.Fatal("immediate should rewrite")
		}
		if arm == ArmDynCLM && res.RewriteCount == 0 {
			t.Fatalf("dynclm should rewrite at least once; parse=%v dead_carried=%d", res.MarkerParseRate, res.DeadCarriedTurnSum)
		}
		if arm == ArmBaseline && res.RewriteCount != 0 {
			t.Fatal("baseline should not rewrite")
		}
	}
}
