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
	"github.com/norbertvannobelen/dynamic-clm/internal/metrics"
	"github.com/norbertvannobelen/dynamic-clm/internal/scheduler"
	"github.com/norbertvannobelen/dynamic-clm/internal/tracing"
)

// ArmResult is one task × arm graded outcome with full efficiency metrics.
type ArmResult struct {
	TaskID   string `json:"task_id"`
	Category string `json:"category"`
	Arm      string `json:"arm"`
	Correct  bool   `json:"correct"`
	Expected string `json:"expected"`
	Got      string `json:"got"`

	ContextTokens     int `json:"context_tokens"`
	LiveTokens        int `json:"live_tokens"`
	DeadTokens        int `json:"dead_tokens"`
	PostRewriteCtx    int `json:"post_rewrite_context_tokens"`
	PrefillTokens     int `json:"prefill_tokens"`
	RewriteCount      int `json:"rewrite_count"`
	PromptTokens      int `json:"prompt_tokens"`
	CompletionToks    int `json:"completion_tokens"`
	TotalUsageTokens  int `json:"total_usage_tokens"`
	WallClockS        float64 `json:"wall_clock_s"`
}

// ArmSummary aggregates quality + efficiency for one arm.
type ArmSummary struct {
	Arm              string         `json:"arm"`
	N                int            `json:"n"`
	Correct          int            `json:"correct"`
	AccuracyPct      float64        `json:"accuracy_pct"`
	ByCategory       map[string]Cat `json:"by_category"`

	AvgContextTokens   float64 `json:"avg_context_tokens"`
	PeakContextTokens  float64 `json:"peak_context_tokens"`
	AvgLiveTokens      float64 `json:"avg_live_tokens"`
	AvgDeadTokens      float64 `json:"avg_dead_tokens"`
	AvgPostRewriteCtx  float64 `json:"avg_post_rewrite_context_tokens"`
	AvgPrefillTokens   float64 `json:"avg_prefill_tokens"`
	AvgRewrites        float64 `json:"avg_rewrites"`
	AvgPromptTokens    float64 `json:"avg_prompt_tokens"`
	AvgGeneratedTokens float64 `json:"avg_generated_tokens"`
	AvgTotalUsageTokens float64 `json:"avg_total_usage_tokens"`
	AvgWallClockS      float64 `json:"avg_wall_clock_s"`
	TotalWallClockS    float64 `json:"total_wall_clock_s"`
}

// Cat is per-category accuracy.
type Cat struct {
	N           int     `json:"n"`
	Correct     int     `json:"correct"`
	AccuracyPct float64 `json:"accuracy_pct"`
}

// Report is the full quality suite output.
type Report struct {
	GeneratedAt      time.Time                       `json:"generated_at"`
	ModelID          string                          `json:"model_id"`
	HardwareID       string                          `json:"hardware_id"`
	Live             bool                            `json:"live"`
	Tasks            int                             `json:"tasks"`
	Arms             map[string]ArmSummary           `json:"arms"`
	VsBaseline       map[string][]metrics.VsBaseline `json:"vs_baseline"`
	DeltaVsBaseline  map[string]float64              `json:"delta_vs_baseline_pp"`
	DeltaVsImmediate map[string]float64              `json:"delta_vs_immediate_pp"`
	PrefillCutVsImm  float64                         `json:"prefill_cut_vs_immediate_pct"`
	Results          []ArmResult                     `json:"results"`
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

	// Round-robin by task across arms (amendment 2026-10-15, docs/analysis-plan.md):
	// arm-major order let slow hardware drift (e.g. thermal) bias the arm that ran last.
	for _, task := range tasks {
		for _, arm := range arms {
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
			tracing.ProgressLog(cfg.ResultsDir, fmt.Sprintf(
				"finish %s/%s correct=%v ctx=%d prompt=%d gen=%d wall=%.2fs",
				arm, task.ID, res.Correct, res.ContextTokens, res.PromptTokens, res.CompletionToks, res.WallClockS))
		}
	}

	rep := summarize(cfg, results)
	_ = tracing.WriteCell(filepath.Join(cfg.ResultsDir, "report.json"), rep)
	return rep, nil
}

