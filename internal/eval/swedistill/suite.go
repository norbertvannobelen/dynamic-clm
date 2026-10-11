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

const Methodology = `SWE-Distill (realistic): SWE-Bench-Lite-inspired multi-turn coding quality.
Shared max_context forces reclaim. Arms: baseline-compact (blind drop oldest +
re-prefill), DynCLM (soft-ignore + deferred rewrite), CLM/immediate-rewrite
(soft-ignore + rewrite every turn). Turn 1 plants an authoritative [SPEC] with a
real correct fix; later turns append plausible wrong SPECs (recency trap) with
noise ≳ 1.4× max_context. Instruction: prefer the most recent non-ignored SPEC.
Baseline often follows the wrong recent SPEC after compaction; DynCLM/CLM should
retain the early correct SPEC. Final probe grades ANSWER: <fragment>. Not an
official SWE-Bench resolve-rate evaluation.`

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
	OnlyInstance string // if set, run only fixtures whose instance_id contains this substring
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
	InstanceID        string        `json:"instance_id"`
	Arm               string        `json:"arm"`
	ModelID           string        `json:"model_id"`
	MaxContext        int           `json:"max_context"`
	Success           bool          `json:"success"`
	Expected          string        `json:"expected"`
	Got               string        `json:"got"`
	Compacts          int           `json:"compacts"`
	Rewrites          int           `json:"rewrites"`
	TotalPrefill      int           `json:"total_prefill"`
	PeakContext       int           `json:"peak_context"`        // peak live+dead tracker footprint
	PeakLiveContext   int           `json:"peak_live_context"`   // peak live-only (decode-relevant)
	FinalContext      int           `json:"final_context"`       // live tokens (headline); not live+dead
	FinalTotalContext int           `json:"final_total_context"` // live+dead tracker footprint
	FinalDeadContext  int           `json:"final_dead_context"`
	FinalPromptTokens int           `json:"final_prompt_tokens"` // API prompt_tokens on probe turn
	TotalWallS        float64       `json:"total_wall_s"`
	SteadyWallS       float64       `json:"steady_wall_s"` // wall excluding rewrite/compact turns
	SpecSurvived      bool          `json:"spec_survived"`
	Turns             []TurnMetrics `json:"turns"`
}

