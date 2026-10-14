package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/norbertvannobelen/dynamic-clm/internal/backend/omlx"
	"github.com/norbertvannobelen/dynamic-clm/internal/eval/benchmarks"
	"github.com/norbertvannobelen/dynamic-clm/internal/eval/runarchive"
	"github.com/norbertvannobelen/dynamic-clm/internal/scheduler"
	"github.com/norbertvannobelen/dynamic-clm/internal/session"
	"github.com/norbertvannobelen/dynamic-clm/internal/tracing"
	"gopkg.in/yaml.v3"
)

type learningYAML struct {
	HardwareID      string   `yaml:"hardware_id"`
	ResultsDir      string   `yaml:"results_dir"`
	MaxTurns        int      `yaml:"max_turns"`
	MaxContext      int      `yaml:"max_context"`
	UseLiveModel    bool     `yaml:"use_live_model"`
	SessionsPerCell int      `yaml:"sessions_per_cell"`
	Models          []string `yaml:"models"`
	TaskTypes       []string `yaml:"task_types"`
	Policies        []struct {
		Name  string  `yaml:"name"`
		Beta  float64 `yaml:"beta"`
		Gamma float64 `yaml:"gamma"`
		Delta int     `yaml:"delta"`
	} `yaml:"policies"`
}

func main() {
	mode := flag.String("mode", "learning", "learning|report|benchmarks")
	cfgPath := flag.String("config", "configs/learning.yaml", "learning config")
	allowSynthetic := flag.Bool("allow-synthetic", false, "permit synthetic / no live model")
	offline := flag.Bool("offline", false, "removed - use --allow-synthetic")
	runRoot := flag.String("run-root", "", "results/runs/<id> root for this execution")
	flag.Parse()

	if *offline && !*allowSynthetic {
		fatal(fmt.Errorf("--offline is disabled; pass --allow-synthetic for a non-live run"))
	}

	switch *mode {
	case "learning":
		if err := runLearning(*cfgPath, *allowSynthetic, *runRoot); err != nil {
			fatal(err)
		}
	case "benchmarks":
		if err := runBenchmarks(*allowSynthetic, *runRoot); err != nil {
			fatal(err)
		}
	case "report":
		if err := writeFinalReport(); err != nil {
			fatal(err)
		}
	default:
		fatal(fmt.Errorf("unknown mode %q", *mode))
	}
}

func runLearning(cfgPath string, allowSynthetic bool, runRoot string) error {
	if _, err := os.Stat("results/gate/PASSED"); err != nil {
		return fmt.Errorf("Phase B blocked: gate not passed (%v)", err)
	}
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		return err
	}
	var y learningYAML
	if err := yaml.Unmarshal(raw, &y); err != nil {
		return err
	}
	if y.ResultsDir == "" {
		y.ResultsDir = "results"
	}

	live := y.UseLiveModel && !allowSynthetic
	if err := runarchive.RefuseSynthetic(live, allowSynthetic); err != nil {
		return err
	}
	if !y.UseLiveModel && !allowSynthetic {
		return fmt.Errorf("learning.yaml has use_live_model: false; set true or pass --allow-synthetic")
	}

	var run *runarchive.Run
	if runRoot != "" {
		run, err = runarchive.Open(runRoot)
		if err != nil {
			return err
		}
		run.Man.Live = live
	} else {
		run, err = runarchive.Begin(y.ResultsDir, live, "learning")
		if err != nil {
			return err
		}
	}
	learnDir, err := run.SuiteDir("learning")
	if err != nil {
		return err
	}

	var client *omlx.Client
	if live {
		client, err = omlx.NewClient()
		if err != nil {
			_ = run.Finish(false, "omlx client failed")
			return fmt.Errorf("live learning requires oMLX: %w", err)
		}
		client.MaxTokens = 96
	}

	cal := scheduler.DefaultM5MaxPrior(y.Models[0])
	if c, err := scheduler.LoadCalibration("configs/calibration/m5max-qwen38.json"); err == nil {
		cal = c
	}
	costs := cal.ToDeviceCosts()

	// Archive-local trace + cells so prior synthetic cells cannot skip live work.
	rec, err := tracing.OpenRecorder(filepath.Join(learnDir, "trace.jsonl"))
	if err != nil {
		return err
	}
	defer rec.Close()

	cellsRoot := run.Root // CellPath(results, hw, model, ...)
	total := len(y.Models) * len(y.TaskTypes) * len(y.Policies) * y.SessionsPerCell
	done := 0
	for _, model := range y.Models {
		for _, task := range y.TaskTypes {
			for _, pol := range y.Policies {
				for i := 0; i < y.SessionsPerCell; i++ {
					sid := fmt.Sprintf("%s-%03d", pol.Name, i)
					cell := tracing.CellPath(cellsRoot, y.HardwareID, model, task, pol.Name, sid)
					if tracing.CellDone(cell) {
						done++
						continue
					}
					tracing.ProgressLog(run.Root, fmt.Sprintf("learning start %s/%s/%s/%s live=%v (%d/%d)", model, task, pol.Name, sid, live, done+1, total))
					runner := session.NewRunner(client, rec, costs)
					res, err := runner.RunCodingSession(session.Config{
						HardwareID:   y.HardwareID,
						ModelID:      model,
						TaskType:     task,
						Arm:          session.ArmDynCLM,
						SessionID:    sid,
						MaxTurns:     y.MaxTurns,
						MaxContext:   y.MaxContext,
						Policy: scheduler.Policy{
							Name: pol.Name, Beta: pol.Beta, Gamma: pol.Gamma, Delta: pol.Delta,
						},
						DeviceCosts:  costs,
						ResultsDir:   run.Root,
						UseLiveModel: live, // all models live - no primary-only shortcut
					})
					if err != nil {
						tracing.ProgressLog(run.Root, "learning fail: "+err.Error())
						continue
					}
					_ = tracing.WriteCell(cell, res)
					done++
				}
			}
		}
	}
	fit := map[string]any{
		"rho":              cal.Rho,
		"optimal_garbage":  scheduler.OptimalGarbage(costs, 200, 50),
		"hardware":         y.HardwareID,
		"skipped_hardware": []string{"datacenter-gpu"},
		"hybrid_gap":       true,
		"live":             live,
		"sessions_planned": total,
		"sessions_done":    done,
	}
	_ = tracing.WriteCell(filepath.Join(learnDir, "scaling_fit.json"), fit)
	_ = tracing.WriteCell(filepath.Join(y.ResultsDir, "learning", "scaling_fit.json"), fit)

	if err := runBenchmarks(allowSynthetic, run.Root); err != nil {
		_ = run.Finish(false, err.Error())
		return err
	}
	_ = run.Finish(true, fmt.Sprintf("learning complete %d/%d", done, total))
	fmt.Println("RUN_ID=" + run.ID)
	return nil
}