func runOne(client *omlx.Client, cfg Config, arm string, task Task) (ArmResult, error) {
	start := time.Now()

	tr := &contextmgr.Tracker{}
	for _, t := range task.Turns {
		tr.AppendLive(t.SpanID, t.Content)
		if t.Dead && arm != "baseline" {
			tr.ApplyMarkers(contextmgr.ParseMarkers(contextmgr.FormatIgnore(t.SpanID)), 1)
		}
	}
	live, dead, total, _ := tr.Stats()
	prefill, rewrites := 0, 0
	postCtx := total
	if arm != "baseline" {
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
				postCtx = rr.PostRewriteContext
			}
		}
	}

	var got string
	var promptTok, compTok int
	userPrompt := BuildUserPrompt(task, arm)
	sys := SystemPrompt(task, arm)
	if cfg.UseLiveModel && client != nil {
		res, err := client.Chat(cfg.ModelID, []omlx.Message{
			{Role: "system", Content: sys},
			{Role: "user", Content: userPrompt},
		})
		if err != nil {
			return ArmResult{}, err
		}
		got = extractFinalAnswer(res.Text)
		promptTok = res.Response.Usage.PromptTokens
		compTok = res.Response.Usage.CompletionTokens
	} else {
		got = syntheticAnswer(arm, task)
		promptTok = contextmgr.EstimateTokens(sys + userPrompt)
		compTok = contextmgr.EstimateTokens(got)
	}

	return ArmResult{
		TaskID:           task.ID,
		Category:         task.Category,
		Arm:              arm,
		Correct:          Grade(task.ExpectedAnswer, got),
		Expected:         task.ExpectedAnswer,
		Got:              truncate(got, 200),
		ContextTokens:    total,
		LiveTokens:       live,
		DeadTokens:       dead,
		PostRewriteCtx:   postCtx,
		PrefillTokens:    prefill,
		RewriteCount:     rewrites,
		PromptTokens:     promptTok,
		CompletionToks:   compTok,
		TotalUsageTokens: promptTok + compTok,
		WallClockS:       time.Since(start).Seconds(),
	}, nil
}

