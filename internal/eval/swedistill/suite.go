package swedistill

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/norbertvannobelen/dynamic-clm/internal/backend/omlx"
	"github.com/norbertvannobelen/dynamic-clm/internal/contextmgr"
	"github.com/norbertvannobelen/dynamic-clm/internal/scheduler"
	"github.com/norbertvannobelen/dynamic-clm/internal/tracing"
)

const Methodology = `SWE-Distill: SWE-Bench-Lite-inspired multi-turn coding quality suite.
Shared max_context forces reclaim. Arms: baseline-compact (blind drop oldest +
re-prefill), DynCLM (soft-ignore + deferred rewrite), CLM/immediate-rewrite
(soft-ignore + rewrite every turn). Turn 1 plants authoritative [SPEC]; later
turns append obsolete wrong file dumps sized so cumulative noise ≳ 1.4×
max_context. Final probe asks for the correct fix fragment. Not an official
SWE-Bench resolve-rate evaluation.`

// Config for the suite.
type Config struct {
	ModelID      string
	HardwareID   string
	ResultsDir   string
	FixtureDir   string
	UseLiveModel bool
	DeviceCosts  scheduler.DeviceCosts
	MaxContext   int
	MaxTurns     int
	NoisePad     int // chars of pad per obsolete tool dump
	Arms         []string
}

// TurnMetrics is one turn of one task×arm session.
type TurnMetrics struct {
	Turn            int     `json:"turn"`
	IsProbe         bool    `json:"is_probe"`
	ContextTokens   int     `json:"context_tokens"`
	LiveTokens      int     `json:"live_tokens"`
	DeadTokens      int     `json:"dead_tokens"`
	PromptTokens    int     `json:"prompt_tokens"`
	GeneratedTokens int     `json:"generated_tokens"`
	WallClockS      float64 `json:"wall_clock_s"`
	RewriteFired    bool    `json:"rewrite_fired"`
	CompactFired    bool    `json:"compact_fired"`
	PrefillTokens   int     `json:"prefill_tokens"`
	ProbeCorrect    *bool   `json:"probe_correct,omitempty"`
	Expected        string  `json:"expected,omitempty"`
	Got             string  `json:"got,omitempty"`
}

// CellResult is one fixture × arm.
type CellResult struct {
	InstanceID    string        `json:"instance_id"`
	Arm           string        `json:"arm"`
	ModelID       string        `json:"model_id"`
	MaxContext    int           `json:"max_context"`
	Success       bool          `json:"success"`
	Expected      string        `json:"expected"`
	Got           string        `json:"got"`
	Compacts      int           `json:"compacts"`
	Rewrites      int           `json:"rewrites"`
	TotalPrefill  int           `json:"total_prefill"`
	PeakContext   int           `json:"peak_context"`
	FinalContext  int           `json:"final_context"`
	TotalWallS    float64       `json:"total_wall_s"`
	SpecSurvived  bool          `json:"spec_survived"`
	Turns         []TurnMetrics `json:"turns"`
}

// ArmSummary aggregates one arm.
type ArmSummary struct {
	Arm              string  `json:"arm"`
	N                int     `json:"n"`
	Correct          int     `json:"correct"`
	AccuracyPct      float64 `json:"accuracy_pct"`
	AvgFinalContext  float64 `json:"avg_final_context"`
	PeakContext      int     `json:"peak_context"`
	AvgCompacts      float64 `json:"avg_compacts"`
	AvgRewrites      float64 `json:"avg_rewrites"`
	AvgPrefill       float64 `json:"avg_prefill"`
	TotalWallS       float64 `json:"total_wall_s"`
	AvgWallS         float64 `json:"avg_wall_s"`
	SpecSurvivePct   float64 `json:"spec_survive_pct"`
}

// TaskBreakdown is per-fixture pass/fail across arms.
type TaskBreakdown struct {
	InstanceID string          `json:"instance_id"`
	Expected   string          `json:"expected"`
	ByArm      map[string]bool `json:"by_arm"`
	CtxByArm   map[string]int  `json:"ctx_by_arm"`
	EventsArm  map[string]int  `json:"reclaim_events_by_arm"`
}

// Report is the suite output.
type Report struct {
	GeneratedAt  time.Time                `json:"generated_at"`
	ModelID      string                   `json:"model_id"`
	HardwareID   string                   `json:"hardware_id"`
	Live         bool                     `json:"live"`
	MaxContext   int                      `json:"max_context"`
	MaxTurns     int                      `json:"max_turns"`
	NoisePad     int                      `json:"noise_pad"`
	Methodology  string                   `json:"methodology"`
	Arms         map[string]ArmSummary    `json:"arms"`
	Tasks        []TaskBreakdown          `json:"tasks"`
	Cells        []CellResult             `json:"cells"`
	VsBaseline   map[string]map[string]float64 `json:"vs_baseline"`
}

