package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/norbertvannobelen/dynamic-clm/internal/eval/runarchive"
	"github.com/norbertvannobelen/dynamic-clm/internal/eval/swedistill"
	"github.com/norbertvannobelen/dynamic-clm/internal/scheduler"
)

func main() {
	model := flag.String("model", "Qwen3.8-27B-4bit", "model id")
	out := flag.String("out", "docs/swe-distill-report.md", "markdown report path")
	results := flag.String("results", "", "results dir (default: new run archive suite dir)")
	fixtures := flag.String("fixtures", "testdata/swe_distill", "fixture directory")
	turns := flag.Int("turns", 28, "turns per task")
	maxContext := flag.Int("max-context", 16384, "shared max context (forces compact/reclaim)")
	noisePad := flag.Int("noise-pad", 0, "chars of pad per obsolete dump (0=auto ~1.4× max_context)")
	fresh := flag.Bool("fresh", false, "delete prior cells in results dir")
	allowSynthetic := flag.Bool("allow-synthetic", false, "permit synthetic (no oMLX)")
	runRoot := flag.String("run-root", "", "attach to existing results/runs/<id> parent")
	flag.Parse()

	live := !*allowSynthetic
	if !live {
		fmt.Fprintln(os.Stderr, "live evaluation required: refusing synthetic swe-distill (pass --allow-synthetic to override)")
		os.Exit(1)
	}

	resultsDir := *results
	var run *runarchive.Run
	var err error
	if *runRoot != "" {
		run, err = runarchive.Open(*runRoot)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		resultsDir, err = run.SuiteDir("swe_distill")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	} else if resultsDir == "" {
		run, err = runarchive.Begin("results", live, "SWE-Distill Baseline+compact vs DynCLM vs CLM")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		resultsDir, err = run.SuiteDir("swe_distill")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	if *fresh {
		_ = os.RemoveAll(filepath.Join(resultsDir, "cells"))
	}

	cal := scheduler.DefaultM5MaxPrior(*model)
	if c, err := scheduler.LoadCalibration("configs/calibration/m5max-qwen38.json"); err == nil {
		cal = c
	}

	cfg := swedistill.Config{
		ModelID:      *model,
		HardwareID:   "apple-m5-max",
		ResultsDir:   resultsDir,
		FixtureDir:   *fixtures,
		UseLiveModel: live,
		DeviceCosts:  cal.ToDeviceCosts(),
		MaxContext:   *maxContext,
		MaxTurns:     *turns,
		NoisePad:     *noisePad,
	}

	rep, err := swedistill.Run(cfg)
	if err != nil {
		if run != nil {
			_ = run.Finish(false, err.Error())
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	mdPath := *out
	if err := swedistill.WriteMarkdownReport(mdPath, rep); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_ = swedistill.WriteMarkdownReport(filepath.Join(resultsDir, "swe-distill-report.md"), rep)

	if run != nil {
		_ = run.Finish(true, "SWE-Distill complete")
		fmt.Println("RUN_ID=" + run.ID)
	}

	enc, _ := json.MarshalIndent(map[string]any{
		"live":        rep.Live,
		"max_context": rep.MaxContext,
		"arms":        rep.Arms,
	}, "", "  ")
	fmt.Println(string(enc))
	fmt.Println("swe-distill report →", mdPath)
}
