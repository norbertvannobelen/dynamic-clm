package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/norbertvannobelen/dynamic-clm/internal/backend/omlx"
	"github.com/norbertvannobelen/dynamic-clm/internal/eval/benchmarks"
	"github.com/norbertvannobelen/dynamic-clm/internal/eval/gate"
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
	offline := flag.Bool("offline", false, "synthetic gate (no oMLX)")
	sessions := flag.Int("sessions", 0, "override sessions per arm")
	turns := flag.Int("turns", 0, "override max turns")
	flag.Parse()

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
	live := y.UseLiveModel && !*offline

	cal := scheduler.DefaultM5MaxPrior(y.ModelID)
	if c, err := scheduler.LoadCalibration(y.CalibrationPath); err == nil {
		cal = c
	} else {
		tracing.ProgressLog(y.ResultsDir, "calibration missing; using prior — run dynclm-calibrate")
	}

	var cacheProbe map[string]string
	if client, err := omlx.NewClient(); err == nil {
		cacheProbe = client.ProbeCacheAPIs()
	}

	tracing.ProgressLog(y.ResultsDir, "gate start")
	rep, err := gate.Run(gate.Config{
		ResultsDir:   y.ResultsDir,
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
		fatal(err)
	}

	// Benchmark 3 (instruction decay) — cheap add-on for interim report.
	var client *omlx.Client
	if live {
		client, _ = omlx.NewClient()
		if client != nil {
			client.MaxTokens = 64
		}
	}
	bench3, err := benchmarks.RunBench3(client, y.ModelID, 5, live)
	if err != nil {
		tracing.ProgressLog(y.ResultsDir, "bench3 error: "+err.Error())
	} else {
		_ = tracing.WriteCell(y.ResultsDir+"/gate/bench3.json", bench3)
	}

	if err := gate.WriteInterimMarkdown("docs/interim-report.md", rep, cacheProbe); err != nil {
		fatal(err)
	}
	b, _ := json.MarshalIndent(rep, "", "  ")
	fmt.Println(string(b))
	fmt.Println("interim report → docs/interim-report.md")

	if !rep.Passed {
		tracing.ProgressLog(y.ResultsDir, "GATE FAIL — stopping before Phase B")
		os.Exit(2)
	}
	tracing.ProgressLog(y.ResultsDir, "GATE PASS — Phase B may continue")
	// Signal file for eval runner.
	_ = os.WriteFile(y.ResultsDir+"/gate/PASSED", []byte("ok\n"), 0o644)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
