package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/norbertvannobelen/dynamic-clm/internal/eval/longhorizon"
	"github.com/norbertvannobelen/dynamic-clm/internal/scheduler"
)

func main() {
	model := flag.String("model", "Qwen3.8-27B-4bit", "model id")
	out := flag.String("out", "docs/longhorizon-report.md", "markdown report")
	results := flag.String("results", "results/longhorizon", "results dir")
	turns := flag.Int("turns", 24, "turns per session")
	sessions := flag.Int("sessions", 3, "sessions per arm")
	probeEvery := flag.Int("probe-every", 4, "probe interval")
	maxContext := flag.Int("max-context", 8192, "max context (suite A)")
	offline := flag.Bool("offline", false, "deprecated alias for --allow-synthetic")
	allowSynthetic := flag.Bool("allow-synthetic", false, "permit synthetic (no oMLX)")
	fresh := flag.Bool("fresh", false, "delete prior cells")
	mode := flag.String("mode", "a", "suite mode: a (raw baseline) or b (compaction matrix)")
	limits := flag.String("limits", "8192,16384,32768,65536", "comma-separated max contexts for mode b")
	flag.Parse()

	if *fresh {
		_ = os.RemoveAll(*results)
	}
	if *offline && !*allowSynthetic {
		fmt.Fprintln(os.Stderr, "--offline is disabled; pass --allow-synthetic for a non-live run")
		os.Exit(1)
	}
	live := !*allowSynthetic
	if !live {
		fmt.Fprintln(os.Stderr, "live evaluation required: refusing synthetic long-horizon (pass --allow-synthetic to override)")
		os.Exit(1)
	}

	cal := scheduler.DefaultM5MaxPrior(*model)
	if c, err := scheduler.LoadCalibration("configs/calibration/m5max-qwen38.json"); err == nil {
		cal = c
	}

	cfg := longhorizon.Config{
		ModelID:      *model,
		HardwareID:   "apple-m5-max",
		ResultsDir:   *results,
		UseLiveModel: live,
		DeviceCosts:  cal.ToDeviceCosts(),
		MaxContext:   *maxContext,
		MaxTurns:     *turns,
		Sessions:     *sessions,
		ProbeEvery:   *probeEvery,
		NoisePad:     1200,
	}

	switch strings.ToLower(*mode) {
	case "b", "compaction", "matrix":
		mcs := parseLimits(*limits)
		if *results == "results/longhorizon" {
			cfg.ResultsDir = "results/longhorizon_compaction"
			if *fresh {
				_ = os.RemoveAll(cfg.ResultsDir)
			}
		}
		if *out == "docs/longhorizon-report.md" {
			*out = "docs/longhorizon-compaction-report.md"
		}
		m, err := longhorizon.RunCompactionMatrix(cfg, mcs)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := longhorizon.WriteMatrixMarkdownReport(*out, m); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		enc, _ := json.MarshalIndent(map[string]any{
			"live":         m.Live,
			"max_contexts": m.MaxContexts,
			"noise_pad":    m.NoisePad,
			"by_max_context": m.ByMaxContext,
		}, "", "  ")
		fmt.Println(string(enc))
		fmt.Println("long-horizon B report →", *out)
	default:
		rep, err := longhorizon.Run(cfg)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := longhorizon.WriteMarkdownReport(*out, rep); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		b, d := rep.Arms["baseline"], rep.Arms["dynclm"]
		imm := rep.Arms["immediate-rewrite"]
		summary := map[string]any{
			"live": rep.Live,
			"late_probe_accuracy": map[string]float64{
				"baseline": b.Late.ProbeAccuracyPct,
				"dynclm":   d.Late.ProbeAccuracyPct,
				"delta_pp": d.Late.ProbeAccuracyPct - b.Late.ProbeAccuracyPct,
			},
			"late_avg_context": map[string]float64{
				"baseline": b.Late.AvgContextTokens,
				"dynclm":   d.Late.AvgContextTokens,
				"pct":      pct(d.Late.AvgContextTokens, b.Late.AvgContextTokens),
			},
			"late_steady_wall_s": map[string]float64{
				"baseline":        b.LateSteady.AvgWallClockS,
				"dynclm":          d.LateSteady.AvgWallClockS,
				"immediate":       imm.LateSteady.AvgWallClockS,
				"dyn_pct_vs_base": pct(d.LateSteady.AvgWallClockS, b.LateSteady.AvgWallClockS),
				"imm_pct_vs_base": pct(imm.LateSteady.AvgWallClockS, b.LateSteady.AvgWallClockS),
			},
			"late_steady_gen_tok_s": map[string]float64{
				"baseline":    b.LateSteady.AvgGenTokensPerSec,
				"dynclm":      d.LateSteady.AvgGenTokensPerSec,
				"immediate":   imm.LateSteady.AvgGenTokensPerSec,
				"pct_vs_base": pct(d.LateSteady.AvgGenTokensPerSec, b.LateSteady.AvgGenTokensPerSec),
			},
		}
		enc, _ := json.MarshalIndent(summary, "", "  ")
		fmt.Println(string(enc))
		fmt.Println("long-horizon A report →", *out)
	}
}

func parseLimits(s string) []int {
	var out []int
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil || n <= 0 {
			continue
		}
		out = append(out, n)
	}
	return out
}

func pct(val, base float64) float64 {
	if base == 0 {
		return 0
	}
	return 100 * (val - base) / base
}