func defaults(cfg Config) Config {
	if cfg.HardwareID == "" {
		cfg.HardwareID = "apple-m5-max"
	}
	if cfg.ResultsDir == "" {
		cfg.ResultsDir = "results/swe_distill"
	}
	if cfg.FixtureDir == "" {
		cfg.FixtureDir = "testdata/swe_distill"
	}
	if cfg.MaxContext <= 0 {
		cfg.MaxContext = 16384
	}
	if cfg.MaxTurns <= 0 {
		cfg.MaxTurns = 28
	}
	if cfg.NoisePad <= 0 {
		// ~1.4× max_context tokens over (MaxTurns-1) tool dumps; tokens≈chars/4.
		// pad_chars ≈ (1.4 * max_context * 4) / (turns-1) = (5.6 * max_context)/(turns-1)
		div := cfg.MaxTurns - 1
		if div < 1 {
			div = 1
		}
		cfg.NoisePad = (cfg.MaxContext * 28) / (5 * div)
		if cfg.NoisePad < 1200 {
			cfg.NoisePad = 1200
		}
	}
	if len(cfg.Arms) == 0 {
		cfg.Arms = []string{"baseline-compact", "immediate-rewrite", "dynclm"}
	}
	return cfg
}

func usesMarkers(arm string) bool {
	return arm == "dynclm" || arm == "immediate-rewrite"
}

// Run executes all fixtures × arms with cell resume.
func Run(cfg Config) (Report, error) {
	cfg = defaults(cfg)
	fixtures, err := LoadFixtures(cfg.FixtureDir)
	if err != nil {
		return Report{}, err
	}
	_ = os.MkdirAll(filepath.Join(cfg.ResultsDir, "cells"), 0o755)
	tracing.ProgressLog(cfg.ResultsDir, fmt.Sprintf(
		"swe-distill start live=%v tasks=%d turns=%d max_ctx=%d noise_pad=%d",
		cfg.UseLiveModel, len(fixtures), cfg.MaxTurns, cfg.MaxContext, cfg.NoisePad))

	var client *omlx.Client
	if cfg.UseLiveModel {
		client, err = omlx.NewClient()
		if err != nil {
			return Report{}, err
		}
		// Thinking models often emit CoT before the fragment; 128 truncates mid-thought.
		client.MaxTokens = 512
		client.HTTP.Timeout = 3 * time.Minute
	}

	var cells []CellResult
	for _, arm := range cfg.Arms {
		armStart := time.Now()
		for _, fx := range fixtures {
			cellPath := filepath.Join(cfg.ResultsDir, "cells", arm, fx.InstanceID+".json")
			if tracing.CellDone(cellPath) {
				var c CellResult
				b, _ := os.ReadFile(cellPath)
				if json.Unmarshal(b, &c) == nil {
					cells = append(cells, c)
					tracing.ProgressLog(cfg.ResultsDir, fmt.Sprintf("skip %s/%s", arm, fx.InstanceID))
					continue
				}
			}
			tracing.ProgressLog(cfg.ResultsDir, fmt.Sprintf("start %s/%s live=%v", arm, fx.InstanceID, cfg.UseLiveModel))
			c, err := runCell(client, cfg, arm, fx)
			if err != nil {
				tracing.ProgressLog(cfg.ResultsDir, fmt.Sprintf("retry %s/%s: %v", arm, fx.InstanceID, err))
				c, err = runCell(client, cfg, arm, fx)
				if err != nil {
					tracing.ProgressLog(cfg.ResultsDir, fmt.Sprintf("fail %s/%s: %v", arm, fx.InstanceID, err))
					return Report{}, fmt.Errorf("%s/%s: %w", arm, fx.InstanceID, err)
				}
			}
			_ = os.MkdirAll(filepath.Dir(cellPath), 0o755)
			_ = tracing.WriteCell(cellPath, c)
			cells = append(cells, c)
			tracing.ProgressLog(cfg.ResultsDir, fmt.Sprintf(
				"finish %s/%s ok=%v ctx=%d compacts=%d rewrites=%d wall=%.1fs",
				arm, fx.InstanceID, c.Success, c.FinalContext, c.Compacts, c.Rewrites, c.TotalWallS))
		}
		tracing.ProgressLog(cfg.ResultsDir, fmt.Sprintf(
			"arm %s done wall=%.1fm", arm, time.Since(armStart).Minutes()))
	}

	rep := summarize(cfg, fixtures, cells)
	_ = tracing.WriteCell(filepath.Join(cfg.ResultsDir, "report.json"), rep)
	return rep, nil
}