func syntheticAnswer(arm string, task Task) string {
	switch task.Category {
	case "retention", "coding", "constraint":
		if arm == "baseline" {
			if strings.HasSuffix(task.ID, "1") || strings.HasSuffix(task.ID, "3") || strings.HasSuffix(task.ID, "5") || strings.HasSuffix(task.ID, "7") || strings.HasSuffix(task.ID, "8") {
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
		a.TotalWallClockS += r.WallClockS
		a.AvgContextTokens += float64(r.ContextTokens)
		if float64(r.ContextTokens) > a.PeakContextTokens {
			a.PeakContextTokens = float64(r.ContextTokens)
		}
		a.AvgLiveTokens += float64(r.LiveTokens)
		a.AvgDeadTokens += float64(r.DeadTokens)
		a.AvgPostRewriteCtx += float64(r.PostRewriteCtx)
		a.AvgPromptTokens += float64(r.PromptTokens)
		a.AvgGeneratedTokens += float64(r.CompletionToks)
		a.AvgTotalUsageTokens += float64(r.TotalUsageTokens)
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
			n := float64(a.N)
			a.AccuracyPct = 100 * float64(a.Correct) / n
			a.AvgPrefillTokens /= n
			a.AvgRewrites /= n
			a.AvgWallClockS /= n
			a.AvgContextTokens /= n
			a.AvgLiveTokens /= n
			a.AvgDeadTokens /= n
			a.AvgPostRewriteCtx /= n
			a.AvgPromptTokens /= n
			a.AvgGeneratedTokens /= n
			a.AvgTotalUsageTokens /= n
		}
		for cat, c := range a.ByCategory {
			if c.N > 0 {
				c.AccuracyPct = 100 * float64(c.Correct) / float64(c.N)
			}
			a.ByCategory[cat] = c
		}
		outArms[k] = *a
	}

	baseAcc := outArms["baseline"].AccuracyPct
	immAcc := outArms["immediate-rewrite"].AccuracyPct
	deltaBase := map[string]float64{}
	deltaImm := map[string]float64{}
	vsBase := map[string][]metrics.VsBaseline{}
	baseSnap := summaryToSnapshot(outArms["baseline"])
	for name, a := range outArms {
		deltaBase[name] = a.AccuracyPct - baseAcc
		deltaImm[name] = a.AccuracyPct - immAcc
		vsBase[name] = metrics.CompareToBaseline(summaryToSnapshot(a), baseSnap)
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
		VsBaseline:       vsBase,
		DeltaVsBaseline:  deltaBase,
		DeltaVsImmediate: deltaImm,
		PrefillCutVsImm:  prefillCut,
		Results:          results,
	}
}

func summaryToSnapshot(a ArmSummary) metrics.Snapshot {
	s := metrics.Snapshot{
		Arm:                a.Arm,
		AccuracyPct:        a.AccuracyPct,
		N:                  a.N,
		Correct:            a.Correct,
		AvgContextTokens:   a.AvgContextTokens,
		PeakContextTokens:  a.PeakContextTokens,
		FinalContextTokens: a.AvgPostRewriteCtx,
		AvgDeadTokens:      a.AvgDeadTokens,
		AvgLiveTokens:      a.AvgLiveTokens,
		AvgPrefillTokens:   a.AvgPrefillTokens,
		AvgPromptTokens:    a.AvgPromptTokens,
		AvgGeneratedTokens: a.AvgGeneratedTokens,
		AvgRewrites:        a.AvgRewrites,
		AvgWallClockS:      a.AvgWallClockS,
		TotalWallClockS:    a.TotalWallClockS,
	}
	s.FinalizeDerived()
	return s
}

// WriteMarkdownReport writes a human-readable quality + efficiency report vs baseline.
func WriteMarkdownReport(path string, rep Report) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	_, _ = fmt.Fprintf(f, "# DynCLM Quality & Efficiency Report\n\n")
	_, _ = fmt.Fprintf(f, "Generated: %s\n\n", rep.GeneratedAt.Format(time.RFC3339))
	_, _ = fmt.Fprintf(f, "- Model: `%s`\n- Hardware: `%s`\n- Live oMLX: %v\n- Tasks: %d (ground-truth graded)\n",
		rep.ModelID, rep.HardwareID, rep.Live, rep.Tasks)
	_, _ = fmt.Fprintf(f, "- **Reference arm: raw baseline** (all deltas are vs baseline unless noted)\n\n")

	_, _ = fmt.Fprintf(f, "## Summary table (all arms)\n\n")
	_, _ = fmt.Fprintf(f, "| Arm | Accuracy | Avg ctx | Peak ctx | Prefill | Prompt | Generated | Wall-s | Rewrites |\n")
	_, _ = fmt.Fprintf(f, "|---|---|---|---|---|---|---|---|---|\n")
	for _, name := range []string{"baseline", "immediate-rewrite", "dynclm"} {
		a := rep.Arms[name]
		_, _ = fmt.Fprintf(f, "| %s | **%.1f%%** (%d/%d) | %.0f | %.0f | %.0f | %.0f | %.0f | %.2f | %.2f |\n",
			name, a.AccuracyPct, a.Correct, a.N,
			a.AvgContextTokens, a.PeakContextTokens,
			a.AvgPrefillTokens, a.AvgPromptTokens, a.AvgGeneratedTokens,
			a.AvgWallClockS, a.AvgRewrites)
	}

	_, _ = fmt.Fprintf(f, "\n## DynCLM vs raw baseline (full comparison)\n\n")
	_, _ = fmt.Fprintf(f, "| Metric | Baseline | DynCLM | Δ | %% change | Better than baseline? |\n")
	_, _ = fmt.Fprintf(f, "|---|---|---|---|---|---|\n")
	for _, row := range rep.VsBaseline["dynclm"] {
		better := "-"
		if row.BetterThanBase != nil {
			if *row.BetterThanBase {
				better = "yes"
			} else {
				better = "no"
			}
		}
		_, _ = fmt.Fprintf(f, "| %s | %.2f | %.2f | %s | %s | %s |\n",
			row.Metric, row.Baseline, row.Value, metrics.FormatDelta(row.Delta), metrics.FormatPct(row.PctChange), better)
	}

	_, _ = fmt.Fprintf(f, "\n## Immediate-rewrite vs raw baseline\n\n")
	_, _ = fmt.Fprintf(f, "| Metric | Baseline | Immediate | Δ | %% change | Better than baseline? |\n")
	_, _ = fmt.Fprintf(f, "|---|---|---|---|---|---|\n")
	for _, row := range rep.VsBaseline["immediate-rewrite"] {
		better := "-"
		if row.BetterThanBase != nil {
			if *row.BetterThanBase {
				better = "yes"
			} else {
				better = "no"
			}
		}
		_, _ = fmt.Fprintf(f, "| %s | %.2f | %.2f | %s | %s | %s |\n",
			row.Metric, row.Baseline, row.Value, metrics.FormatDelta(row.Delta), metrics.FormatPct(row.PctChange), better)
	}

	dyn := rep.Arms["dynclm"]
	base := rep.Arms["baseline"]
	_, _ = fmt.Fprintf(f, "\n## Headlines\n\n")
	_, _ = fmt.Fprintf(f, "- **Quality vs baseline:** %+.1f pp (%.1f%% → %.1f%%)\n",
		rep.DeltaVsBaseline["dynclm"], base.AccuracyPct, dyn.AccuracyPct)
	_, _ = fmt.Fprintf(f, "- **Quality vs immediate rewrite:** %+.1f pp\n", rep.DeltaVsImmediate["dynclm"])
	_, _ = fmt.Fprintf(f, "- **Wall-clock vs baseline:** %+.1f%% (%.2fs → %.2fs avg)\n",
		pct(dyn.AvgWallClockS, base.AvgWallClockS), base.AvgWallClockS, dyn.AvgWallClockS)
	_, _ = fmt.Fprintf(f, "- **Prompt tokens vs baseline:** %+.1f%% (%.0f → %.0f avg)\n",
		pct(dyn.AvgPromptTokens, base.AvgPromptTokens), base.AvgPromptTokens, dyn.AvgPromptTokens)
	_, _ = fmt.Fprintf(f, "- **Generated tokens vs baseline:** %+.1f%% (%.0f → %.0f avg)\n",
		pct(dyn.AvgGeneratedTokens, base.AvgGeneratedTokens), base.AvgGeneratedTokens, dyn.AvgGeneratedTokens)
	_, _ = fmt.Fprintf(f, "- **Context length (avg) vs baseline:** %+.1f%% (%.0f → %.0f)\n",
		pct(dyn.AvgContextTokens, base.AvgContextTokens), base.AvgContextTokens, dyn.AvgContextTokens)
	_, _ = fmt.Fprintf(f, "- **Rewrite prefill vs baseline:** %.0f → %.0f (baseline has none)\n",
		base.AvgPrefillTokens, dyn.AvgPrefillTokens)
	_, _ = fmt.Fprintf(f, "- **Rewrite prefill vs immediate:** cut %.1f%%\n\n", rep.PrefillCutVsImm)

	_, _ = fmt.Fprintf(f, "### By category (accuracy)\n\n")
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
		_, _ = fmt.Fprintf(f, "DynCLM quality is **tied** with baseline on this suite (Δ ≈ 0 pp).\n")
	}
	_, _ = fmt.Fprintf(f, "\nEfficiency vs baseline depends on the metric: rewrite prefill is an *extra* cost vs raw append-only; ")
	_, _ = fmt.Fprintf(f, "prompt/context/wall-clock deltas above show whether the end-to-end request is leaner or heavier.\n")
	return nil
}

func pct(val, base float64) float64 {
	if base == 0 {
		if val == 0 {
			return 0
		}
		return 100
	}
	return 100 * (val - base) / base
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func extractFinalAnswer(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return text
	}
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
