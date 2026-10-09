package longhorizon

import (
	"fmt"
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

func TestOfflineCompactionMatrix(t *testing.T) {
	dir := t.TempDir()
	m, err := RunCompactionMatrix(Config{
		ModelID:      "synthetic",
		ResultsDir:   dir,
		UseLiveModel: false,
		DeviceCosts:  scheduler.DeviceCosts{R: 0.1, Kd: 0.01, CFixed: 5},
		MaxTurns:     16,
		Sessions:     1,
		ProbeEvery:   4,
	}, []int{2048, 4096})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.MaxContexts) != 2 {
		t.Fatalf("expected 2 limits, got %v", m.MaxContexts)
	}
	for _, mc := range m.MaxContexts {
		key := fmt.Sprintf("%d", mc)
		arms := m.ByMaxContext[key]
		bc := arms["baseline-compact"].(map[string]any)
		dyn := arms["dynclm"].(map[string]any)
		if dyn["late_probe_accuracy"].(float64) < bc["late_probe_accuracy"].(float64) {
			t.Fatalf("at %d: expected dynclm quality >= compact baseline", mc)
		}
		if dyn["late_avg_context"].(float64) >= bc["late_avg_context"].(float64)*1.5 {
			t.Fatalf("at %d: expected dynclm ctx not much larger than compact: dyn=%v bc=%v",
				mc, dyn["late_avg_context"], bc["late_avg_context"])
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "matrix_report.json")); err != nil {
		t.Fatal(err)
	}
}