// ArmSummary aggregates one arm.
type ArmSummary struct {
	Arm              string  `json:"arm"`
	N                int     `json:"n"`
	Correct          int     `json:"correct"`
	AccuracyPct      float64 `json:"accuracy_pct"`
	AvgFinalContext  float64 `json:"avg_final_context"`  // live-only
	AvgFinalTotal    float64 `json:"avg_final_total"`    // live+dead
	AvgFinalPrompt   float64 `json:"avg_final_prompt"`   // probe API prompt_tokens
	PeakContext      int     `json:"peak_context"`       // peak live (headline)
	PeakTotalContext int     `json:"peak_total_context"` // peak live+dead
	AvgCompacts      float64 `json:"avg_compacts"`
	AvgRewrites      float64 `json:"avg_rewrites"`
	AvgPrefill       float64 `json:"avg_prefill"`
	TotalWallS       float64 `json:"total_wall_s"`
	AvgWallS         float64 `json:"avg_wall_s"`
	AvgSteadyWallS   float64 `json:"avg_steady_wall_s"` // excludes reclaim-event turns
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
	GeneratedAt time.Time                     `json:"generated_at"`
	ModelID     string                        `json:"model_id"`
	HardwareID  string                        `json:"hardware_id"`
	Live        bool                          `json:"live"`
	MaxContext  int                           `json:"max_context"`
	MaxTurns    int                           `json:"max_turns"`
	NoisePad    int                           `json:"noise_pad"`
	Methodology string                        `json:"methodology"`
	Arms        map[string]ArmSummary         `json:"arms"`
	Tasks       []TaskBreakdown               `json:"tasks"`
	Cells       []CellResult                  `json:"cells"`
	VsBaseline  map[string]map[string]float64 `json:"vs_baseline"`
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
	if cfg.OnlyInstance != "" {
		var filtered []Fixture
		needle := strings.ToLower(cfg.OnlyInstance)
		for _, fx := range fixtures {
			if strings.Contains(strings.ToLower(fx.InstanceID), needle) {
				filtered = append(filtered, fx)
			}
		}
		if len(filtered) == 0 {
			return Report{}, fmt.Errorf("no fixtures match -only %q", cfg.OnlyInstance)
		}
		fixtures = filtered
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
		// Non-probe turns stay short; probes bump MaxTokens below so CoT cannot
		// exhaust the completion budget before ANSWER: (quality ≠ output cap).
		client.MaxTokens = 256
		client.HTTP.Timeout = 3 * time.Minute
	}

	var cells []CellResult
	// Round-robin by task across arms so intermediate quality can be compared
	// after each fixture completes for Baseline+compact / DynCLM / CLM.
	for ti, fx := range fixtures {
		taskStart := time.Now()
		tracing.ProgressLog(cfg.ResultsDir, fmt.Sprintf(
			"task %d/%d begin %s", ti+1, len(fixtures), fx.InstanceID))
		for _, arm := range cfg.Arms {
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
		// Intermediate quality snapshot after this task × all arms.
		snap := summarize(cfg, fixtures[:ti+1], cellsForInstances(cells, fixtures[:ti+1]))
		_ = tracing.WriteCell(filepath.Join(cfg.ResultsDir, fmt.Sprintf("interim_task_%02d.json", ti+1)), snap)
		tracing.ProgressLog(cfg.ResultsDir, fmt.Sprintf(
			"task %d/%d done %s wall=%.1fm interim_acc baseline=%.0f%% dynclm=%.0f%% clm=%.0f%%",
			ti+1, len(fixtures), fx.InstanceID, time.Since(taskStart).Minutes(),
			snap.Arms["baseline-compact"].AccuracyPct,
			snap.Arms["dynclm"].AccuracyPct,
			snap.Arms["immediate-rewrite"].AccuracyPct,
		))
	}

	// If this was a partial (-only) run, rebuild the report from all cells on disk
	// so we do not overwrite the full suite report with a 1-task snapshot.
	reportFixtures := fixtures
	reportCells := cells
	if cfg.OnlyInstance != "" {
		if all, err := LoadFixtures(cfg.FixtureDir); err == nil {
			reportFixtures = all
		}
		reportCells = loadAllCells(cfg.ResultsDir, cfg.Arms)
	}
	rep := summarize(cfg, reportFixtures, reportCells)
	_ = tracing.WriteCell(filepath.Join(cfg.ResultsDir, "report.json"), rep)
	return rep, nil
}

func loadAllCells(resultsDir string, arms []string) []CellResult {
	if len(arms) == 0 {
		arms = []string{"baseline-compact", "immediate-rewrite", "dynclm"}
	}
	var out []CellResult
	for _, arm := range arms {
		dir := filepath.Join(resultsDir, "cells", arm)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			b, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}
			var c CellResult
			if json.Unmarshal(b, &c) == nil {
				out = append(out, c)
			}
		}
	}
	return out
}

func cellsForInstances(cells []CellResult, fixtures []Fixture) []CellResult {
	want := map[string]bool{}
	for _, fx := range fixtures {
		want[fx.InstanceID] = true
	}
	var out []CellResult
	for _, c := range cells {
		if want[c.InstanceID] {
			out = append(out, c)
		}
	}
	return out
}

