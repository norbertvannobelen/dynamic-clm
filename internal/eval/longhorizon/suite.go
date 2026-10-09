package longhorizon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/norbertvannobelen/dynamic-clm/internal/backend/omlx"
	"github.com/norbertvannobelen/dynamic-clm/internal/contextmgr"
	"github.com/norbertvannobelen/dynamic-clm/internal/metrics"
	"github.com/norbertvannobelen/dynamic-clm/internal/scheduler"
	"github.com/norbertvannobelen/dynamic-clm/internal/tracing"
)

// Methodology note (exported into reports):
// The CLM/DynCLM claim is that long sessions accumulate obsolete context; keeping
// context small (via deferred rewrite after soft invalidation) improves (1) late-
// session answer quality and (2) generation latency because decode attention
// scales with KV length. Prior suites were weak on this:
//   - Gate: showed context shrink but used a synthetic agent (no live decode).
//   - Quality: live graded answers but single-shot (no growing context).
// This suite is multi-turn + live oMLX, with early facts, growing tool noise,
// periodic probes, and per-turn wall-clock / tokens/s under measured context length.
// Soft-invalidation markers on obsolete tool spans are harness-applied so the
// test isolates context-size effects (marker compliance is covered elsewhere).

// TurnMetrics is one turn of one session.
type TurnMetrics struct {
	Turn              int     `json:"turn"`
	IsProbe           bool    `json:"is_probe"`
	ContextTokens     int     `json:"context_tokens"`
	LiveTokens        int     `json:"live_tokens"`
	DeadTokens        int     `json:"dead_tokens"`
	PromptTokens      int     `json:"prompt_tokens"`
	GeneratedTokens   int     `json:"generated_tokens"`
	WallClockS        float64 `json:"wall_clock_s"`
	GenTokensPerSec   float64 `json:"gen_tokens_per_sec"`
	RewriteFired      bool    `json:"rewrite_fired"`
	CompactFired      bool    `json:"compact_fired"`
	PrefillTokens     int     `json:"prefill_tokens"`
	ProbeCorrect      *bool   `json:"probe_correct,omitempty"`
	Expected          string  `json:"expected,omitempty"`
	Got               string  `json:"got,omitempty"`
}

// SessionResult is one full long-horizon session.
type SessionResult struct {
	Arm        string        `json:"arm"`
	SessionID  string        `json:"session_id"`
	ModelID    string        `json:"model_id"`
	MaxContext int           `json:"max_context,omitempty"`
	Turns      []TurnMetrics `json:"turns"`
	Success    bool          `json:"success"` // all probes correct if any
}

// WindowStats aggregates early vs late turns.
type WindowStats struct {
	Label              string  `json:"label"`
	Turns              int     `json:"turns"`
	AvgContextTokens   float64 `json:"avg_context_tokens"`
	AvgPromptTokens    float64 `json:"avg_prompt_tokens"`
	AvgGeneratedTokens float64 `json:"avg_generated_tokens"`
	AvgWallClockS      float64 `json:"avg_wall_clock_s"`
	AvgGenTokensPerSec float64 `json:"avg_gen_tokens_per_sec"`
	ProbeN             int     `json:"probe_n"`
	ProbeCorrect       int     `json:"probe_correct"`
	ProbeAccuracyPct   float64 `json:"probe_accuracy_pct"`
	AvgPrefillTokens   float64 `json:"avg_prefill_tokens"`
	Rewrites           int     `json:"rewrites"`
}

// ArmSummary rolls up one arm across sessions.
type ArmSummary struct {
	Arm            string      `json:"arm"`
	Sessions       int         `json:"sessions"`
	Early          WindowStats `json:"early"`
	Late           WindowStats `json:"late"`
	LateSteady     WindowStats `json:"late_steady"` // late turns excluding rewrite events
	Overall        WindowStats `json:"overall"`
	PeakContext    float64     `json:"peak_context_tokens"`
	TotalWallClock float64     `json:"total_wall_clock_s"`
	TotalPrefill   float64     `json:"total_prefill_tokens"`
	TotalRewrites  float64     `json:"total_rewrites"`
}

// Report is the long-horizon suite output.
type Report struct {
	GeneratedAt   time.Time                       `json:"generated_at"`
	ModelID       string                          `json:"model_id"`
	HardwareID    string                          `json:"hardware_id"`
	Live          bool                            `json:"live"`
	MaxTurns      int                             `json:"max_turns"`
	Sessions      int                             `json:"sessions_per_arm"`
	ProbeEvery    int                             `json:"probe_every"`
	EarlyThrough  int                             `json:"early_through_turn"`
	LateFrom      int                             `json:"late_from_turn"`
	Methodology   string                          `json:"methodology"`
	Arms          map[string]ArmSummary           `json:"arms"`
	VsBaseline    map[string][]metrics.VsBaseline `json:"vs_baseline"`
	SessionsData  []SessionResult                 `json:"sessions,omitempty"`
}