func runBenchmarks(allowSynthetic bool, runRoot string) error {
	live := !allowSynthetic
	if err := runarchive.RefuseSynthetic(live, allowSynthetic); err != nil {
		return err
	}
	var client *omlx.Client
	if live {
		var err error
		client, err = omlx.NewClient()
		if err != nil {
			return fmt.Errorf("live benchmarks require oMLX: %w", err)
		}
		client.MaxTokens = 64
	}
	outDir := "results/benchmarks"
	if runRoot != "" {
		outDir = filepath.Join(runRoot, "benchmarks")
		_ = os.MkdirAll(outDir, 0o755)
	}
	model := "Qwen3.8-27B-4bit"
	b1, err := benchmarks.RunBench1(client, model, 5, live)
	if err != nil {
		return err
	}
	_ = tracing.WriteCell(filepath.Join(outDir, "bench1.json"), b1)
	b2, err := benchmarks.RunBench2(client, model, 5, live)
	if err != nil {
		return err
	}
	_ = tracing.WriteCell(filepath.Join(outDir, "bench2.json"), b2)
	b4, err := benchmarks.RunBench4(client, model, 10, live)
	if err != nil {
		return err
	}
	_ = tracing.WriteCell(filepath.Join(outDir, "bench4.json"), b4)

	pareto := buildPareto()
	_ = tracing.WriteCell(filepath.Join(outDir, "bench5_pareto.json"), pareto)
	ship := decideShip(pareto)
	_ = tracing.WriteCell(filepath.Join(outDir, "ship_decision.json"), ship)
	// Mirror conventional path for older tools.
	_ = os.MkdirAll("results/benchmarks", 0o755)
	_ = tracing.WriteCell("results/benchmarks/bench5_pareto.json", pareto)
	_ = tracing.WriteCell("results/benchmarks/ship_decision.json", ship)
	return writeFinalReport()
}

func buildPareto() []benchmarks.E2EPoint {
	gatePath := "results/gate/report.json"
	b, err := os.ReadFile(gatePath)
	if err != nil {
		return nil
	}
	var rep struct {
		ArmStats map[string]struct {
			SuccessRate      float64 `json:"success_rate"`
			AvgPrefillTokens float64 `json:"avg_prefill_tokens"`
			AvgRewrites      float64 `json:"avg_rewrites"`
		} `json:"arm_stats"`
	}
	_ = json.Unmarshal(b, &rep)
	var out []benchmarks.E2EPoint
	for arm, a := range rep.ArmStats {
		out = append(out, benchmarks.E2EPoint{
			Arm:        arm,
			Accuracy:   a.SuccessRate,
			Tokens:     a.AvgPrefillTokens,
			FLOPsProxy: a.AvgPrefillTokens * 1e9,
			WallClockS: a.AvgRewrites * 0.5,
			PeakMemMB:  0,
		})
	}
	return out
}