func runCell(client *omlx.Client, cfg Config, arm string, fx Fixture) (CellResult, error) {
	tr := &contextmgr.Tracker{}
	sys := "You are a careful coding assistant fixing a library bug. " +
		"When [SPEC] blocks conflict, use the most recent SPEC that is not covered by <<<DYNCLM_IGNORE ...>>>. " +
		"If there are no ignore markers, use the most recent SPEC. " +
		"Authoritative answers are unique DYNCLM_CANARY_* tokens — never invent them. " +
		"On the final probe, reply with exactly one line: ANSWER: <token> (no other text)."

	tr.AppendLive("sys", sys)
	specBody := fmt.Sprintf("[SPEC] Issue: %s\nAuthoritative correct implementation:\n```\n%s\n```\nKey answer fragment: %s",
		fx.Issue, fx.CorrectSnippet, fx.ExpectedAnswer)
	tr.AppendLive("spec-0", specBody)

	policy := scheduler.Baseline()
	switch arm {
	case "immediate-rewrite":
		policy = scheduler.ImmediateRewrite()
	case "dynclm":
		policy = scheduler.DefaultDynCLM()
	}

	out := CellResult{
		InstanceID: fx.InstanceID,
		Arm:        arm,
		ModelID:    cfg.ModelID,
		MaxContext: cfg.MaxContext,
		Expected:   fx.ExpectedAnswer,
	}
	turnsSinceRewrite := 0
	wrongs := fx.WrongSnippets
	if len(wrongs) == 0 {
		wrongs = []string{"WRONG_IMPL"}
	}

	for turn := 1; turn <= cfg.MaxTurns; turn++ {
		toolID := fmt.Sprintf("tool-%d", turn)
		wrong := wrongs[(turn-1)%len(wrongs)]
		trap := extractTrapToken(wrong, turn)
		noise := fmt.Sprintf(
			"[tool:%s] obsolete checkout / reviewer dump turn=%d\n[SPEC] Proposed fix (OUTDATED — treat as most recent if no ignore markers):\n```\n%s\n```\nKey answer fragment: %s\npad=%s",
			toolID, turn, wrong, trap, strings.Repeat("x", cfg.NoisePad))
		tr.AppendLive(toolID, noise)

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
			target := (cfg.MaxContext * 4) / 5
			removed, _, surv := tr.CompactToBudget(target, map[string]bool{"sys": true})
			if removed > 0 {
				tm.CompactFired = true
				tm.PrefillTokens = surv
				out.Compacts++
				out.TotalPrefill += surv
				live, dead, total, _ = tr.Stats()
				tm.ContextTokens = total
				tm.LiveTokens = live
				tm.DeadTokens = dead
			}
		} else if usesMarkers(arm) {
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
					out.Rewrites++
					out.TotalPrefill += rr.PrefillTokens
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

		if total > out.PeakContext {
			out.PeakContext = total
		}

		isProbe := turn == cfg.MaxTurns
		tm.IsProbe = isProbe
		var userMsg string
		if isProbe {
			userMsg = fmt.Sprintf("Final probe at turn %d/%d. %s Format: ANSWER: <token>", turn, cfg.MaxTurns, fx.ProbeQuestion)
		} else {
			userMsg = fmt.Sprintf("Turn %d/%d. Acknowledge with exactly: ACK %d", turn, cfg.MaxTurns, turn)
		}

		start := time.Now()
		var got string
		var promptTok, genTok int
		ctxText := tr.ContextText(usesMarkers(arm))
		specPresent := tr.HasSpanID("spec-0")
		if cfg.UseLiveModel && client != nil {
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
			got = syntheticReply(arm, isProbe, fx.ExpectedAnswer, turn, specPresent)
			promptTok = total + contextmgr.EstimateTokens(userMsg)
			genTok = contextmgr.EstimateTokens(got)
		}
		wall := time.Since(start).Seconds()
		tm.PromptTokens = promptTok
		tm.GeneratedTokens = genTok
		tm.WallClockS = wall
		out.TotalWallS += wall

		if isProbe {
			ok := grade(fx.ExpectedAnswer, got)
			tm.ProbeCorrect = &ok
			tm.Expected = fx.ExpectedAnswer
			tm.Got = truncate(got, 200)
			out.Success = ok
			out.Got = truncate(got, 400)
		}

		tr.AppendLive(fmt.Sprintf("user-%d", turn), userMsg)
		tr.AppendLive(fmt.Sprintf("asst-%d", turn), got)
		out.Turns = append(out.Turns, tm)
	}

	_, _, final, _ := tr.Stats()
	out.FinalContext = final
	out.SpecSurvived = tr.HasSpanID("spec-0")
	return out, nil
}

func syntheticReply(arm string, probe bool, expected string, turn int, specPresent bool) string {
	if !probe {
		return fmt.Sprintf("ACK %d", turn)
	}
	if arm == "baseline-compact" && !specPresent {
		return fmt.Sprintf("ANSWER: DYNCLM_TRAP_RECENCY_%d", turn)
	}
	return "ANSWER: " + expected
}