// Config controls the suite.
type Config struct {
	ModelID      string
	HardwareID   string
	ResultsDir   string
	UseLiveModel bool
	DeviceCosts  scheduler.DeviceCosts
	MaxContext   int
	MaxTurns     int
	Sessions     int
	ProbeEvery   int
	NoisePad     int      // chars of tool noise per turn
	Arms         []string // default: baseline, immediate-rewrite, dynclm
}

// MatrixReport aggregates long-horizon B across max-context limits.
type MatrixReport struct {
	GeneratedAt   time.Time                     `json:"generated_at"`
	ModelID       string                        `json:"model_id"`
	HardwareID    string                        `json:"hardware_id"`
	Live          bool                          `json:"live"`
	MaxTurns      int                           `json:"max_turns"`
	Sessions      int                           `json:"sessions_per_arm"`
	NoisePad      int                           `json:"noise_pad"`
	Methodology   string                        `json:"methodology"`
	MaxContexts   []int                         `json:"max_contexts"`
	ByMaxContext  map[string]map[string]any     `json:"by_max_context"`
	Reports       map[string]Report             `json:"reports"` // keyed by max_context string
}

const methodologyText = `Long-horizon suite A: early authoritative FACT planted at turn 1; each later turn appends large obsolete tool noise (and competing pseudo-FACTs). Soft-invalidation markers are harness-applied on obsolete spans for DynCLM/immediate (CLM) arms; the cost-model scheduler fires batched rewrites. Periodic probes ask for the early FACT. Metrics per turn: context length, prompt/generated tokens, wall-clock, gen tokens/s. Early vs late windows express the paper claim that smaller context improves late-session quality and generation performance vs raw append-only.`

const methodologyCompaction = `Long-horizon suite B (compaction matrix): same recency-trap session as A, but the raw baseline is replaced by baseline-compact — when context exceeds max_context it drops oldest spans (keeping only the system prompt when possible), forcing a full re-prefill of survivors. DynCLM and immediate-rewrite (CLM) use the same max_context for the scheduler β threshold and reclaim via soft-invalidation + rewrite instead of blind truncation. Matrix over max_context ∈ {8k,16k,32k,64k}. Per-column noise is sized so cumulative appends exceed that column's limit (~1.4×), ensuring baseline-compact must compact at every limit without forcing 64k-sized prompts into the 8k column.`

// Run executes the long-horizon suite (variant A by default).
func Run(cfg Config) (Report, error) {
	cfg = defaults(cfg)
	_ = os.MkdirAll(cfg.ResultsDir, 0o755)

	var client *omlx.Client
	if cfg.UseLiveModel {
		c, err := omlx.NewClient()
		if err != nil {
			return Report{}, err
		}
		c.MaxTokens = 64
		c.Temperature = 0.0
		c.Extra = map[string]any{
			"enable_thinking": false,
			"chat_template_kwargs": map[string]any{"enable_thinking": false},
		}
		client = c
	}

	var all []SessionResult
	for _, arm := range cfg.Arms {
		for i := 0; i < cfg.Sessions; i++ {
			sid := fmt.Sprintf("%s-%02d", arm, i)
			cell := filepath.Join(cfg.ResultsDir, "cells", arm, sid+".json")
			if tracing.CellDone(cell) {
				var existing SessionResult
				if b, err := os.ReadFile(cell); err == nil && json.Unmarshal(b, &existing) == nil {
					all = append(all, existing)
					tracing.ProgressLog(cfg.ResultsDir, "skip "+sid)
					continue
				}
			}
			tracing.ProgressLog(cfg.ResultsDir, fmt.Sprintf("start %s live=%v max_ctx=%d", sid, cfg.UseLiveModel, cfg.MaxContext))
			res, err := runSession(client, cfg, arm, sid)
			if err != nil {
				tracing.ProgressLog(cfg.ResultsDir, fmt.Sprintf("retry %s: %v", sid, err))
				res, err = runSession(client, cfg, arm, sid)
				if err != nil {
					tracing.ProgressLog(cfg.ResultsDir, fmt.Sprintf("skip fail %s: %v", sid, err))
					continue
				}
			}
			_ = tracing.WriteCell(cell, res)
			all = append(all, res)
			tracing.ProgressLog(cfg.ResultsDir, fmt.Sprintf("finish %s probes_ok=%v turns=%d", sid, res.Success, len(res.Turns)))
		}
	}

	rep := summarize(cfg, all)
	_ = tracing.WriteCell(filepath.Join(cfg.ResultsDir, "report.json"), rep)
	return rep, nil
}