func runCell(client *omlx.Client, cfg Config, arm string, fx Fixture) (CellResult, error) {
	tr := &contextmgr.Tracker{}
	sys := "You are a careful coding assistant fixing a library bug from SPEC notes in context. " +
		"When [SPEC] blocks conflict, use the most recent SPEC that is not covered by <<<DYNCLM_IGNORE ...>>>. " +
		"If there are no ignore markers, use the most recent SPEC (including reviewer dumps). " +
		"On the final probe, reply with exactly one line: ANSWER: <fragment> (no other text)."

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
		wrongKey := fx.WrongAnswer
		if wrongKey == "" {
			wrongKey = "outdated_fix"
		}
		noise := fmt.Sprintf(
			"[tool:%s] reviewer dump turn=%d\n[SPEC] Updated recommendation (most recent if unmarked):\n```\n%s\n```\nKey answer fragment: %s\npad=%s",
			toolID, turn, wrong, wrongKey, strings.Repeat("x", cfg.NoisePad))
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
		if live > out.PeakLiveContext {
			out.PeakLiveContext = live
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
		// Marker arms keep dead span text in the prompt (soft-ignore markers); only
		// Rewrite() physically drops them. oMLX Chat has no KV soft-ignore, so
		// prompt_tokens tracks billed/prefill size — often ≫ live_tokens for DynCLM.
		ctxText := tr.ContextText(usesMarkers(arm))
		specPresent := tr.HasSpanID("spec-0")
		if cfg.UseLiveModel && client != nil {
			// Probe replies need headroom for CoT; quality should not fail on MaxTokens.
			if isProbe {
				client.MaxTokens = 2048
			} else {
				client.MaxTokens = 256
			}
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
			got = syntheticReply(arm, isProbe, fx.ExpectedAnswer, fx.WrongAnswer, turn, specPresent)
			// Match live ContextText(includeDead): marker arms still serialize dead spans.
			promptTok = total + contextmgr.EstimateTokens(userMsg)
			genTok = contextmgr.EstimateTokens(got)
		}
		wall := time.Since(start).Seconds()
		tm.PromptTokens = promptTok
		tm.GeneratedTokens = genTok
		tm.WallClockS = wall
		out.TotalWallS += wall
		if !tm.RewriteFired && !tm.CompactFired {
			out.SteadyWallS += wall
		}

		if isProbe {
			ok := grade(fx.ExpectedAnswer, got)
			tm.ProbeCorrect = &ok
			tm.Expected = fx.ExpectedAnswer
			tm.Got = truncate(got, 200)
			out.Success = ok
			out.Got = truncate(got, 400)
			out.FinalPromptTokens = promptTok
		}

		tr.AppendLive(fmt.Sprintf("user-%d", turn), userMsg)
		tr.AppendLive(fmt.Sprintf("asst-%d", turn), got)
		out.Turns = append(out.Turns, tm)
	}

	live, dead, total, _ := tr.Stats()
	// Headline final_context is live-only. Tracker footprint (live+dead) is final_total_context.
	out.FinalContext = live
	out.FinalTotalContext = total
	out.FinalDeadContext = dead
	out.SpecSurvived = tr.HasSpanID("spec-0")
	return out, nil
}

// cellFinalLive returns decode-relevant final context. Legacy cells stored
// live+dead in final_context; prefer last-turn live_tokens when total fields are absent.
func cellFinalLive(c CellResult) int {
	if c.FinalTotalContext > 0 || c.FinalDeadContext > 0 {
		return c.FinalContext
	}
	if n := len(c.Turns); n > 0 && c.Turns[n-1].LiveTokens > 0 {
		return c.Turns[n-1].LiveTokens
	}
	return c.FinalContext
}

func cellFinalTotal(c CellResult) int {
	if c.FinalTotalContext > 0 {
		return c.FinalTotalContext
	}
	return c.FinalContext
}

func cellFinalPrompt(c CellResult) int {
	if c.FinalPromptTokens > 0 {
		return c.FinalPromptTokens
	}
	if n := len(c.Turns); n > 0 {
		return c.Turns[n-1].PromptTokens
	}
	return 0
}

