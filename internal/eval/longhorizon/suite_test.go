package longhorizon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/norbertvannobelen/dynamic-clm/internal/scheduler"
)

func TestOfflineLongHorizonShowsContextGap(t *testing.T) {
	dir := t.TempDir()
	rep, err := Run(Config{
		ModelID:      "synthetic",
		ResultsDir:   dir,
		UseLiveModel: false,
		DeviceCosts:  scheduler.DeviceCosts{R: 0.1, Kd: 0.01, CFixed: 5},
		MaxContext:   4096,
		MaxTurns:     18,
		Sessions:     1,
		ProbeEvery:   3,
		NoisePad:     800,
	})
	if err != nil {
		t.Fatal(err)
	}
	b := rep.Arms["baseline"]
	d := rep.Arms["dynclm"]
	if d.Late.AvgContextTokens >= b.Late.AvgContextTokens {
		t.Fatalf("expected dynclm late ctx < baseline: dyn=%.0f base=%.0f", d.Late.AvgContextTokens, b.Late.AvgContextTokens)
	}
	if d.Late.ProbeAccuracyPct < b.Late.ProbeAccuracyPct {
		t.Fatalf("expected dynclm late quality >= baseline: dyn=%.1f base=%.1f", d.Late.ProbeAccuracyPct, b.Late.ProbeAccuracyPct)
	}
	if _, err := os.Stat(filepath.Join(dir, "report.json")); err != nil {
		t.Fatal(err)
	}
}