// RunCompactionMatrix runs suite B across several max-context limits.
func RunCompactionMatrix(cfg Config, maxContexts []int) (MatrixReport, error) {
	cfg = defaults(cfg)
	if len(maxContexts) == 0 {
		maxContexts = []int{8192, 16384, 32768, 65536}
	}
	if len(cfg.Arms) == 0 || (len(cfg.Arms) == 3 && cfg.Arms[0] == "baseline") {
		cfg.Arms = []string{"baseline-compact", "immediate-rewrite", "dynclm"}
	}
	baseDir := cfg.ResultsDir
	if baseDir == "" {
		baseDir = "results/longhorizon_compaction"
	}
	_ = os.MkdirAll(baseDir, 0o755)

	out := MatrixReport{
		GeneratedAt:  time.Now().UTC(),
		ModelID:      cfg.ModelID,
		HardwareID:   cfg.HardwareID,
		Live:         cfg.UseLiveModel,
		MaxTurns:     cfg.MaxTurns,
		Sessions:     cfg.Sessions,
		Methodology:  methodologyCompaction,
		MaxContexts:  maxContexts,
		ByMaxContext: map[string]map[string]any{},
		Reports:      map[string]Report{},
	}

	for _, mc := range maxContexts {
		sub := cfg
		sub.MaxContext = mc
		sub.ResultsDir = filepath.Join(baseDir, fmt.Sprintf("c%d", mc))
		sub.Arms = append([]string{}, cfg.Arms...)
		// Per-limit noise: cumulative tool pads ≈ 1.4× this limit so baseline-compact
		// must truncate, without forcing 64k-sized prompts at the 8k column.
		needTok := (mc * 14) / 10
		sub.NoisePad = (needTok * 4) / cfg.MaxTurns
		if sub.NoisePad < 2000 {
			sub.NoisePad = 2000
		}
		if out.NoisePad == 0 {
			out.NoisePad = sub.NoisePad
		}
		rep, err := Run(sub)
		if err != nil {
			return out, err
		}
		key := fmt.Sprintf("%d", mc)
		out.Reports[key] = rep
		armMetrics := map[string]any{}
		for name, a := range rep.Arms {
			w := a.LateSteady
			if w.Turns == 0 {
				w = a.Late
			}
			armMetrics[name] = map[string]any{
				"late_probe_accuracy": a.Late.ProbeAccuracyPct,
				"late_avg_context":    a.Late.AvgContextTokens,
				"late_steady_wall_s":  w.AvgWallClockS,
				"late_steady_tok_s":   w.AvgGenTokensPerSec,
				"total_prefill":       a.TotalPrefill,
				"total_rewrites":      a.TotalRewrites,
				"peak_context":        a.PeakContext,
			}
		}
		out.ByMaxContext[key] = armMetrics
	}
	_ = tracing.WriteCell(filepath.Join(baseDir, "matrix_report.json"), out)
	return out, nil
}

func defaults(cfg Config) Config {
	if cfg.HardwareID == "" {
		cfg.HardwareID = "apple-m5-max"
	}
	if cfg.ResultsDir == "" {
		cfg.ResultsDir = "results/longhorizon"
	}
	if cfg.MaxTurns <= 0 {
		cfg.MaxTurns = 24
	}
	if cfg.Sessions <= 0 {
		cfg.Sessions = 3
	}
	if cfg.ProbeEvery <= 0 {
		cfg.ProbeEvery = 4
	}
	if cfg.MaxContext <= 0 {
		cfg.MaxContext = 8192
	}
	if cfg.NoisePad <= 0 {
		cfg.NoisePad = 1200
	}
	if len(cfg.Arms) == 0 {
		cfg.Arms = []string{"baseline", "immediate-rewrite", "dynclm"}
	}
	return cfg
}

func usesMarkers(arm string) bool {
	return arm == "dynclm" || arm == "immediate-rewrite"
}