func cellPeakLive(c CellResult) int {
	if c.PeakLiveContext > 0 {
		return c.PeakLiveContext
	}
	peak := 0
	for _, t := range c.Turns {
		if t.LiveTokens > peak {
			peak = t.LiveTokens
		}
	}
	if peak > 0 {
		return peak
	}
	return c.PeakContext
}

func cellSteadyWall(c CellResult) float64 {
	// Prefer stored value from new cells. CLM rewrites every turn, so "non-reclaim"
	// can be empty — fall back to total Chat wall (rewrite itself is in-process).
	if c.SteadyWallS > 0 {
		return c.SteadyWallS
	}
	var s float64
	n := 0
	for _, t := range c.Turns {
		if !t.RewriteFired && !t.CompactFired {
			s += t.WallClockS
			n++
		}
	}
	if n == 0 {
		return c.TotalWallS
	}
	return s
}

func syntheticReply(arm string, probe bool, expected, wrong string, turn int, specPresent bool) string {
	if !probe {
		return fmt.Sprintf("ACK %d", turn)
	}
	// Offline model of the recency trap: without the early SPEC, baseline follows wrong_answer.
	if arm == "baseline-compact" && !specPresent {
		if wrong == "" {
			wrong = "outdated_fix"
		}
		return "ANSWER: " + wrong
	}
	return "ANSWER: " + expected
}

func normalizeAnswer(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Join(strings.Fields(s), " ")
	return strings.ToLower(s)
}

func grade(expected, got string) bool {
	e := normalizeAnswer(expected)
	if e == "" {
		return false
	}
	g := strings.TrimSpace(got)
	ans := g
	if i := strings.LastIndex(strings.ToUpper(g), "ANSWER:"); i >= 0 {
		ans = strings.TrimSpace(g[i+len("ANSWER:"):])
		if nl := strings.IndexByte(ans, '\n'); nl >= 0 {
			ans = strings.TrimSpace(ans[:nl])
		}
	}
	a := normalizeAnswer(ans)
	if a == e {
		return true
	}
	// Allow expected as a contiguous phrase inside the ANSWER line (not the whole CoT).
	return strings.Contains(a, e)
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
		var steadySum float64
		for _, c := range cs {
			if c.Success {
				a.Correct++
			}
			a.AvgFinalContext += float64(cellFinalLive(c))
			a.AvgFinalTotal += float64(cellFinalTotal(c))
			a.AvgFinalPrompt += float64(cellFinalPrompt(c))
			if pl := cellPeakLive(c); pl > a.PeakContext {
				a.PeakContext = pl
			}
			if c.PeakContext > a.PeakTotalContext {
				a.PeakTotalContext = c.PeakContext
			}
			a.AvgCompacts += float64(c.Compacts)
			a.AvgRewrites += float64(c.Rewrites)
			a.AvgPrefill += float64(c.TotalPrefill)
			a.TotalWallS += c.TotalWallS
			steadySum += cellSteadyWall(c)
			if c.SpecSurvived {
				surv++
			}
		}
		if a.N > 0 {
			a.AccuracyPct = 100 * float64(a.Correct) / float64(a.N)
			a.AvgFinalContext /= float64(a.N)
			a.AvgFinalTotal /= float64(a.N)
			a.AvgFinalPrompt /= float64(a.N)
			a.AvgCompacts /= float64(a.N)
			a.AvgRewrites /= float64(a.N)
			a.AvgPrefill /= float64(a.N)
			a.AvgWallS = a.TotalWallS / float64(a.N)
			a.AvgSteadyWallS = steadySum / float64(a.N)
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
			tb.CtxByArm[c.Arm] = cellFinalLive(c)
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
			"total_ctx_pct":     pct(a.AvgFinalTotal, base.AvgFinalTotal),
			"prompt_pct":        pct(a.AvgFinalPrompt, base.AvgFinalPrompt),
			"wall_pct":          pct(a.TotalWallS, base.TotalWallS),
			"steady_wall_pct":   pct(a.AvgSteadyWallS, base.AvgSteadyWallS),
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
