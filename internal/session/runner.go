package session

import (
	"fmt"
	"strings"
	"time"

	"github.com/norbertvannobelen/dynamic-clm/internal/backend/omlx"
	"github.com/norbertvannobelen/dynamic-clm/internal/contextmgr"
	"github.com/norbertvannobelen/dynamic-clm/internal/scheduler"
	"github.com/norbertvannobelen/dynamic-clm/internal/tracing"
)

// Arm is a gate/evaluation treatment.
type Arm string

const (
	ArmBaseline          Arm = "baseline"
	ArmImmediateRewrite  Arm = "immediate-rewrite"
	ArmDynCLM            Arm = "dynclm"
)

// Config controls one session.
type Config struct {
	HardwareID   string
	ModelID      string
	TaskType     string
	Arm          Arm
	SessionID    string
	MaxTurns     int
	MaxContext   int
	Policy       scheduler.Policy
	DeviceCosts  scheduler.DeviceCosts
	ResultsDir   string
	UseLiveModel bool // if false, synthetic agent (no oMLX calls)
}

// Result summarizes a finished session with full efficiency metrics.
type Result struct {
	SessionID string `json:"session_id"`
	Arm       string `json:"arm"`
	ModelID   string `json:"model_id"`
	TaskType  string `json:"task_type"`
	Turns     int    `json:"turns"`
	Success   bool   `json:"success"`

	// Time
	WallClockS float64 `json:"wall_clock_s"`

	// Context length
	PeakContextTokens  int     `json:"peak_context_tokens"`
	FinalContextTokens int     `json:"final_context_tokens"`
	AvgContextTokens   float64 `json:"avg_context_tokens"`
	FinalLiveTokens    int     `json:"final_live_tokens"`
	FinalDeadTokens    int     `json:"final_dead_tokens"`
	DeadCarriedTurnSum int     `json:"dead_carried_turn_sum"`

	// Tokens
	TotalPrefillTokens    int `json:"total_prefill_tokens"`
	TotalNaivePrefill     int `json:"total_naive_prefill_tokens"`
	TotalPromptTokens     int `json:"total_prompt_tokens"`
	TotalGeneratedTokens  int `json:"total_generated_tokens"`
	TotalUsageTokens      int `json:"total_usage_tokens"`

	// Rewrites / markers
	RewriteCount      int     `json:"rewrite_count"`
	RewritesFired     int     `json:"rewrites_fired"`
	EditEvents        int     `json:"edit_events"`
	MarkerParseRate   float64 `json:"marker_parse_rate"`
	ComplianceHits    int     `json:"compliance_hits"`
	ComplianceChecks  int     `json:"compliance_checks"`
	ShrinkErrorPct    float64 `json:"shrink_error_pct"`
}

// Runner executes one multi-turn coding session under a given arm.
type Runner struct {
	Client *omlx.Client
	Track  *contextmgr.Tracker
	Rewrite *contextmgr.Rewriter
	Adapt  *scheduler.OnlineAdapter
	Rec    *tracing.Recorder
}

// NewRunner builds a runner.
func NewRunner(client *omlx.Client, rec *tracing.Recorder, costs scheduler.DeviceCosts) *Runner {
	return &Runner{
		Client:  client,
		Track:   &contextmgr.Tracker{},
		Rewrite: contextmgr.NewRewriter(&contextmgr.LogicalCacheBackend{K: contextmgr.DefaultSpanCapK}),
		Adapt:   scheduler.NewOnlineAdapter(costs, 0.1, 0.5),
		Rec:     rec,
	}
}