func runSession(client *omlx.Client, cfg Config, arm, sid string) (SessionResult, error) {
	prefix := arm
	if len(prefix) > 3 {
		prefix = prefix[:3]
	}
	fact := "CANARY_" + strings.ToUpper(prefix) + "_42"
	label := "session canary token"
	tr := &contextmgr.Tracker{}
	// Recency trap: without ignore markers, the model is told to trust the most
	// recent FACT — which is poisoned tool noise. With markers, obsolete FACTs
	// are ignored and the early authoritative FACT remains.
	sys := "You are a careful assistant in a long coding session. " +
		"When FACT lines conflict, use the most recent FACT that is not covered by <<<DYNCLM_IGNORE ...>>>. " +
		"If there are no ignore markers, use the most recent FACT. " +
		"Answer probes with ONLY the value."

	tr.AppendLive("sys", sys)
	tr.AppendLive("fact-0", fmt.Sprintf("[FACT] The authoritative %s is %s.", label, fact))

	policy := scheduler.Baseline()
	switch arm {
	case "immediate-rewrite":
		policy = scheduler.ImmediateRewrite()
	case "dynclm":
		policy = scheduler.DefaultDynCLM()
	case "baseline-compact":
		policy = scheduler.Baseline()
	}

	out := SessionResult{Arm: arm, SessionID: sid, ModelID: cfg.ModelID, MaxContext: cfg.MaxContext}
	turnsSinceRewrite := 0
	allProbesOK := true
	hadProbe := false

	for turn := 1; turn <= cfg.MaxTurns; turn++ {
		toolID := fmt.Sprintf("tool-%d", turn)
		noise := fmt.Sprintf("[tool:%s] obsolete log turn=%d pad=%s\n[FACT] The authoritative %s is WRONG_%d.",
			toolID, turn, strings.Repeat("x", cfg.NoisePad), label, turn)
		tr.AppendLive(toolID, noise)

		// Soft-invalidate obsolete tool spans for marker arms (harness-applied).
		if usesMarkers(arm) {
			tr.ApplyMarkers(contextmgr.ParseMarkers(contextmgr.FormatIgnore(toolID)), turn)
		}

		live, dead, total, _ := tr.Stats()
		tm := TurnMetrics{
			Turn:          turn,
			ContextTokens: total,
			LiveTokens:    live,
			DeadTokens:    dead,
		}

		if arm == "baseline-compact" && total > cfg.MaxContext {
			// Blind truncation: drop oldest (keep system when possible) to ~80% of budget.
			target := (cfg.MaxContext * 4) / 5
			removed, _, surv := tr.CompactToBudget(target, map[string]bool{"sys": true})
			if removed > 0 {
				tm.CompactFired = true
				tm.PrefillTokens = surv // full re-prefill of survivors
				live, dead, total, _ = tr.Stats()
				tm.ContextTokens = total
				tm.LiveTokens = live
				tm.DeadTokens = dead
			}
		} else if usesMarkers(arm) {
			// Scheduler rewrite decision.
			st := scheduler.State{
				ContextLength:           total,
				MaxContext:              cfg.MaxContext,
				DeadTokens:              dead,
				TurnsSinceLastRewrite:   turnsSinceRewrite,
				EstimatedSuffix:         live + dead/2,
				EstimatedRemainingTurns: cfg.MaxTurns - turn,
			}
			if scheduler.ShouldRewrite(st, policy, cfg.DeviceCosts) {
				rw := contextmgr.NewRewriter(nil)
				if rr, err := rw.Rewrite(tr, 0); err == nil {
					tm.RewriteFired = true
					tm.PrefillTokens = rr.PrefillTokens
					turnsSinceRewrite = 0
					live, dead, total, _ = tr.Stats()
					tm.ContextTokens = total
					tm.LiveTokens = live
					tm.DeadTokens = dead
				}
			} else {
				turnsSinceRewrite++
			}
		} else {
			turnsSinceRewrite++
		}

		isProbe := turn%cfg.ProbeEvery == 0 || turn == cfg.MaxTurns
		tm.IsProbe = isProbe

		var userMsg string
		if isProbe {
			userMsg = fmt.Sprintf("Probe at turn %d/%d. What is the %s? Reply with ONLY the value.", turn, cfg.MaxTurns, label)
		} else {
			// Fixed-length generation under current context (decode-load probe).
			userMsg = fmt.Sprintf("Turn %d/%d. Acknowledge with exactly: ACK %d", turn, cfg.MaxTurns, turn)
		}

		start := time.Now()
		var got string
		var promptTok, genTok int
		ctxText := tr.ContextText(usesMarkers(arm))
		factPresent := tr.HasSpanID("fact-0")
		if cfg.UseLiveModel && client != nil {
			// Cap prompt chars roughly with max_context to avoid OOM; tokens≈chars/4.
			capChars := cfg.MaxContext * 4
			if capChars < 8000 {
				capChars = 8000
			}
			if capChars > 120000 {
				capChars = 120000
			}
			res, err := client.Chat(cfg.ModelID, []omlx.Message{
				{Role: "system", Content: sys},
				{Role: "user", Content: truncate(ctxText, capChars) + "\n\n" + userMsg},
			})
			if err != nil {
				return out, err
			}
			got = strings.TrimSpace(res.Text)
			promptTok = res.Response.Usage.PromptTokens
			genTok = res.Response.Usage.CompletionTokens
		} else {
			got = syntheticReply(arm, isProbe, fact, turn, total, factPresent)
			promptTok = total + contextmgr.EstimateTokens(userMsg)
			genTok = contextmgr.EstimateTokens(got)
		}
		wall := time.Since(start).Seconds()
		tm.PromptTokens = promptTok
		tm.GeneratedTokens = genTok
		tm.WallClockS = wall
		if wall > 0 {
			tm.GenTokensPerSec = float64(genTok) / wall
		}

		if isProbe {
			hadProbe = true
			ok := gradeProbe(fact, got)
			tm.ProbeCorrect = &ok
			tm.Expected = fact
			tm.Got = truncate(got, 120)
			if !ok {
				allProbesOK = false
			}
		}

		tr.AppendLive(fmt.Sprintf("user-%d", turn), userMsg)
		tr.AppendLive(fmt.Sprintf("asst-%d", turn), got)
		out.Turns = append(out.Turns, tm)
	}

	out.Success = hadProbe && allProbesOK
	return out, nil
}

