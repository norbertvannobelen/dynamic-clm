package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/norbertvannobelen/dynamic-clm/internal/backend/omlx"
	"github.com/norbertvannobelen/dynamic-clm/internal/eval/benchmarks"
	"github.com/norbertvannobelen/dynamic-clm/internal/eval/gate"
	"github.com/norbertvannobelen/dynamic-clm/internal/eval/runarchive"
	"github.com/norbertvannobelen/dynamic-clm/internal/scheduler"
	"github.com/norbertvannobelen/dynamic-clm/internal/tracing"
	"gopkg.in/yaml.v3"
)

type gateYAML struct {
	HardwareID      string `yaml:"hardware_id"`
	ModelID         string `yaml:"model_id"`
	Sessions        int    `yaml:"sessions"`
	MaxTurns        int    `yaml:"max_turns"`
	MaxContext      int    `yaml:"max_context"`
	UseLiveModel    bool   `yaml:"use_live_model"`
	ResultsDir      string `yaml:"results_dir"`
	CalibrationPath string `yaml:"calibration_path"`
}

func main() {
	cfgPath := flag.String("config", "configs/gate.yaml", "config path")
	allowSynthetic := flag.Bool("allow-synthetic", false, "permit synthetic gate (no oMLX)")
	offline := flag.Bool("offline", false, "removed — use --allow-synthetic")
	sessions := flag.Int("sessions", 0, "override sessions per arm")
	turns := flag.Int("turns", 0, "override max turns")
	runRoot := flag.String("run-root", "", "optional results/runs/<id> root (created if empty)")
	flag.Parse()

	if *offline && !*allowSynthetic {
		fatal(fmt.Errorf("--offline is disabled; pass --allow-synthetic for a non-live run"))
	}

	raw, err := os.ReadFile(*cfgPath)
	if err != nil {
		fatal(err)
	}
	var y gateYAML
	if err := yaml.Unmarshal(raw, &y); err != nil {
		fatal(err)
	}
	if *sessions > 0 {
		y.Sessions = *sessions
	}
	if *turns > 0 {
		y.MaxTurns = *turns
	}
	if y.ResultsDir == "" {
		y.ResultsDir = "results"
	}

	live := y.UseLiveModel && !*allowSynthetic
	if err := runarchive.RefuseSynthetic(live, *allowSynthetic); err != nil {
		fatal(err)
	}
	if !y.UseLiveModel && !*allowSynthetic {
		fatal(fmt.Errorf("configs/gate.yaml has use_live_model: false; set true or pass --allow-synthetic"))
	}

	var run *runarchive.Run
	if *runRoot != "" {
		_ = os.MkdirAll(*runRoot, 0o755)
		run, err = runarchive.Open(*runRoot)
		if err != nil {
			fatal(err)
		}
		run.Man.Live = live
	} else {
		run, err = runarchive.Begin(y.ResultsDir, live, "gate")
		if err != nil {
			fatal(err)
		}
	}
	gateDir, err := run.SuiteDir("gate")
	if err != nil {
		fatal(err)
	}
	// SuiteDir is .../gate; gate.Run writes into ResultsDir/gate/ — set parent.
	resultsForGate := run.Root

	cal := scheduler.DefaultM5MaxPrior(y.ModelID)
	if c, err := scheduler.LoadCalibration(y.CalibrationPath); err == nil {
		cal = c
	} else {
		tracing.ProgressLog(resultsForGate, "calibration missing; using prior — run dynclm-calibrate")
	}

	var cacheProbe map[string]string
	if live {
		client, err := omlx.NewClient()
		if err != nil {
			_ = run.Finish(false, "omlx client failed")
			fatal(fmt.Errorf("live gate requires oMLX: %w", err))
		}
		cacheProbe = client.ProbeCacheAPIs()
	}

	tracing.ProgressLog(resultsForGate, fmt.Sprintf("gate start live=%v run=%s", live, run.ID))
	rep, err := gate.Run(gate.Config{
		ResultsDir:   resultsForGate,
		HardwareID:   y.HardwareID,
		ModelID:      y.ModelID,
		Sessions:     y.Sessions,
		MaxTurns:     y.MaxTurns,
		MaxContext:   y.MaxContext,
		UseLiveModel: live,
		Calibration:  cal,
		DeviceCosts:  cal.ToDeviceCosts(),
	})
	if err != nil {
		_ = run.Finish(false, err.Error())
		fatal(err)
	}

	var client *omlx.Client
	if live {
		client, err = omlx.NewClient()
		if err != nil {
			_ = run.Finish(false, "omlx client failed for bench3")
			fatal(err)
		}
		client.MaxTokens = 64
	}
	bench3, err := benchmarks.RunBench3(client, y.ModelID, 5, live)
	if err != nil {
		tracing.ProgressLog(resultsForGate, "bench3 error: "+err.Error())
	} else {
		_ = tracing.WriteCell(filepath.Join(gateDir, "bench3.json"), bench3)
	}

	interim := filepath.Join("docs", "interim-report.md")
	if err := gate.WriteInterimMarkdown(interim, rep, cacheProbe); err != nil {
		fatal(err)
	}
	// Also keep a copy inside the run archive.
	_ = gate.WriteInterimMarkdown(filepath.Join(run.Root, "interim-report.md"), rep, cacheProbe)

	// Mirror PASSED into conventional results/gate for Phase B gate check,
	// and into the run archive.
	if rep.Passed {
		_ = os.MkdirAll("results/gate", 0o755)
		_ = os.WriteFile("results/gate/PASSED", []byte("ok\n"), 0o644)
		_ = os.WriteFile(filepath.Join(gateDir, "PASSED"), []byte("ok\n"), 0o644)
		// Copy latest gate report to conventional path for analysis tools.
		if b, err := os.ReadFile(filepath.Join(resultsForGate, "gate", "report.json")); err == nil {
			_ = os.WriteFile("results/gate/report.json", b, 0o644)
		}
	}

	b, _ := json.MarshalIndent(rep, "", "  ")
	fmt.Println(string(b))
	fmt.Println("interim report →", interim)
	fmt.Println("run archive →", run.Root)

	if !rep.Passed {
		_ = run.Finish(false, "GATE FAIL")
		tracing.ProgressLog(resultsForGate, "GATE FAIL — stopping before Phase B")
		os.Exit(2)
	}
	_ = run.Finish(true, "GATE PASS")
	tracing.ProgressLog(resultsForGate, "GATE PASS — Phase B may continue")
	fmt.Println("RUN_ID=" + run.ID)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