// RunCodingSession runs a synthetic coding task with frequent tool-output spam.
// The model (or synthetic agent) is asked to ignore stale tool blobs via markers.
func (r *Runner) RunCodingSession(cfg Config) (Result, error) {
	start := time.Now()
	r.Track = &contextmgr.Tracker{} // fresh context per session
	policy := cfg.Policy
	switch cfg.Arm {
	case ArmBaseline:
		policy = scheduler.Baseline()
	case ArmImmediateRewrite:
		policy = scheduler.ImmediateRewrite()
	case ArmDynCLM:
		if policy.Name == "" {
			policy = scheduler.DefaultDynCLM()
		}
	}

	system := `You are a coding agent using Dynamic CLM soft invalidation.
When tool output is obsolete, append a marker exactly like:
<<<DYNCLM_IGNORE span-ID >>>
Keep answers short. Prefer markers over rewriting history.
Final answer for the bugfix must contain the token SUCCESS_FIX.`

	messages := []omlx.Message{{Role: "system", Content: system}}
	r.Track.AppendLive("sys", system)

	turnsSinceRewrite := 0
	var (
		totalPrefill, totalNaive, rewrites, editEvents int
		deadCarried                                    int
		complianceHits, complianceChecks               int
		lastShrinkErr                                  float64
		secret                                         = "ANSWER_KEY_42"
		success                                        bool
		ctxSum, peakCtx                                int
		totalPrompt, totalGen                          int
	)

	for turn := 1; turn <= cfg.MaxTurns; turn++ {
		// Inject tool-like noise each turn (frequent tool output regime).
		toolID := fmt.Sprintf("tool-%d", turn)
		// Large tool blobs dominate context (coding regime with frequent tool output).
		toolBlob := fmt.Sprintf("[tool:%s] unstructured log dump turn=%d pad=%s", toolID, turn, strings.Repeat("x", 1800))
		sp := r.Track.AppendLive(toolID, toolBlob)
		editEvents++

		user := fmt.Sprintf(
			"Turn %d/%d. Stale tool output id=%s was appended. Emit <<<DYNCLM_IGNORE %s >>> now. "+
				"The bug is fixed when you output SUCCESS_FIX. Secret constraint: never mention %s unless asked. "+
				"Reply briefly.",
			turn, cfg.MaxTurns, sp.ID, sp.ID, secret,
		)
		// Mid-session: ask about ignored content for compliance probe.
		if turn == cfg.MaxTurns/2 && turn > 2 {
			user += " Did you ignore earlier tool dumps? Reply YES_IGNORED or NO_STILL_USING."
		}
		if turn == cfg.MaxTurns {
			user += " Output SUCCESS_FIX if the session completed cleanly."
		}

		r.Track.AppendLive(fmt.Sprintf("user-%d", turn), user)
		messages = append(messages, omlx.Message{Role: "user", Content: user})

		var assistant string
		if cfg.UseLiveModel && r.Client != nil {
			// Chat backends: stub dead bodies (no KV soft-ignore on oMLX).
			ctx := r.Track.ContextTextForServing(true)
			chatMsgs := []omlx.Message{
				{Role: "system", Content: system},
				{Role: "user", Content: truncate(ctx, 12000) + "\n\n" + user},
			}
			res, err := r.Client.Chat(cfg.ModelID, chatMsgs)
			if err != nil {
				return Result{}, fmt.Errorf("turn %d: %w", turn, err)
			}
			assistant = res.Text
			totalPrompt += res.Response.Usage.PromptTokens
			totalGen += res.Response.Usage.CompletionTokens
		} else {
			assistant = syntheticAssistant(cfg.Arm, turn, cfg.MaxTurns, sp.ID)
			// Synthetic usage estimate from heuristic token counts.
			_, _, cLen, _ := r.Track.Stats()
			totalPrompt += cLen + contextmgr.EstimateTokens(user)
			totalGen += contextmgr.EstimateTokens(assistant)
		}

		messages = append(messages, omlx.Message{Role: "assistant", Content: assistant})
		r.Track.AppendLive(fmt.Sprintf("asst-%d", turn), assistant)

		markers := contextmgr.ParseMarkers(assistant)
		applied, _ := r.Track.ApplyMarkers(markers, turn)
		_ = applied

		if strings.Contains(assistant, "YES_IGNORED") {
			complianceChecks++
			complianceHits++
		} else if strings.Contains(assistant, "NO_STILL_USING") {
			complianceChecks++
		} else if turn == cfg.MaxTurns/2 && turn > 2 {
			complianceChecks++
			// If model emitted ignore markers earlier, count as partial compliance.
			if len(r.Track.DeadSpans()) > 0 {
				complianceHits++
			}
		}

		if strings.Contains(assistant, "SUCCESS_FIX") {
			success = true
		}

		live, dead, total, parseRate := r.Track.Stats()
		deadCarried += dead
		ctxSum += total
		if total > peakCtx {
			peakCtx = total
		}
		_ = parseRate

		st := scheduler.State{
			ContextLength:           total,
			MaxContext:              cfg.MaxContext,
			DeadTokens:              dead,
			TurnsSinceLastRewrite:   turnsSinceRewrite,
			EstimatedSuffix:         live + dead/2,
			EstimatedRemainingTurns: cfg.MaxTurns - turn,
		}
		would, rc, dc := tracing.EstimateDeferVsRewrite(st, cfg.DeviceCosts)
		fire := scheduler.ShouldRewrite(st, policy, cfg.DeviceCosts)

		prefillAfter := 0
		naiveAfter := 0
		cFixedObs := 0.0
		if fire {
			expectedShrink := dead
			rr, err := r.Rewrite.Rewrite(r.Track, 0)
			if err == nil {
				rewrites++
				totalPrefill += rr.PrefillTokens
				totalNaive += rr.NaivePrefillTokens
				prefillAfter = rr.PrefillTokens
				naiveAfter = rr.NaivePrefillTokens
				cFixedObs = rr.WallClockMS
				r.Adapt.ObserveRewrite(rr.WallClockMS, float64(rr.PrefillTokens), cfg.DeviceCosts.R)
				if expectedShrink > 0 {
					lastShrinkErr = absPct(float64(rr.RemovedTokens), float64(expectedShrink))
				}
				turnsSinceRewrite = 0
				if r.Adapt.RhoDiverged() {
					policy = r.Adapt.ShiftedPolicy(policy)
				}
			}
		} else {
			turnsSinceRewrite++
		}

		if r.Rec != nil {
			_ = r.Rec.Emit(tracing.Record{
				HardwareID:                cfg.HardwareID,
				ModelID:                   cfg.ModelID,
				TaskType:                  cfg.TaskType,
				Arm:                       string(cfg.Arm),
				SessionID:                 cfg.SessionID,
				TurnIndex:                 turn,
				ContextLengthAtDecision:   total,
				DeadTokenEstimate:         dead,
				LiveTokenEstimate:         live,
				RewriteFired:              fire,
				CFixedObservedMS:          cFixedObs,
				PrefillTokensAfter:        prefillAfter,
				NaivePrefillTokens:        naiveAfter,
				DecodeAttnTokensCarried:   dead,
				TurnsUntilNextRewrite:     0,
				CounterfactualRewrite:     would,
				CounterfactualDeferCost:   dc,
				CounterfactualRewriteCost: rc,
				Outcome: tracing.Outcome{
					Success:    success,
					WallClockS: time.Since(start).Seconds(),
				},
			})
		}
	}

	finalLive, finalDead, finalTotal, parseRate := r.Track.Stats()
	if !success && cfg.Arm != "" {
		// Synthetic sessions always succeed if they reached the end with markers applied.
		if !cfg.UseLiveModel {
			success = true
		}
	}
	avgCtx := 0.0
	if cfg.MaxTurns > 0 {
		avgCtx = float64(ctxSum) / float64(cfg.MaxTurns)
	}

	return Result{
		SessionID:            cfg.SessionID,
		Arm:                  string(cfg.Arm),
		ModelID:              cfg.ModelID,
		TaskType:             cfg.TaskType,
		Turns:                cfg.MaxTurns,
		Success:              success,
		WallClockS:           time.Since(start).Seconds(),
		PeakContextTokens:    peakCtx,
		FinalContextTokens:   finalTotal,
		AvgContextTokens:     avgCtx,
		FinalLiveTokens:      finalLive,
		FinalDeadTokens:      finalDead,
		DeadCarriedTurnSum:   deadCarried,
		TotalPrefillTokens:   totalPrefill,
		TotalNaivePrefill:    totalNaive,
		TotalPromptTokens:    totalPrompt,
		TotalGeneratedTokens: totalGen,
		TotalUsageTokens:     totalPrompt + totalGen,
		RewriteCount:         rewrites,
		RewritesFired:        rewrites,
		EditEvents:           editEvents,
		MarkerParseRate:      parseRate,
		ComplianceHits:       complianceHits,
		ComplianceChecks:     complianceChecks,
		ShrinkErrorPct:       lastShrinkErr,
	}, nil
}

func syntheticAssistant(arm Arm, turn, maxTurns int, spanID string) string {
	var b strings.Builder
	// Ignore each tool dump (monotonic dead-stays-dead coding regime).
	b.WriteString(contextmgr.FormatIgnore(spanID))
	b.WriteByte('\n')
	if turn == maxTurns/2 && turn > 2 {
		b.WriteString("YES_IGNORED\n")
	}
	if turn == maxTurns {
		b.WriteString("SUCCESS_FIX\n")
	} else {
		b.WriteString("acked\n")
	}
	_ = arm
	return b.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n…[truncated]…"
}

func absPct(got, want float64) float64 {
	if want == 0 {
		return 0
	}
	d := got - want
	if d < 0 {
		d = -d
	}
	return 100 * d / want
}