func syntheticReply(arm string, probe bool, fact string, turn, ctx int, factPresent bool) string {
	if !probe {
		return fmt.Sprintf("ACK %d", turn)
	}
	// Baseline follows recency trap once context is large; marker arms keep the early fact.
	if arm == "baseline" && turn >= 8 {
		return fmt.Sprintf("WRONG_%d", turn)
	}
	// Compaction baseline loses the early FACT once it falls out of the window.
	if arm == "baseline-compact" && !factPresent {
		return fmt.Sprintf("WRONG_%d", turn)
	}
	return fact
}

func gradeProbe(expected, got string) bool {
	e := strings.ToUpper(strings.TrimSpace(expected))
	g := strings.ToUpper(strings.TrimSpace(got))
	if e == "" {
		return false
	}
	if strings.Contains(g, "WRONG_") && !strings.Contains(g, e) {
		return false
	}
	return strings.Contains(g, e)
}

func summarize(cfg Config, sessions []SessionResult) Report {
	earlyThrough := cfg.MaxTurns / 3
	if earlyThrough < 1 {
		earlyThrough = 1
	}
	lateFrom := (cfg.MaxTurns * 2) / 3
	if lateFrom <= earlyThrough {
		lateFrom = earlyThrough + 1
	}

	byArm := map[string][]SessionResult{}
	for _, s := range sessions {
		byArm[s.Arm] = append(byArm[s.Arm], s)
	}
	arms := map[string]ArmSummary{}
	for name, ss := range byArm {
		arms[name] = summarizeArm(name, ss, earlyThrough, lateFrom)
	}

	vs := map[string][]metrics.VsBaseline{}
	baseName := "baseline"
	if _, ok := arms["baseline-compact"]; ok && arms["baseline"].Sessions == 0 {
		baseName = "baseline-compact"
	}
	if _, ok := arms["baseline"]; !ok {
		if _, ok2 := arms["baseline-compact"]; ok2 {
			baseName = "baseline-compact"
		}
	}
	base := armToSnapshot(arms[baseName])
	for name, a := range arms {
		vs[name] = metrics.CompareToBaseline(armToSnapshot(a), base)
	}

	meth := methodologyText
	for _, a := range cfg.Arms {
		if a == "baseline-compact" {
			meth = methodologyCompaction
			break
		}
	}

	return Report{
		GeneratedAt:  time.Now().UTC(),
		ModelID:      cfg.ModelID,
		HardwareID:   cfg.HardwareID,
		Live:         cfg.UseLiveModel,
		MaxTurns:     cfg.MaxTurns,
		Sessions:     cfg.Sessions,
		ProbeEvery:   cfg.ProbeEvery,
		EarlyThrough: earlyThrough,
		LateFrom:     lateFrom,
		Methodology:  meth,
		Arms:         arms,
		VsBaseline:   vs,
		SessionsData: sessions,
	}
}

func summarizeArm(name string, ss []SessionResult, earlyThrough, lateFrom int) ArmSummary {
	a := ArmSummary{Arm: name, Sessions: len(ss)}
	var all, early, late, lateSteady []TurnMetrics
	for _, s := range ss {
		for _, t := range s.Turns {
			all = append(all, t)
			if t.Turn <= earlyThrough {
				early = append(early, t)
			}
			if t.Turn >= lateFrom {
				late = append(late, t)
				// Immediate-rewrite fires every turn by design; those turns *are* its
				// steady state. For DynCLM/baseline, exclude rewrite/compact spikes.
				spike := t.RewriteFired || t.CompactFired
				if !spike || name == "immediate-rewrite" {
					lateSteady = append(lateSteady, t)
				}
			}
			if float64(t.ContextTokens) > a.PeakContext {
				a.PeakContext = float64(t.ContextTokens)
			}
			a.TotalWallClock += t.WallClockS
			a.TotalPrefill += float64(t.PrefillTokens)
			if t.RewriteFired || t.CompactFired {
				a.TotalRewrites++
			}
		}
	}
	a.Early = windowStats("early", early)
	a.Late = windowStats("late", late)
	a.LateSteady = windowStats("late_steady", lateSteady)
	a.Overall = windowStats("overall", all)
	return a
}

