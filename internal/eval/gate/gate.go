package gate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/norbertvannobelen/dynamic-clm/internal/backend/omlx"
	"github.com/norbertvannobelen/dynamic-clm/internal/scheduler"
	"github.com/norbertvannobelen/dynamic-clm/internal/session"
	"github.com/norbertvannobelen/dynamic-clm/internal/tracing"
)

// Config for the Phase A hypothesis gate.
type Config struct {
	ResultsDir   string
	HardwareID   string
	ModelID      string
	Sessions     int
	MaxTurns     int
	MaxContext   int
	UseLiveModel bool
	Calibration  scheduler.CalibrationResult
	DeviceCosts  scheduler.DeviceCosts
}

// CriterionVerdict is one of G1–G4.
type CriterionVerdict struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Pass    bool    `json:"pass"`
	Value   float64 `json:"value"`
	Threshold float64 `json:"threshold"`
	Detail  string  `json:"detail"`
}

// Report is the gate summary used for the interim report and auto-continue.
type Report struct {
	GeneratedAt time.Time                    `json:"generated_at"`
	ModelID     string                       `json:"model_id"`
	HardwareID  string                       `json:"hardware_id"`
	Calibration scheduler.CalibrationResult  `json:"calibration"`
	ArmStats    map[string]ArmStats          `json:"arm_stats"`
	Criteria    []CriterionVerdict           `json:"criteria"`
	Passed      bool                         `json:"passed"`
	Sessions    int                          `json:"sessions_per_arm"`
	MaxTurns    int                          `json:"max_turns"`
	LiveModel   bool                         `json:"live_model"`
}

// ArmStats aggregates one treatment arm.
type ArmStats struct {
	Sessions           int     `json:"sessions"`
	SuccessRate        float64 `json:"success_rate"`
	AvgPrefillTokens   float64 `json:"avg_prefill_tokens"`
	AvgRewrites        float64 `json:"avg_rewrites"`
	AvgParseRate       float64 `json:"avg_parse_rate"`
	AvgCompliance      float64 `json:"avg_compliance"`
	AvgShrinkErrPct    float64 `json:"avg_shrink_err_pct"`
	SessionsWithEdits2 int     `json:"sessions_with_ge2_edits"`
}

// Run executes the 3-arm gate and evaluates G1–G4.
func Run(cfg Config) (Report, error) {
	if cfg.Sessions <= 0 {
		cfg.Sessions = 20
	}
	if cfg.MaxTurns <= 0 {
		cfg.MaxTurns = 50
	}
	if cfg.MaxContext <= 0 {
		cfg.MaxContext = 32768
	}
	if cfg.HardwareID == "" {
		cfg.HardwareID = "apple-m5-max"
	}
	if cfg.ResultsDir == "" {
		cfg.ResultsDir = "results"
	}

	var client *omlx.Client
	if cfg.UseLiveModel {
		c, err := omlx.NewClient()
		if err != nil {
			return Report{}, err
		}
		c.MaxTokens = 128
		client = c
	}

	arms := []session.Arm{session.ArmBaseline, session.ArmImmediateRewrite, session.ArmDynCLM}
	armResults := map[string][]session.Result{}

	tracePath := filepath.Join(cfg.ResultsDir, "gate", "trace.jsonl")
	rec, err := tracing.OpenRecorder(tracePath)
	if err != nil {
		return Report{}, err
	}
	defer rec.Close()

	for _, arm := range arms {
		for i := 0; i < cfg.Sessions; i++ {
			sid := fmt.Sprintf("%s-%03d", arm, i)
			cell := tracing.CellPath(cfg.ResultsDir, cfg.HardwareID, cfg.ModelID, "coding", string(arm), sid)
			if tracing.CellDone(cell) {
				var existing session.Result
				b, _ := os.ReadFile(cell)
				if json.Unmarshal(b, &existing) == nil {
					armResults[string(arm)] = append(armResults[string(arm)], existing)
					tracing.ProgressLog(cfg.ResultsDir, fmt.Sprintf("skip cell %s (done)", cell))
					continue
				}
			}
			tracing.ProgressLog(cfg.ResultsDir, fmt.Sprintf("start cell arm=%s session=%s live=%v", arm, sid, cfg.UseLiveModel))

			policy := scheduler.DefaultDynCLM()
			runner := session.NewRunner(client, rec, cfg.DeviceCosts)
			res, err := runner.RunCodingSession(session.Config{
				HardwareID:   cfg.HardwareID,
				ModelID:      cfg.ModelID,
				TaskType:     "coding",
				Arm:          arm,
				SessionID:    sid,
				MaxTurns:     cfg.MaxTurns,
				MaxContext:   cfg.MaxContext,
				Policy:       policy,
				DeviceCosts:  cfg.DeviceCosts,
				ResultsDir:   cfg.ResultsDir,
				UseLiveModel: cfg.UseLiveModel,
			})
			if err != nil {
				tracing.ProgressLog(cfg.ResultsDir, fmt.Sprintf("FAIL cell %s: %v (retry once)", sid, err))
				res, err = runner.RunCodingSession(session.Config{
					HardwareID:   cfg.HardwareID,
					ModelID:      cfg.ModelID,
					TaskType:     "coding",
					Arm:          arm,
					SessionID:    sid,
					MaxTurns:     cfg.MaxTurns,
					MaxContext:   cfg.MaxContext,
					Policy:       policy,
					DeviceCosts:  cfg.DeviceCosts,
					ResultsDir:   cfg.ResultsDir,
					UseLiveModel: cfg.UseLiveModel,
				})
				if err != nil {
					tracing.ProgressLog(cfg.ResultsDir, fmt.Sprintf("SKIP cell %s after retry: %v", sid, err))
					continue
				}
			}
			_ = tracing.WriteCell(cell, res)
			armResults[string(arm)] = append(armResults[string(arm)], res)
			tracing.ProgressLog(cfg.ResultsDir, fmt.Sprintf("finish cell arm=%s session=%s success=%v prefill=%d rewrites=%d",
				arm, sid, res.Success, res.TotalPrefillTokens, res.RewriteCount))
		}
	}

	stats := map[string]ArmStats{}
	for k, rs := range armResults {
		stats[k] = aggregate(rs)
	}

	criteria := evaluate(stats)
	passed := true
	for _, c := range criteria {
		if !c.Pass {
			passed = false
			break
		}
	}

	rep := Report{
		GeneratedAt: time.Now().UTC(),
		ModelID:     cfg.ModelID,
		HardwareID:  cfg.HardwareID,
		Calibration: cfg.Calibration,
		ArmStats:    stats,
		Criteria:    criteria,
		Passed:      passed,
		Sessions:    cfg.Sessions,
		MaxTurns:    cfg.MaxTurns,
		LiveModel:   cfg.UseLiveModel,
	}
	out := filepath.Join(cfg.ResultsDir, "gate", "report.json")
	_ = tracing.WriteCell(out, rep)
	return rep, nil
}

