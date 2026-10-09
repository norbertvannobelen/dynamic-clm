package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/norbertvannobelen/dynamic-clm/internal/eval/longhorizon"
	"github.com/norbertvannobelen/dynamic-clm/internal/scheduler"
)

func main() {
	model := flag.String("model", "Qwen3.8-27B-4bit", "model id")
	out := flag.String("out", "docs/longhorizon-report.md", "markdown report")
	results := flag.String("results", "results/longhorizon", "results dir")
	turns := flag.Int("turns", 24, "turns per session")
	sessions := flag.Int("sessions", 3, "sessions per arm")
	probeEvery := flag.Int("probe-every", 4, "probe interval")
	offline := flag.Bool("offline", false, "synthetic (no oMLX)")
	fresh := flag.Bool("fresh", false, "delete prior cells")
	flag.Parse()

	if *fresh {
		_ = os.RemoveAll(*results)
	}

	cal := scheduler.DefaultM5MaxPrior(*model)
	if c, err := scheduler.LoadCalibration("configs/calibration/m5max-qwen38.json"); err == nil {
		cal = c
	}

	rep, err := longhorizon.Run(longhorizon.Config{
		ModelID:      *model,
		HardwareID:   "apple-m5-max",
		ResultsDir:   *results,
		UseLiveModel: !*offline,
		DeviceCosts:  cal.ToDeviceCosts(),
		MaxContext:   8192,
		MaxTurns:     *turns,
		Sessions:     *sessions,
		ProbeEvery:   *probeEvery,
		NoisePad:     1200,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := longhorizon.WriteMarkdownReport(*out, rep); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	b, d := rep.Arms["baseline"], rep.Arms["dynclm"]
	imm := rep.Arms["immediate-rewrite"]
	summary := map[string]any{
		"live": rep.Live,
		"late_probe_accuracy": map[string]float64{
			"baseline": b.Late.ProbeAccuracyPct,
			"dynclm":   d.Late.ProbeAccuracyPct,
			"delta_pp": d.Late.ProbeAccuracyPct - b.Late.ProbeAccuracyPct,
		},
		"late_avg_context": map[string]float64{
			"baseline": b.Late.AvgContextTokens,
			"dynclm":   d.Late.AvgContextTokens,
			"pct":      pct(d.Late.AvgContextTokens, b.Late.AvgContextTokens),
		},
		"late_steady_wall_s": map[string]float64{
			"baseline":  b.LateSteady.AvgWallClockS,
			"dynclm":    d.LateSteady.AvgWallClockS,
			"immediate": imm.LateSteady.AvgWallClockS,
			"dyn_pct_vs_base": pct(d.LateSteady.AvgWallClockS, b.LateSteady.AvgWallClockS),
			"imm_pct_vs_base": pct(imm.LateSteady.AvgWallClockS, b.LateSteady.AvgWallClockS),
		},
		"late_steady_gen_tok_s": map[string]float64{
			"baseline": b.LateSteady.AvgGenTokensPerSec,
			"dynclm":   d.LateSteady.AvgGenTokensPerSec,
			"immediate": imm.LateSteady.AvgGenTokensPerSec,
			"pct_vs_base": pct(d.LateSteady.AvgGenTokensPerSec, b.LateSteady.AvgGenTokensPerSec),
		},
	}
	enc, _ := json.MarshalIndent(summary, "", "  ")
	fmt.Println(string(enc))
	fmt.Println("long-horizon report →", *out)
}

func pct(val, base float64) float64 {
	if base == 0 {
		return 0
	}
	return 100 * (val - base) / base
}