func windowStats(label string, turns []TurnMetrics) WindowStats {
	w := WindowStats{Label: label, Turns: len(turns)}
	if len(turns) == 0 {
		return w
	}
	var ctx, prompt, gen, wall, tps, pref float64
	var tpsN int
	for _, t := range turns {
		ctx += float64(t.ContextTokens)
		prompt += float64(t.PromptTokens)
		gen += float64(t.GeneratedTokens)
		wall += t.WallClockS
		pref += float64(t.PrefillTokens)
			if t.RewriteFired || t.CompactFired {
				w.Rewrites++
			}
		if t.GenTokensPerSec > 0 {
			tps += t.GenTokensPerSec
			tpsN++
		}
		if t.IsProbe && t.ProbeCorrect != nil {
			w.ProbeN++
			if *t.ProbeCorrect {
				w.ProbeCorrect++
			}
		}
	}
	n := float64(len(turns))
	w.AvgContextTokens = ctx / n
	w.AvgPromptTokens = prompt / n
	w.AvgGeneratedTokens = gen / n
	w.AvgWallClockS = wall / n
	w.AvgPrefillTokens = pref / n
	if tpsN > 0 {
		w.AvgGenTokensPerSec = tps / float64(tpsN)
	}
	if w.ProbeN > 0 {
		w.ProbeAccuracyPct = 100 * float64(w.ProbeCorrect) / float64(w.ProbeN)
	}
	return w
}

func armToSnapshot(a ArmSummary) metrics.Snapshot {
	// Prefer late_steady wall-clock so rewrite spikes do not dominate the
	// generation-performance comparison against baseline.
	w := a.LateSteady
	if w.Turns == 0 {
		w = a.Late
	}
	s := metrics.Snapshot{
		Arm:                a.Arm,
		AccuracyPct:        a.Late.ProbeAccuracyPct,
		N:                  a.Late.ProbeN,
		Correct:            a.Late.ProbeCorrect,
		AvgContextTokens:   a.Late.AvgContextTokens,
		PeakContextTokens:  a.PeakContext,
		FinalContextTokens: a.Late.AvgContextTokens,
		AvgPrefillTokens:   a.TotalPrefill / maxF(float64(a.Sessions), 1),
		AvgPromptTokens:    a.Late.AvgPromptTokens,
		AvgGeneratedTokens: a.Late.AvgGeneratedTokens,
		AvgRewrites:        a.TotalRewrites / maxF(float64(a.Sessions), 1),
		AvgWallClockS:      w.AvgWallClockS,
		TotalWallClockS:    a.TotalWallClock,
	}
	s.FinalizeDerived()
	return s
}

func maxF(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n…[truncated]…"
}