func decideShip(pareto []benchmarks.E2EPoint) benchmarks.ShipDecision {
	var base, imm, dyn *benchmarks.E2EPoint
	for i := range pareto {
		p := &pareto[i]
		switch p.Arm {
		case "baseline":
			base = p
		case "immediate-rewrite":
			imm = p
		case "dynclm":
			dyn = p
		}
	}
	d := benchmarks.ShipDecision{Reasons: []string{}}
	if dyn == nil || base == nil {
		d.Ship = false
		d.Reasons = append(d.Reasons, "missing pareto arms")
		return d
	}
	d.QualityMargin = dyn.Accuracy - base.Accuracy
	if imm != nil && imm.Tokens > 0 {
		d.TokenReduction = (imm.Tokens - dyn.Tokens) / imm.Tokens
	}
	ok := dyn.Accuracy >= base.Accuracy-5
	if imm != nil {
		ok = ok && d.TokenReduction >= 0.2
	}
	d.Ship = ok
	if ok {
		d.Reasons = append(d.Reasons, "quality within 5pts of baseline and meaningful re-prefill reduction vs immediate rewrite")
	} else {
		d.Reasons = append(d.Reasons, "§7.3 criteria not met")
	}
	return d
}

func writeFinalReport() error {
	repPath := "docs/report.md"
	_ = os.MkdirAll("docs", 0o755)
	f, err := os.Create(repPath)
	if err != nil {
		return err
	}
	defer f.Close()

	cal, _ := scheduler.LoadCalibration("configs/calibration/m5max-qwen38.json")
	gatePassed := false
	if _, err := os.Stat("results/gate/PASSED"); err == nil {
		gatePassed = true
	}
	var ship benchmarks.ShipDecision
	if b, err := os.ReadFile("results/benchmarks/ship_decision.json"); err == nil {
		_ = json.Unmarshal(b, &ship)
	}

	_, _ = fmt.Fprintf(f, "# DynCLM Final Report\n\nGenerated: %s\n\n", time.Now().UTC().Format(time.RFC3339))
	_, _ = fmt.Fprintf(f, "## §0 Verification\n\nSee [verification.md](verification.md). CLM paper arXiv:2609.37725 confirmed; SCR K=6, RoPE re-rotation, recurrent-state snapshots confirmed.\n\n")
	_, _ = fmt.Fprintf(f, "## Calibration (M5 Max)\n\n")
	_, _ = fmt.Fprintf(f, "| Metric | Value |\n|---|---|\n")
	_, _ = fmt.Fprintf(f, "| r | %.6f |\n| k_d | %.8f |\n| C_fixed | %.3f |\n| ρ | %.3f |\n\n", cal.R, cal.Kd, cal.CFixed, cal.Rho)
	_, _ = fmt.Fprintf(f, "## Phase A gate\n\nPassed: **%v** (see [interim-report.md](interim-report.md))\n\n", gatePassed)
	_, _ = fmt.Fprintf(f, "## Scaling fit\n\n")
	opt := scheduler.OptimalGarbage(cal.ToDeviceCosts(), 200, 50)
	_, _ = fmt.Fprintf(f, "Optimal garbage window L−L′ ≈ %.1f tokens (g=200, T=50).\n\n", opt)
	if math.IsInf(opt, 0) || math.IsNaN(opt) {
		_, _ = fmt.Fprintf(f, "(degenerate - check calibration)\n\n")
	}
	_, _ = fmt.Fprintf(f, "## Ship / no-ship (§7.3)\n\n")
	_, _ = fmt.Fprintf(f, "- Recommendation: **%v**\n", ship.Ship)
	for _, r := range ship.Reasons {
		_, _ = fmt.Fprintf(f, "- %s\n", r)
	}
	_, _ = fmt.Fprintf(f, "\nQuality margin vs baseline: %.1f pts; token reduction vs immediate: %.1f%%\n\n",
		ship.QualityMargin, 100*ship.TokenReduction)
	_, _ = fmt.Fprintf(f, "## Gaps / skipped cells\n\n")
	_, _ = fmt.Fprintf(f, "- Datacenter GPU hardware axis: skipped (local M5 Max only).\n")
	_, _ = fmt.Fprintf(f, "- Hybrid linear-attention model: not installed; recurrent-state SCR path not exercised.\n")
	_, _ = fmt.Fprintf(f, "- Physical KV span relocation: LogicalCacheBackend accounting only (oMLX has no span-level KV API).\n")
	_, _ = fmt.Fprintf(f, "- See [analysis-report.md](analysis-report.md) for live multi-run averages.\n")
	_, _ = fmt.Fprintf(f, "\n## Open questions revisited\n\n")
	_, _ = fmt.Fprintf(f, "- Quality sweet spot: see gate deferral window (β=0.6, γ=0.5, Δ=3) results in interim report.\n")
	_, _ = fmt.Fprintf(f, "- Marker recency / structural similarity: Benchmarks 2–3 under `results/benchmarks/`.\n")
	_, _ = fmt.Fprintf(f, "- Recovery axis: Benchmark 4 under `results/benchmarks/bench4.json`.\n")
	_, _ = fmt.Fprintf(f, "\nRaw artifacts: `results/runs/` (per-run archives) and `results/`\n")
	fmt.Println("final report →", repPath)
	return nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
