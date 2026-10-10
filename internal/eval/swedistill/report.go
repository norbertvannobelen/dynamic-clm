package swedistill

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// WriteMarkdownReport writes a detailed human-readable report.
// The extensive narrative with figures is produced by scripts/gen_swe_distill_report.py;
// this Go report is a complete standalone summary used during/after the run.
func WriteMarkdownReport(path string, rep Report) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	_, _ = fmt.Fprintf(f, "# DynCLM SWE-Distill Report\n\n")
	_, _ = fmt.Fprintf(f, "Generated: %s\n\n", rep.GeneratedAt.Format(time.RFC3339))
	_, _ = fmt.Fprintf(f, "**Disclaimer:** SWE-Bench-**inspired** distill — not an official SWE-Bench resolve-rate evaluation.\n\n")
	_, _ = fmt.Fprintf(f, "- Model: `%s` · Hardware: `%s` · Live: %v\n", rep.ModelID, rep.HardwareID, rep.Live)
	_, _ = fmt.Fprintf(f, "- max_context: **%d** · turns/task: **%d** · noise_pad: **%d** chars\n",
		rep.MaxContext, rep.MaxTurns, rep.NoisePad)
	_, _ = fmt.Fprintf(f, "- Arms: Baseline+compact · DynCLM · CLM (immediate-rewrite)\n\n")

	_, _ = fmt.Fprintf(f, "## Methodology\n\n%s\n\n", rep.Methodology)

	_, _ = fmt.Fprintf(f, "## Executive verdict\n\n")
	bc := rep.Arms["baseline-compact"]
	dyn := rep.Arms["dynclm"]
	imm := rep.Arms["immediate-rewrite"]
	_, _ = fmt.Fprintf(f, "| Arm | Accuracy | Correct/N | Avg final ctx | Peak ctx | Avg reclaim events | Avg prefill | Total wall (s) |\n")
	_, _ = fmt.Fprintf(f, "|---|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, name := range []string{"baseline-compact", "dynclm", "immediate-rewrite"} {
		a := rep.Arms[name]
		events := a.AvgCompacts + a.AvgRewrites
		label := name
		if name == "baseline-compact" {
			label = "Baseline+compact"
		}
		if name == "immediate-rewrite" {
			label = "CLM"
		}
		if name == "dynclm" {
			label = "DynCLM"
		}
		_, _ = fmt.Fprintf(f, "| %s | %.1f%% | %d/%d | %.0f | %d | %.1f | %.0f | %.1f |\n",
			label, a.AccuracyPct, a.Correct, a.N, a.AvgFinalContext, a.PeakContext, events, a.AvgPrefill, a.TotalWallS)
	}
	_, _ = fmt.Fprintf(f, "\n")
	_, _ = fmt.Fprintf(f, "DynCLM accuracy Δ vs Baseline+compact: **%+.1f pp**. CLM: **%+.1f pp**.\n",
		dyn.AccuracyPct-bc.AccuracyPct, imm.AccuracyPct-bc.AccuracyPct)
	_, _ = fmt.Fprintf(f, "DynCLM final ctx vs Baseline+compact: **%+.1f%%**. CLM: **%+.1f%%**.\n\n",
		pct(dyn.AvgFinalContext, bc.AvgFinalContext), pct(imm.AvgFinalContext, bc.AvgFinalContext))

	_, _ = fmt.Fprintf(f, "## Vs Baseline+compact\n\n")
	_, _ = fmt.Fprintf(f, "| Metric | DynCLM | CLM |\n|---|---|---|\n")
	vd, vi := rep.VsBaseline["dynclm"], rep.VsBaseline["immediate-rewrite"]
	_, _ = fmt.Fprintf(f, "| Accuracy Δ (pp) | %+.1f | %+.1f |\n", vd["accuracy_delta_pp"], vi["accuracy_delta_pp"])
	_, _ = fmt.Fprintf(f, "| Final ctx %% | %+.1f%% | %+.1f%% |\n", vd["ctx_pct"], vi["ctx_pct"])
	_, _ = fmt.Fprintf(f, "| Total wall %% | %+.1f%% | %+.1f%% |\n", vd["wall_pct"], vi["wall_pct"])
	_, _ = fmt.Fprintf(f, "| Prefill Δ (avg) | %+.0f | %+.0f |\n\n", vd["prefill_delta"], vi["prefill_delta"])

	_, _ = fmt.Fprintf(f, "## Per-task breakdown\n\n")
	_, _ = fmt.Fprintf(f, "| Instance | Expected | Baseline+compact | DynCLM | CLM | Ctx B/D/C |\n|---|---|:-:|:-:|:-:|---|\n")
	for _, t := range rep.Tasks {
		_, _ = fmt.Fprintf(f, "| `%s` | `%s` | %s | %s | %s | %d / %d / %d |\n",
			t.InstanceID,
			truncate(t.Expected, 40),
			passFail(t.ByArm["baseline-compact"]),
			passFail(t.ByArm["dynclm"]),
			passFail(t.ByArm["immediate-rewrite"]),
			t.CtxByArm["baseline-compact"],
			t.CtxByArm["dynclm"],
			t.CtxByArm["immediate-rewrite"],
		)
	}
	_, _ = fmt.Fprintf(f, "\n")

	_, _ = fmt.Fprintf(f, "## Failure analysis\n\n")
	var baseFails, dynFails, immFails []string
	for _, c := range rep.Cells {
		if c.Success {
			continue
		}
		line := fmt.Sprintf("- `%s`/%s: got `%s` (spec_survived=%v, compacts=%d, rewrites=%d)",
			c.InstanceID, c.Arm, truncate(c.Got, 80), c.SpecSurvived, c.Compacts, c.Rewrites)
		switch c.Arm {
		case "baseline-compact":
			baseFails = append(baseFails, line)
		case "dynclm":
			dynFails = append(dynFails, line)
		case "immediate-rewrite":
			immFails = append(immFails, line)
		}
	}
	writeFailSection := func(title string, lines []string) {
		_, _ = fmt.Fprintf(f, "### %s\n\n", title)
		if len(lines) == 0 {
			_, _ = fmt.Fprintf(f, "_None._\n\n")
			return
		}
		_, _ = fmt.Fprintf(f, "%s\n\n", strings.Join(lines, "\n"))
	}
	writeFailSection("Baseline+compact failures", baseFails)
	writeFailSection("DynCLM failures", dynFails)
	writeFailSection("CLM failures", immFails)

	_, _ = fmt.Fprintf(f, "## Efficiency deep-dive\n\n")
	_, _ = fmt.Fprintf(f, "| Arm | Spec survive %% | Avg compacts | Avg rewrites | Avg prefill |\n|---|---:|---:|---:|---:|\n")
	for _, name := range []string{"baseline-compact", "dynclm", "immediate-rewrite"} {
		a := rep.Arms[name]
		_, _ = fmt.Fprintf(f, "| %s | %.1f | %.2f | %.2f | %.0f |\n", name, a.SpecSurvivePct, a.AvgCompacts, a.AvgRewrites, a.AvgPrefill)
	}
	_, _ = fmt.Fprintf(f, "\nBlind compaction can look fast while dropping the early SPEC (spec survive %.1f%%). "+
		"DynCLM/CLM keep the SPEC via soft-invalidation (%.1f%% / %.1f%%).\n\n",
		bc.SpecSurvivePct, dyn.SpecSurvivePct, imm.SpecSurvivePct)

	_, _ = fmt.Fprintf(f, "## Figures\n\n")
	_, _ = fmt.Fprintf(f, "Regenerate with `python3 scripts/gen_swe_distill_report.py --report <report.json>`.\n\n")
	_, _ = fmt.Fprintf(f, "- ![accuracy](figures/swe_probe_accuracy.svg)\n")
	_, _ = fmt.Fprintf(f, "- ![context](figures/swe_final_context.svg)\n")
	_, _ = fmt.Fprintf(f, "- ![wall](figures/swe_wall.svg)\n")
	_, _ = fmt.Fprintf(f, "- ![prefill](figures/swe_prefill.svg)\n")
	_, _ = fmt.Fprintf(f, "- ![per-task](figures/swe_per_task_accuracy.svg)\n\n")

	_, _ = fmt.Fprintf(f, "## Artifacts\n\n")
	keys := make([]string, 0, len(rep.Arms))
	for k := range rep.Arms {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	_, _ = fmt.Fprintf(f, "- Arms present: %s\n", strings.Join(keys, ", "))
	_, _ = fmt.Fprintf(f, "- Cells: %d\n", len(rep.Cells))
	return nil
}

func passFail(ok bool) string {
	if ok {
		return "PASS"
	}
	return "FAIL"
}