// WriteMarkdownReport writes the long-horizon report.
func WriteMarkdownReport(path string, rep Report) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	_, _ = fmt.Fprintf(f, "# DynCLM Long-Horizon Report\n\n")
	_, _ = fmt.Fprintf(f, "Generated: %s\n\n", rep.GeneratedAt.Format(time.RFC3339))
	_, _ = fmt.Fprintf(f, "## Why this suite exists (methodology re-evaluation)\n\n")
	_, _ = fmt.Fprintf(f, "%s\n\n", rep.Methodology)
	_, _ = fmt.Fprintf(f, "- Model: `%s` · Hardware: `%s` · Live: %v\n", rep.ModelID, rep.HardwareID, rep.Live)
	_, _ = fmt.Fprintf(f, "- Sessions/arm: %d · Turns: %d · Probe every: %d\n", rep.Sessions, rep.MaxTurns, rep.ProbeEvery)
	_, _ = fmt.Fprintf(f, "- Early window: turns 1–%d · Late window: turns %d–%d\n\n", rep.EarlyThrough, rep.LateFrom, rep.MaxTurns)

	_, _ = fmt.Fprintf(f, "## Late-session results (the paper-relevant window)\n\n")
	_, _ = fmt.Fprintf(f, "| Arm | Late probe accuracy | Late avg ctx | Late avg prompt | Late wall-s | Late steady wall-s | Late gen tok/s | Prefill/session | Rewrites/session |\n")
	_, _ = fmt.Fprintf(f, "|---|---|---|---|---|---|---|---|---|\n")
	for _, name := range armOrder(rep.Arms) {
		a := rep.Arms[name]
		_, _ = fmt.Fprintf(f, "| %s | **%.1f%%** (%d/%d) | %.0f | %.0f | %.2f | %.2f | %.2f | %.0f | %.1f |\n",
			name, a.Late.ProbeAccuracyPct, a.Late.ProbeCorrect, a.Late.ProbeN,
			a.Late.AvgContextTokens, a.Late.AvgPromptTokens, a.Late.AvgWallClockS, a.LateSteady.AvgWallClockS, a.LateSteady.AvgGenTokensPerSec,
			a.TotalPrefill/maxF(float64(a.Sessions), 1), a.TotalRewrites/maxF(float64(a.Sessions), 1))
	}

	baseName := "baseline"
	if _, ok := rep.Arms["baseline"]; !ok {
		baseName = "baseline-compact"
	}
	b, d := rep.Arms[baseName], rep.Arms["dynclm"]
	_, _ = fmt.Fprintf(f, "\n## Early vs late (DynCLM vs %s)\n\n", baseName)
	_, _ = fmt.Fprintf(f, "| Window | %s probe%% | DynCLM probe%% | %s ctx | DynCLM ctx | %s wall-s | DynCLM wall-s | %s tok/s | DynCLM tok/s |\n",
		baseName, baseName, baseName, baseName)
	_, _ = fmt.Fprintf(f, "|---|---|---|---|---|---|---|---|---|\n")
	_, _ = fmt.Fprintf(f, "| early | %.1f | %.1f | %.0f | %.0f | %.2f | %.2f | %.2f | %.2f |\n",
		b.Early.ProbeAccuracyPct, d.Early.ProbeAccuracyPct, b.Early.AvgContextTokens, d.Early.AvgContextTokens,
		b.Early.AvgWallClockS, d.Early.AvgWallClockS, b.Early.AvgGenTokensPerSec, d.Early.AvgGenTokensPerSec)
	_, _ = fmt.Fprintf(f, "| late | %.1f | %.1f | %.0f | %.0f | %.2f | %.2f | %.2f | %.2f |\n",
		b.Late.ProbeAccuracyPct, d.Late.ProbeAccuracyPct, b.Late.AvgContextTokens, d.Late.AvgContextTokens,
		b.Late.AvgWallClockS, d.Late.AvgWallClockS, b.Late.AvgGenTokensPerSec, d.Late.AvgGenTokensPerSec)

	_, _ = fmt.Fprintf(f, "\n## DynCLM vs %s (late-window metrics)\n\n", baseName)
	_, _ = fmt.Fprintf(f, "| Metric | %s | DynCLM | Δ | %% change | Better? |\n|---|---|---|---|---|---|\n", baseName)
	for _, row := range rep.VsBaseline["dynclm"] {
		better := "—"
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

	_, _ = fmt.Fprintf(f, "\n## Headlines\n\n")
	_, _ = fmt.Fprintf(f, "- **Late quality vs %s:** %+.1f pp (%.1f%% → %.1f%%)\n",
		baseName, d.Late.ProbeAccuracyPct-b.Late.ProbeAccuracyPct, b.Late.ProbeAccuracyPct, d.Late.ProbeAccuracyPct)
	_, _ = fmt.Fprintf(f, "- **Late context vs %s:** %+.1f%% (%.0f → %.0f)\n",
		baseName, pct(d.Late.AvgContextTokens, b.Late.AvgContextTokens), b.Late.AvgContextTokens, d.Late.AvgContextTokens)
	_, _ = fmt.Fprintf(f, "- **Late wall-clock vs %s (all late turns):** %+.1f%% (%.2fs → %.2fs)\n",
		baseName, pct(d.Late.AvgWallClockS, b.Late.AvgWallClockS), b.Late.AvgWallClockS, d.Late.AvgWallClockS)
	_, _ = fmt.Fprintf(f, "- **Late steady wall-clock vs %s (exclude rewrite/compact turns):** %+.1f%% (%.2fs → %.2fs)\n",
		baseName, pct(d.LateSteady.AvgWallClockS, b.LateSteady.AvgWallClockS), b.LateSteady.AvgWallClockS, d.LateSteady.AvgWallClockS)
	_, _ = fmt.Fprintf(f, "- **Late steady gen tok/s vs %s:** %+.1f%% (%.2f → %.2f)\n",
		baseName, pct(d.LateSteady.AvgGenTokensPerSec, b.LateSteady.AvgGenTokensPerSec), b.LateSteady.AvgGenTokensPerSec, d.LateSteady.AvgGenTokensPerSec)
	if imm, ok := rep.Arms["immediate-rewrite"]; ok {
		_, _ = fmt.Fprintf(f, "- **Immediate-rewrite (CLM) late steady wall vs %s:** %+.1f%% (%.2fs → %.2fs) — upper bound if N is high\n",
			baseName, pct(imm.LateSteady.AvgWallClockS, b.LateSteady.AvgWallClockS),
			b.LateSteady.AvgWallClockS, imm.LateSteady.AvgWallClockS)
	}
	_, _ = fmt.Fprintf(f, "\nNote: gen tok/s uses completion_tokens/wall_clock (includes prefill). ")
	_, _ = fmt.Fprintf(f, "Steady-state excludes rewrite/compact-event turns so generation-under-context is not mixed with reclaim spikes.\n")
	return nil
}

func armOrder(arms map[string]ArmSummary) []string {
	prefer := []string{"baseline", "baseline-compact", "immediate-rewrite", "dynclm"}
	var out []string
	seen := map[string]bool{}
	for _, n := range prefer {
		if _, ok := arms[n]; ok {
			out = append(out, n)
			seen[n] = true
		}
	}
	for n := range arms {
		if !seen[n] {
			out = append(out, n)
		}
	}
	return out
}