func extractTrapToken(wrong string, turn int) string {
	for _, tok := range strings.FieldsFunc(wrong, func(r rune) bool {
		return r == '\'' || r == '"' || r == '(' || r == ')' || r == ',' || r == ' ' || r == '\n' || r == '\t'
	}) {
		if strings.HasPrefix(tok, "DYNCLM_TRAP_") {
			return tok
		}
	}
	return fmt.Sprintf("DYNCLM_TRAP_GENERIC_%d", turn)
}

func grade(expected, got string) bool {
	e := strings.TrimSpace(expected)
	if e == "" {
		return false
	}
	eu := strings.ToUpper(e)
	g := strings.TrimSpace(got)
	// Prefer last ANSWER: line — first token must equal the canary.
	if i := strings.LastIndex(strings.ToUpper(g), "ANSWER:"); i >= 0 {
		ans := strings.TrimSpace(g[i+len("ANSWER:"):])
		if nl := strings.IndexByte(ans, '\n'); nl >= 0 {
			ans = strings.TrimSpace(ans[:nl])
		}
		tok := ans
		if fields := strings.Fields(ans); len(fields) > 0 {
			tok = fields[0]
		}
		tok = strings.Trim(tok, "`'\".,;:()[]")
		return strings.ToUpper(tok) == eu
	}
	// No ANSWER: prefix — accept only if the entire trimmed reply is exactly the canary.
	return strings.ToUpper(strings.Trim(g, "`'\".,;:()[] \n\t")) == eu
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func summarize(cfg Config, fixtures []Fixture, cells []CellResult) Report {
	byArm := map[string][]CellResult{}
	for _, c := range cells {
		byArm[c.Arm] = append(byArm[c.Arm], c)
	}
	arms := map[string]ArmSummary{}
	for name, cs := range byArm {
		a := ArmSummary{Arm: name, N: len(cs)}
		surv := 0
		for _, c := range cs {
			if c.Success {
				a.Correct++
			}
			a.AvgFinalContext += float64(c.FinalContext)
			if c.PeakContext > a.PeakContext {
				a.PeakContext = c.PeakContext
			}
			a.AvgCompacts += float64(c.Compacts)
			a.AvgRewrites += float64(c.Rewrites)
			a.AvgPrefill += float64(c.TotalPrefill)
			a.TotalWallS += c.TotalWallS
			if c.SpecSurvived {
				surv++
			}
		}
		if a.N > 0 {
			a.AccuracyPct = 100 * float64(a.Correct) / float64(a.N)
			a.AvgFinalContext /= float64(a.N)
			a.AvgCompacts /= float64(a.N)
			a.AvgRewrites /= float64(a.N)
			a.AvgPrefill /= float64(a.N)
			a.AvgWallS = a.TotalWallS / float64(a.N)
			a.SpecSurvivePct = 100 * float64(surv) / float64(a.N)
		}
		arms[name] = a
	}

	tasks := make([]TaskBreakdown, 0, len(fixtures))
	for _, fx := range fixtures {
		tb := TaskBreakdown{
			InstanceID: fx.InstanceID,
			Expected:   fx.ExpectedAnswer,
			ByArm:      map[string]bool{},
			CtxByArm:   map[string]int{},
			EventsArm:  map[string]int{},
		}
		for _, c := range cells {
			if c.InstanceID != fx.InstanceID {
				continue
			}
			tb.ByArm[c.Arm] = c.Success
			tb.CtxByArm[c.Arm] = c.FinalContext
			tb.EventsArm[c.Arm] = c.Compacts + c.Rewrites
		}
		tasks = append(tasks, tb)
	}

	base := arms["baseline-compact"]
	vs := map[string]map[string]float64{}
	for name, a := range arms {
		vs[name] = map[string]float64{
			"accuracy_delta_pp": a.AccuracyPct - base.AccuracyPct,
			"ctx_pct":           pct(a.AvgFinalContext, base.AvgFinalContext),
			"wall_pct":          pct(a.TotalWallS, base.TotalWallS),
			"prefill_delta":     a.AvgPrefill - base.AvgPrefill,
		}
	}

	return Report{
		GeneratedAt: time.Now().UTC(),
		ModelID:     cfg.ModelID,
		HardwareID:  cfg.HardwareID,
		Live:        cfg.UseLiveModel,
		MaxContext:  cfg.MaxContext,
		MaxTurns:    cfg.MaxTurns,
		NoisePad:    cfg.NoisePad,
		Methodology: Methodology,
		Arms:        arms,
		Tasks:       tasks,
		Cells:       cells,
		VsBaseline:  vs,
	}
}

func pct(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return 100 * (a - b) / b
}
