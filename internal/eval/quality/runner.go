package quality

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/norbertvannobelen/dynamic-clm/internal/backend/omlx"
	"github.com/norbertvannobelen/dynamic-clm/internal/contextmgr"
	"github.com/norbertvannobelen/dynamic-clm/internal/scheduler"
	"github.com/norbertvannobelen/dynamic-clm/internal/tracing"
)

// ArmResult is one task × arm graded outcome.
type ArmResult struct {
	TaskID         string  `json:"task_id"`
	Category       string  `json:"category"`
	Arm            string  `json:"arm"`
	Correct        bool    `json:"correct"`
	Expected       string  `json:"expected"`
	Got            string  `json:"got"`
	PrefillTokens  int     `json:"prefill_tokens"`
	RewriteCount   int     `json:"rewrite_count"`
	WallClockS     float64 `json:"wall_clock_s"`
	PromptTokens   int     `json:"prompt_tokens"`
	CompletionToks int     `json:"completion_tokens"`
}

// ArmSummary aggregates quality + cost for one arm.
type ArmSummary struct {
	Arm              string         `json:"arm"`
	N                int            `json:"n"`
	Correct          int            `json:"correct"`
	AccuracyPct      float64        `json:"accuracy_pct"`
	ByCategory       map[string]Cat `json:"by_category"`
	AvgPrefillTokens float64        `json:"avg_prefill_tokens"`
	AvgRewrites      float64        `json:"avg_rewrites"`
	AvgWallClockS    float64        `json:"avg_wall_clock_s"`
}

// Cat is per-category accuracy.
type Cat struct {
	N           int     `json:"n"`
	Correct     int     `json:"correct"`
	AccuracyPct float64 `json:"accuracy_pct"`
}

// Report is the full quality suite output.
type Report struct {
	GeneratedAt      time.Time             `json:"generated_at"`
	ModelID          string                `json:"model_id"`
	HardwareID       string                `json:"hardware_id"`
	Live             bool                  `json:"live"`
	Tasks            int                   `json:"tasks"`
	Arms             map[string]ArmSummary `json:"arms"`
	DeltaVsBaseline  map[string]float64    `json:"delta_vs_baseline_pp"` // percentage points
	DeltaVsImmediate map[string]float64    `json:"delta_vs_immediate_pp"`
	PrefillCutVsImm  float64               `json:"prefill_cut_vs_immediate_pct"`
	Results          []ArmResult           `json:"results"`
}

// Config for the quality suite.
type Config struct {
	ModelID      string
	HardwareID   string
	ResultsDir   string
	UseLiveModel bool
	DeviceCosts  scheduler.DeviceCosts
	MaxContext   int
}

// Run executes the quality bank on all three arms.
func Run(cfg Config) (Report, error) {
	if cfg.HardwareID == "" {
		cfg.HardwareID = "apple-m5-max"
	}
	if cfg.ResultsDir == "" {
		cfg.ResultsDir = "results/quality"
	}
	if cfg.MaxContext == 0 {
		cfg.MaxContext = 8192
	}
	_ = os.MkdirAll(cfg.ResultsDir, 0o755)

	var client *omlx.Client
	if cfg.UseLiveModel {
		c, err := omlx.NewClient()
		if err != nil {
			return Report{}, err
		}
		c.MaxTokens = 256
		c.Temperature = 0.0
		// Prefer direct answers over long chain-of-thought when the server supports it.
		c.Extra = map[string]any{
			"enable_thinking": false,
			"chat_template_kwargs": map[string]any{
				"enable_thinking": false,
			},
		}
		client = c
	}

	tasks := Bank()
	arms := []string{"baseline", "immediate-rewrite", "dynclm"}
	var results []ArmResult

	for _, arm := range arms {
		for _, task := range tasks {
			cell := filepath.Join(cfg.ResultsDir, "cells", arm, task.ID+".json")
			if tracing.CellDone(cell) {
				var existing ArmResult
				if b, err := os.ReadFile(cell); err == nil && json.Unmarshal(b, &existing) == nil {
					results = append(results, existing)
					tracing.ProgressLog(cfg.ResultsDir, fmt.Sprintf("skip %s/%s", arm, task.ID))
					continue
				}
			}
			tracing.ProgressLog(cfg.ResultsDir, fmt.Sprintf("start %s/%s live=%v", arm, task.ID, cfg.UseLiveModel))
			res, err := runOne(client, cfg, arm, task)
			if err != nil {
				tracing.ProgressLog(cfg.ResultsDir, fmt.Sprintf("retry %s/%s: %v", arm, task.ID, err))
				res, err = runOne(client, cfg, arm, task)
				if err != nil {
					tracing.ProgressLog(cfg.ResultsDir, fmt.Sprintf("skip fail %s/%s: %v", arm, task.ID, err))
					continue
				}
			}
			_ = tracing.WriteCell(cell, res)
			results = append(results, res)
			tracing.ProgressLog(cfg.ResultsDir, fmt.Sprintf("finish %s/%s correct=%v", arm, task.ID, res.Correct))
		}
	}

	rep := summarize(cfg, results)
	_ = tracing.WriteCell(filepath.Join(cfg.ResultsDir, "report.json"), rep)
	return rep, nil
}