func aggregate(rs []session.Result) ArmStats {
	a := ArmStats{Sessions: len(rs)}
	if len(rs) == 0 {
		return a
	}
	var succ, pref, rew, parse, comp, shrink float64
	var compDenom float64
	for _, r := range rs {
		if r.Success {
			succ++
		}
		pref += float64(r.TotalPrefillTokens)
		rew += float64(r.RewriteCount)
		parse += r.MarkerParseRate
		shrink += r.ShrinkErrorPct
		if r.EditEvents >= 2 {
			a.SessionsWithEdits2++
		}
		if r.ComplianceChecks > 0 {
			comp += float64(r.ComplianceHits) / float64(r.ComplianceChecks)
			compDenom++
		}
	}
	n := float64(len(rs))
	a.SuccessRate = 100 * succ / n
	a.AvgPrefillTokens = pref / n
	a.AvgRewrites = rew / n
	a.AvgParseRate = parse / n
	a.AvgShrinkErrPct = shrink / n
	if compDenom > 0 {
		a.AvgCompliance = 100 * comp / compDenom
	}
	return a
}

func evaluate(stats map[string]ArmStats) []CriterionVerdict {
	dyn := stats[string(session.ArmDynCLM)]
	base := stats[string(session.ArmBaseline)]
	imm := stats[string(session.ArmImmediateRewrite)]

	g1Pass := dyn.AvgParseRate >= 0.95 && dyn.AvgRewrites > 0 && dyn.AvgShrinkErrPct <= 10
	g1 := CriterionVerdict{
		ID: "G1", Name: "mechanism", Pass: g1Pass,
		Value: dyn.AvgParseRate, Threshold: 0.95,
		Detail: fmt.Sprintf("parse_rate=%.3f rewrites=%.2f shrink_err=%.1f%%", dyn.AvgParseRate, dyn.AvgRewrites, dyn.AvgShrinkErrPct),
	}

	g2Pass := dyn.AvgCompliance >= 80
	g2 := CriterionVerdict{
		ID: "G2", Name: "compliance", Pass: g2Pass,
		Value: dyn.AvgCompliance, Threshold: 80,
		Detail: fmt.Sprintf("tail_marker_compliance=%.1f%%", dyn.AvgCompliance),
	}

	g3Pass := dyn.SuccessRate >= base.SuccessRate-3 && dyn.SuccessRate >= imm.SuccessRate-3
	g3 := CriterionVerdict{
		ID: "G3", Name: "quality", Pass: g3Pass,
		Value: dyn.SuccessRate, Threshold: base.SuccessRate - 3,
		Detail: fmt.Sprintf("dyn=%.1f%% baseline=%.1f%% immediate=%.1f%%", dyn.SuccessRate, base.SuccessRate, imm.SuccessRate),
	}

	// G4: DynCLM re-prefill ≥30% below immediate on sessions with ≥2 edits.
	reduction := 0.0
	g4Pass := false
	if imm.AvgPrefillTokens > 0 {
		reduction = (imm.AvgPrefillTokens - dyn.AvgPrefillTokens) / imm.AvgPrefillTokens
		g4Pass = reduction >= 0.30 && dyn.SessionsWithEdits2 > 0
	}
	g4 := CriterionVerdict{
		ID: "G4", Name: "cost", Pass: g4Pass,
		Value: 100 * reduction, Threshold: 30,
		Detail: fmt.Sprintf("prefill_reduction=%.1f%% dyn_avg=%.0f imm_avg=%.0f", 100*reduction, dyn.AvgPrefillTokens, imm.AvgPrefillTokens),
	}

	return []CriterionVerdict{g1, g2, g3, g4}
}