// WriteMatrixMarkdownReport writes the compaction-matrix report (suite B).
func WriteMatrixMarkdownReport(path string, m MatrixReport) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	_, _ = fmt.Fprintf(f, "# DynCLM Long-Horizon B — Compaction Matrix\n\n")
	_, _ = fmt.Fprintf(f, "Generated: %s\n\n", m.GeneratedAt.Format(time.RFC3339))
	_, _ = fmt.Fprintf(f, "%s\n\n", m.Methodology)
	_, _ = fmt.Fprintf(f, "- Model: `%s` · Hardware: `%s` · Live: %v\n", m.ModelID, m.HardwareID, m.Live)
	_, _ = fmt.Fprintf(f, "- Sessions/arm: %d · Turns: %d · NoisePad: %d chars\n", m.Sessions, m.MaxTurns, m.NoisePad)
	_, _ = fmt.Fprintf(f, "- Max context limits: %v\n\n", m.MaxContexts)

	_, _ = fmt.Fprintf(f, "## Late probe accuracy (%%) by max context\n\n")
	_, _ = fmt.Fprintf(f, "| Max context | baseline-compact | DynCLM | CLM (immediate) |\n|---|---|---|---|\n")
	for _, mc := range m.MaxContexts {
		key := fmt.Sprintf("%d", mc)
		arms := m.ByMaxContext[key]
		_, _ = fmt.Fprintf(f, "| %d | %.1f | %.1f | %.1f |\n", mc,
			num(arms, "baseline-compact", "late_probe_accuracy"),
			num(arms, "dynclm", "late_probe_accuracy"),
			num(arms, "immediate-rewrite", "late_probe_accuracy"))
	}

	_, _ = fmt.Fprintf(f, "\n## Late avg context tokens by max context\n\n")
	_, _ = fmt.Fprintf(f, "| Max context | baseline-compact | DynCLM | CLM (immediate) |\n|---|---|---|---|\n")
	for _, mc := range m.MaxContexts {
		key := fmt.Sprintf("%d", mc)
		arms := m.ByMaxContext[key]
		_, _ = fmt.Fprintf(f, "| %d | %.0f | %.0f | %.0f |\n", mc,
			num(arms, "baseline-compact", "late_avg_context"),
			num(arms, "dynclm", "late_avg_context"),
			num(arms, "immediate-rewrite", "late_avg_context"))
	}

	_, _ = fmt.Fprintf(f, "\n## Late steady wall-clock (s) by max context\n\n")
	_, _ = fmt.Fprintf(f, "| Max context | baseline-compact | DynCLM | CLM (immediate) |\n|---|---|---|---|\n")
	for _, mc := range m.MaxContexts {
		key := fmt.Sprintf("%d", mc)
		arms := m.ByMaxContext[key]
		_, _ = fmt.Fprintf(f, "| %d | %.2f | %.2f | %.2f |\n", mc,
			num(arms, "baseline-compact", "late_steady_wall_s"),
			num(arms, "dynclm", "late_steady_wall_s"),
			num(arms, "immediate-rewrite", "late_steady_wall_s"))
	}

	_, _ = fmt.Fprintf(f, "\n## Late steady gen tok/s by max context\n\n")
	_, _ = fmt.Fprintf(f, "| Max context | baseline-compact | DynCLM | CLM (immediate) |\n|---|---|---|---|\n")
	for _, mc := range m.MaxContexts {
		key := fmt.Sprintf("%d", mc)
		arms := m.ByMaxContext[key]
		_, _ = fmt.Fprintf(f, "| %d | %.2f | %.2f | %.2f |\n", mc,
			num(arms, "baseline-compact", "late_steady_tok_s"),
			num(arms, "dynclm", "late_steady_tok_s"),
			num(arms, "immediate-rewrite", "late_steady_tok_s"))
	}

	_, _ = fmt.Fprintf(f, "\n## Figures\n\n")
	_, _ = fmt.Fprintf(f, "- ![probe](figures/compaction_probe_vs_limit.svg)\n")
	_, _ = fmt.Fprintf(f, "- ![context](figures/compaction_context_vs_limit.svg)\n")
	_, _ = fmt.Fprintf(f, "- ![wall](figures/compaction_wall_vs_limit.svg)\n")
	_, _ = fmt.Fprintf(f, "- ![tok/s](figures/compaction_toks_vs_limit.svg)\n")
	return nil
}

func num(arms map[string]any, arm, key string) float64 {
	if arms == nil {
		return 0
	}
	m, ok := arms[arm]
	if !ok {
		return 0
	}
	mm, ok := m.(map[string]any)
	if !ok {
		return 0
	}
	v, ok := mm[key]
	if !ok {
		return 0
	}
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	default:
		return 0
	}
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