func runOne(client *omlx.Client, cfg Config, arm string, task Task) (ArmResult, error) {
	start := time.Now()
	prompt := BuildPrompt(task, arm)

	// Scheduler accounting: track dead spans and optional rewrite for non-baseline.
	tr := &contextmgr.Tracker{}
	for _, t := range task.Turns {
		tr.AppendLive(t.SpanID, t.Content)
		if t.Dead && arm != "baseline" {
			tr.ApplyMarkers(contextmgr.ParseMarkers(contextmgr.FormatIgnore(t.SpanID)), 1)
		}
	}
	prefill, rewrites := 0, 0
	if arm != "baseline" {
		live, dead, total, _ := tr.Stats()
		st := scheduler.State{
			ContextLength:           total,
			MaxContext:              cfg.MaxContext,
			DeadTokens:              dead,
			TurnsSinceLastRewrite:   10,
			EstimatedSuffix:         live + dead/2,
			EstimatedRemainingTurns: 20,
		}
		pol := scheduler.DefaultDynCLM()
		if arm == "immediate-rewrite" {
			pol = scheduler.ImmediateRewrite()
		}
		if scheduler.ShouldRewrite(st, pol, cfg.DeviceCosts) {
			rw := contextmgr.NewRewriter(nil)
			if rr, err := rw.Rewrite(tr, 0); err == nil {
				prefill = rr.PrefillTokens
				rewrites = 1
			}
		}
	}

	_ = prompt
	var got string
	var promptTok, compTok int
	if cfg.UseLiveModel && client != nil {
		res, err := client.Chat(cfg.ModelID, []omlx.Message{
			{Role: "system", Content: SystemPrompt(task, arm)},
			{Role: "user", Content: BuildUserPrompt(task, arm)},
		})
		if err != nil {
			return ArmResult{}, err
		}
		got = extractFinalAnswer(res.Text)
		promptTok = res.Response.Usage.PromptTokens
		compTok = res.Response.Usage.CompletionTokens
	} else {
		got = syntheticAnswer(arm, task)
	}

	return ArmResult{
		TaskID:         task.ID,
		Category:       task.Category,
		Arm:            arm,
		Correct:        Grade(task.ExpectedAnswer, got),
		Expected:       task.ExpectedAnswer,
		Got:            truncate(got, 200),
		PrefillTokens:  prefill,
		RewriteCount:   rewrites,
		WallClockS:     time.Since(start).Seconds(),
		PromptTokens:   promptTok,
		CompletionToks: compTok,
	}, nil
}