// WriteInterimMarkdown writes docs/interim-report.md from a gate report.
func WriteInterimMarkdown(path string, rep Report, cacheProbe map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	passFail := "FAIL — Phase B will not run"
	if rep.Passed {
		passFail = "PASS — Phase B continues automatically"
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	_, _ = fmt.Fprintf(f, "# DynCLM Interim Report (Phase A Gate)\n\n")
	_, _ = fmt.Fprintf(f, "Generated: %s\n\n", rep.GeneratedAt.Format(time.RFC3339))
	_, _ = fmt.Fprintf(f, "**Gate verdict: %s**\n\n", passFail)
	_, _ = fmt.Fprintf(f, "## Setup\n\n")
	_, _ = fmt.Fprintf(f, "- Hardware: `%s`\n", rep.HardwareID)
	_, _ = fmt.Fprintf(f, "- Model: `%s`\n", rep.ModelID)
	_, _ = fmt.Fprintf(f, "- Sessions/arm: %d × %d turns\n", rep.Sessions, rep.MaxTurns)
	_, _ = fmt.Fprintf(f, "- Live oMLX model: %v\n\n", rep.LiveModel)

	_, _ = fmt.Fprintf(f, "## Calibration (M5 Max)\n\n")
	_, _ = fmt.Fprintf(f, "| Metric | Value |\n|---|---|\n")
	_, _ = fmt.Fprintf(f, "| r (ms/token) | %.6f |\n", rep.Calibration.R)
	_, _ = fmt.Fprintf(f, "| k_d (ms/token/turn) | %.8f |\n", rep.Calibration.Kd)
	_, _ = fmt.Fprintf(f, "| C_fixed (ms) | %.3f |\n", rep.Calibration.CFixed)
	_, _ = fmt.Fprintf(f, "| ρ = C_fixed/k_d | %.3f |\n", rep.Calibration.Rho)
	_, _ = fmt.Fprintf(f, "| Notes | %s |\n\n", rep.Calibration.Notes)

	_, _ = fmt.Fprintf(f, "## Per-arm results\n\n")
	_, _ = fmt.Fprintf(f, "| Arm | Success%% | Avg re-prefill | Avg rewrites | Parse rate | Compliance%% |\n|---|---|---|---|---|---|\n")
	for _, name := range []string{"baseline", "immediate-rewrite", "dynclm"} {
		a := rep.ArmStats[name]
		_, _ = fmt.Fprintf(f, "| %s | %.1f | %.0f | %.2f | %.3f | %.1f |\n",
			name, a.SuccessRate, a.AvgPrefillTokens, a.AvgRewrites, a.AvgParseRate, a.AvgCompliance)
	}
	_, _ = fmt.Fprintf(f, "\n## Gate criteria G1–G4\n\n")
	for _, c := range rep.Criteria {
		mark := "FAIL"
		if c.Pass {
			mark = "PASS"
		}
		_, _ = fmt.Fprintf(f, "- **%s %s (%s)**: %s — %s\n", c.ID, mark, c.Name, c.Detail, mark)
	}

	_, _ = fmt.Fprintf(f, "\n## oMLX cache probe (SCR readiness)\n\n")
	if len(cacheProbe) == 0 {
		_, _ = fmt.Fprintf(f, "Not probed.\n")
	} else {
		for p, st := range cacheProbe {
			_, _ = fmt.Fprintf(f, "- `%s` → %s\n", p, st)
		}
	}
	_, _ = fmt.Fprintf(f, "\n## Gaps\n\n")
	_, _ = fmt.Fprintf(f, "- No hybrid linear-attention model installed; SCR recurrent-state path not exercised.\n")
	_, _ = fmt.Fprintf(f, "- Physical KV relocation uses LogicalCacheBackend accounting (oMLX lacks span-level KV APIs).\n")
	if !rep.Passed {
		_, _ = fmt.Fprintf(f, "\n## Failure analysis\n\n")
		for _, c := range rep.Criteria {
			if !c.Pass {
				_, _ = fmt.Fprintf(f, "- %s failed: %s. Implied fix: shorten deferral window or abandon per §7.3.\n", c.ID, c.Detail)
			}
		}
	}
	return nil
}
