package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/norbertvannobelen/dynamic-clm/internal/eval/quality"
	"github.com/norbertvannobelen/dynamic-clm/internal/scheduler"
)

func main() {
	model := flag.String("model", "Qwen3.8-27B-4bit", "model id")
	out := flag.String("out", "docs/quality-report.md", "markdown report path")
	results := flag.String("results", "results/quality", "results directory")
	offline := flag.Bool("offline", false, "deprecated alias for --allow-synthetic")
	allowSynthetic := flag.Bool("allow-synthetic", false, "permit synthetic answers (no oMLX)")
	fresh := flag.Bool("fresh", false, "delete prior quality cells before run")
	flag.Parse()

	if *fresh {
		_ = os.RemoveAll(*results)
	}
	if *offline && !*allowSynthetic {
		fatal(fmt.Errorf("--offline is disabled; pass --allow-synthetic for a non-live run"))
	}
	live := !*allowSynthetic
	if !live {
		fatal(fmt.Errorf("live evaluation required: refusing synthetic quality run (pass --allow-synthetic to override)"))
	}

	cal := scheduler.DefaultM5MaxPrior(*model)
	if c, err := scheduler.LoadCalibration("configs/calibration/m5max-qwen38.json"); err == nil {
		cal = c
	}

	rep, err := quality.Run(quality.Config{
		ModelID:      *model,
		HardwareID:   "apple-m5-max",
		ResultsDir:   *results,
		UseLiveModel: live,
		DeviceCosts:  cal.ToDeviceCosts(),
		MaxContext:   8192,
	})
	if err != nil {
		fatal(err)
	}
	if err := quality.WriteMarkdownReport(*out, rep); err != nil {
		fatal(err)
	}
	b, _ := json.MarshalIndent(map[string]any{
		"accuracy": map[string]float64{
			"baseline":           rep.Arms["baseline"].AccuracyPct,
			"immediate-rewrite":  rep.Arms["immediate-rewrite"].AccuracyPct,
			"dynclm":             rep.Arms["dynclm"].AccuracyPct,
		},
		"delta_vs_baseline_pp":  rep.DeltaVsBaseline,
		"delta_vs_immediate_pp": rep.DeltaVsImmediate,
		"prefill_cut_vs_immediate_pct": rep.PrefillCutVsImm,
		"live":                         rep.Live,
	}, "", "  ")
	fmt.Println(string(b))
	fmt.Println("quality report →", *out)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