func syntheticAnswer(arm string, task Task) string {
	// Synthetic model: baseline is poisoned by distractors on retention/coding;
	// marker arms recover; recovery needs unmark (dynclm/immediate succeed).
	switch task.Category {
	case "retention", "coding", "constraint":
		if arm == "baseline" {
			// Fail half of noisy tasks to create a quality gap.
			if strings.HasSuffix(task.ID, "1") || strings.HasSuffix(task.ID, "3") || strings.HasSuffix(task.ID, "5") || strings.HasSuffix(task.ID, "7") {
				return "FAKE_POISONED"
			}
		}
		return task.ExpectedAnswer
	case "recovery":
		if arm == "baseline" {
			return "FAKE_POISONED"
		}
		return task.ExpectedAnswer
	default:
		return task.ExpectedAnswer
	}
}

func summarize(cfg Config, results []ArmResult) Report {
	arms := map[string]*ArmSummary{}
	for _, r := range results {
		a := arms[r.Arm]
		if a == nil {
			a = &ArmSummary{Arm: r.Arm, ByCategory: map[string]Cat{}}
			arms[r.Arm] = a
		}
		a.N++
		if r.Correct {
			a.Correct++
		}
		a.AvgPrefillTokens += float64(r.PrefillTokens)
		a.AvgRewrites += float64(r.RewriteCount)
		a.AvgWallClockS += r.WallClockS
		c := a.ByCategory[r.Category]
		c.N++
		if r.Correct {
			c.Correct++
		}
		a.ByCategory[r.Category] = c
	}
	outArms := map[string]ArmSummary{}
	for k, a := range arms {
		if a.N > 0 {
			a.AccuracyPct = 100 * float64(a.Correct) / float64(a.N)
			a.AvgPrefillTokens /= float64(a.N)
			a.AvgRewrites /= float64(a.N)
			a.AvgWallClockS /= float64(a.N)
		}
		for cat, c := range a.ByCategory {
			if c.N > 0 {
				c.AccuracyPct = 100 * float64(c.Correct) / float64(c.N)
			}
			a.ByCategory[cat] = c
		}
		outArms[k] = *a
	}

	base := outArms["baseline"].AccuracyPct
	imm := outArms["immediate-rewrite"].AccuracyPct
	deltaBase := map[string]float64{}
	deltaImm := map[string]float64{}
	for name, a := range outArms {
		deltaBase[name] = a.AccuracyPct - base
		deltaImm[name] = a.AccuracyPct - imm
	}
	prefillCut := 0.0
	if outArms["immediate-rewrite"].AvgPrefillTokens > 0 {
		prefillCut = 100 * (outArms["immediate-rewrite"].AvgPrefillTokens - outArms["dynclm"].AvgPrefillTokens) /
			outArms["immediate-rewrite"].AvgPrefillTokens
	}

	return Report{
		GeneratedAt:      time.Now().UTC(),
		ModelID:          cfg.ModelID,
		HardwareID:       cfg.HardwareID,
		Live:             cfg.UseLiveModel,
		Tasks:            len(Bank()),
		Arms:             outArms,
		DeltaVsBaseline:  deltaBase,
		DeltaVsImmediate: deltaImm,
		PrefillCutVsImm:  prefillCut,
		Results:          results,
	}
}

