package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/norbertvannobelen/dynamic-clm/internal/backend/omlx"
	"github.com/norbertvannobelen/dynamic-clm/internal/scheduler"
	"github.com/norbertvannobelen/dynamic-clm/internal/session"
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
	cfgPath := flag.String("config", "configs/gate.yaml", "gate config")
	sessions := flag.Int("sessions", 0, "override sessions")
	offline := flag.Bool("offline", false, "synthetic baseline (no oMLX)")
	flag.Parse()

	raw, err := os.ReadFile(*cfgPath)
	if err != nil {
		fatal(err)
	}
	var cfg gateYAML
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		fatal(err)
	}
	if *sessions > 0 {
		cfg.Sessions = *sessions
	}
	if cfg.Sessions == 0 {
		cfg.Sessions = 20
	}
	live := cfg.UseLiveModel && !*offline

	costs := scheduler.DefaultM5MaxPrior(cfg.ModelID).ToDeviceCosts()
	if cal, err := scheduler.LoadCalibration(cfg.CalibrationPath); err == nil {
		costs = cal.ToDeviceCosts()
	}

	var client *omlx.Client
	if live {
		client, err = omlx.NewClient()
		if err != nil {
			fatal(err)
		}
		client.MaxTokens = 128
	}

	rec, err := tracing.OpenRecorder(cfg.ResultsDir + "/baseline/trace.jsonl")
	if err != nil {
		fatal(err)
	}
	defer rec.Close()

	var results []session.Result
	for i := 0; i < cfg.Sessions; i++ {
		sid := fmt.Sprintf("baseline-%03d", i)
		cell := tracing.CellPath(cfg.ResultsDir, cfg.HardwareID, cfg.ModelID, "coding", "baseline", sid)
		if tracing.CellDone(cell) {
			continue
		}
		runner := session.NewRunner(client, rec, costs)
		res, err := runner.RunCodingSession(session.Config{
			HardwareID:   cfg.HardwareID,
			ModelID:      cfg.ModelID,
			TaskType:     "coding",
			Arm:          session.ArmBaseline,
			SessionID:    sid,
			MaxTurns:     cfg.MaxTurns,
			MaxContext:   cfg.MaxContext,
			Policy:       scheduler.Baseline(),
			DeviceCosts:  costs,
			ResultsDir:   cfg.ResultsDir,
			UseLiveModel: live,
		})
		if err != nil {
			tracing.ProgressLog(cfg.ResultsDir, "baseline fail: "+err.Error())
			continue
		}
		_ = tracing.WriteCell(cell, res)
		results = append(results, res)
		fmt.Printf("baseline session %s success=%v\n", sid, res.Success)
	}
	b, _ := json.MarshalIndent(results, "", "  ")
	_ = os.WriteFile(cfg.ResultsDir+"/baseline/summary.json", b, 0o644)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