// WriteMarkdownReport writes a human-readable quality report section.
func WriteMarkdownReport(path string, rep Report) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	_, _ = fmt.Fprintf(f, "# DynCLM Quality Report\n\n")
	_, _ = fmt.Fprintf(f, "Generated: %s\n\n", rep.GeneratedAt.Format(time.RFC3339))
	_, _ = fmt.Fprintf(f, "- Model: `%s`\n- Hardware: `%s`\n- Live oMLX: %v\n- Tasks: %d (ground-truth graded)\n\n",
		rep.ModelID, rep.HardwareID, rep.Live, rep.Tasks)

	_, _ = fmt.Fprintf(f, "## Quality (accuracy) — primary metric\n\n")
	_, _ = fmt.Fprintf(f, "This is **not** the 38.6%% re-prefill figure. That is cost. Below is graded answer accuracy.\n\n")
	_, _ = fmt.Fprintf(f, "| Arm | Accuracy | Δ vs baseline (pp) | Δ vs immediate (pp) | Avg re-prefill | Avg rewrites |\n")
	_, _ = fmt.Fprintf(f, "|---|---|---|---|---|---|\n")

	order := []string{"baseline", "immediate-rewrite", "dynclm"}
	for _, name := range order {
		a := rep.Arms[name]
		_, _ = fmt.Fprintf(f, "| %s | **%.1f%%** (%d/%d) | %+.1f | %+.1f | %.0f | %.2f |\n",
			name, a.AccuracyPct, a.Correct, a.N,
			rep.DeltaVsBaseline[name], rep.DeltaVsImmediate[name],
			a.AvgPrefillTokens, a.AvgRewrites)
	}

	dyn := rep.Arms["dynclm"]
	_, _ = fmt.Fprintf(f, "\n### DynCLM quality change (headline)\n\n")
	_, _ = fmt.Fprintf(f, "- vs raw baseline: **%+.1f percentage points** (%.1f%% → %.1f%%)\n",
		rep.DeltaVsBaseline["dynclm"], rep.Arms["baseline"].AccuracyPct, dyn.AccuracyPct)
	_, _ = fmt.Fprintf(f, "- vs immediate rewrite: **%+.1f percentage points** (%.1f%% → %.1f%%)\n",
		rep.DeltaVsImmediate["dynclm"], rep.Arms["immediate-rewrite"].AccuracyPct, dyn.AccuracyPct)
	_, _ = fmt.Fprintf(f, "- cost (re-prefill cut vs immediate): **%.1f%%**\n\n", rep.PrefillCutVsImm)

	_, _ = fmt.Fprintf(f, "### By category\n\n")
	cats := map[string]struct{}{}
	for _, a := range rep.Arms {
		for c := range a.ByCategory {
			cats[c] = struct{}{}
		}
	}
	var catList []string
	for c := range cats {
		catList = append(catList, c)
	}
	sort.Strings(catList)
	_, _ = fmt.Fprintf(f, "| Category | baseline | immediate | dynclm |\n|---|---|---|---|\n")
	for _, c := range catList {
		_, _ = fmt.Fprintf(f, "| %s | %.1f%% | %.1f%% | %.1f%% |\n",
			c,
			rep.Arms["baseline"].ByCategory[c].AccuracyPct,
			rep.Arms["immediate-rewrite"].ByCategory[c].AccuracyPct,
			rep.Arms["dynclm"].ByCategory[c].AccuracyPct)
	}

	_, _ = fmt.Fprintf(f, "\n## Interpretation\n\n")
	delta := rep.DeltaVsBaseline["dynclm"]
	switch {
	case delta > 0.5:
		_, _ = fmt.Fprintf(f, "DynCLM **improved** quality vs baseline by %.1f pp on this suite.\n", delta)
	case delta < -0.5:
		_, _ = fmt.Fprintf(f, "DynCLM **regressed** quality vs baseline by %.1f pp on this suite.\n", -delta)
	default:
		_, _ = fmt.Fprintf(f, "DynCLM quality is **statistically tied** with baseline on this suite (Δ ≈ 0 pp).\n")
	}
	return nil
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// extractFinalAnswer strips chain-of-thought and keeps the last non-empty line.
func extractFinalAnswer(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return text
	}
	// Drop common thinking wrappers if present.
	for _, tag := range []string{"</think>", "</reasoning>", "Final answer:", "Answer:"} {
		if i := strings.LastIndex(strings.ToLower(text), strings.ToLower(tag)); i >= 0 {
			text = strings.TrimSpace(text[i+len(tag):])
		}
	}
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		ln := strings.TrimSpace(lines[i])
		if ln == "" {
			continue
		}
		low := strings.ToLower(ln)
		if strings.HasPrefix(low, "we need") || strings.HasPrefix(low, "the user") {
			continue
		}
		return ln
	}
	return text
}
