# DynCLM method

## Premise (why this exists)

Long agent sessions fill the context window with obsolete tool output, wrong drafts, and stale facts. Something has to reclaim space.

**CLM** (Context Language Models) soft-marks known-stale spans, then **physically rewrites** the context to drop them while keeping survivors **verbatim**. That reclaim is correct for selective deletion - but if you run it **every turn** that has dead text, you pay rewrite / re-prefill cost constantly.

**DynCLM** is based on **garbage collection**: soft-mark dead spans (like marking unreachable objects), keep running while that garbage is cheap to carry, then reclaim in a batched physical rewrite when pressure warrants it (like a GC cycle) rather than collecting on every allocation.

**DynCLM** uses the **same mark → drop → survivors-as-new-context methodology as CLM**. The premise is different timing:

- Mark obsolete spans internally (`<<<DYNCLM_IGNORE …>>>`) and keep operating append-only while soft-dead is cheap to carry.
- When we run low on budget / memory pressure (cost model: `β`, `γ`, `Δ` + defer vs rewrite cost), run **one batched rewrite** - the same physical drop CLM would have done.

On systems where **per-turn rewrite is expensive** relative to carrying soft-dead for a while, DynCLM should beat CLM on rewrite count and rewrite-prefill while matching CLM on the quality canaries that need selective ignore. The [math below](#dynclm-vs-clm-session-context-load) isolates that tradeoff; [report.md](report.md) is the live proof attempt on oMLX.

DynCLM is still a context manager (it shrinks context under a budget). It is **not** “anti-compaction.” The question is *which reclaim protocol* and *when* it fires.

---

## Three reclaim protocols

| | Summarize compact | CLM (immediate rewrite) | DynCLM (deferred rewrite) |
|---|---|---|---|
| **Trigger** | Over `max_context` / age | Dead tokens &gt; 0 every turn | Cost model when budget/dead ratio warrants a batch |
| **Selection** | Fold older history into a short note; keep recent raw | Soft-ignore **known-obsolete** spans | Same marks as CLM |
| **Survivors** | Lossy `[CONTEXT_SUMMARY]` + recent raw | Verbatim live spans after rewrite | Same as CLM after rewrite |
| **Prefix / append** | Compact breaks prefix; re-prefill survivors | Soft phase append-only; rewrite re-prefills survivors | Soft phase longer; fewer rewrites |
| **Session rewrite / compact load** | Compact events | High (every edit) | Low (batched) |

**Information channel.** Summarization trades fidelity for length (age-compressed notes can bury early facts under later wrong dumps). CLM/DynCLM try to remove only dead material and keep critical text byte-for-byte.

**Who decides.** Summarizers fold by age/window. CLM/DynCLM mark known-obsolete spans; DynCLM additionally chooses *when* to pay for the physical drop.

### Arms in this repository

| Label | Meaning | Suites |
|---|---|---|
| **Baseline (raw)** | Append-only; no markers; no rewrite; no compaction | Gate, graded quality, long-horizon A |
| **Baseline + compaction** | Codex-style rolling summary over `max_context` | Long-horizon B, SWE-Distill |
| **CLM (immediate rewrite)** | Soft-invalidation + rewrite every turn | All 3-arm suites |
| **DynCLM** | Soft-invalidation + cost-model deferred rewrite | All 3-arm suites |

---

## How DynCLM runs (three phases)

### 1. Append (normal turns)

New system / user / assistant / tool text is recorded as **live spans** in a tracker (`internal/contextmgr`). The logical context is the ordered list of spans. Prefix reuse stays intact because history is still append-only.

### 2. Soft-invalidation (mark stale spans dead)

When a span is obsolete, the session appends a structurally distinct marker (model-emitted or harness-applied):

```text
<<<DYNCLM_IGNORE span-ID >>>
```

Optional revocation:

```text
<<<DYNCLM_UNMARK span-ID >>>
```

Parser: `internal/contextmgr/marker_parser.go` (`ParseMarkers`, `FormatIgnore`).  
Tracker: `ApplyMarkers` sets `Dead=true` on the matching span. Dead tokens still exist in the tracker until rewrite; the model is instructed not to use ignored content.

**Serving before rewrite (oMLX Chat).** There is no KV soft-ignore API. For marker arms we build the Chat prompt with `ContextTextForServing(true)`: live spans in full; each dead span becomes a short stub (`<<<DYNCLM_IGNORE id >>> [omitted N tokens]`) with **no body**. That shrinks billed prompt size without waiting for a rewrite. Implementation: `internal/contextmgr/dead_span_tracker.go`.

### 3. Deferred rewrite (same methodology as CLM, batched)

A cost-model policy decides when to reclaim (`internal/scheduler/policy.go` → `ShouldRewrite`):

| Knob | Role |
|---|---|
| `β` | Fire only when context length ≥ β · max_context |
| `γ` | Fire only when dead/context ≥ γ |
| `Δ` | Minimum turns between rewrites |
| Cost compare | Rewrite cost vs deferral cost of carrying dead tokens (`r`, `k_d`, `C_fixed` from calibration) |

- **DynCLM:** default β=0.6, γ=0.5, Δ=3, plus cost compare.  
- **CLM (immediate-rewrite):** rewrite whenever `DeadTokens > 0`.  
- **Baseline:** never rewrite.

When the policy fires, `Rewriter.Rewrite` (`internal/contextmgr/rewriter.go`) - **same physical step CLM uses**:

1. Collect **live** spans (survivors) and **dead** spans.
2. Account re-prefill under logical SCR-shaped relocation (`LogicalCacheBackend`, span cap K=6) - not physical KV on oMLX.
3. **`RemoveDead()`** - drop dead spans from the tracker.
4. The remaining live spans **are** the updated context: same role as loading a new context file built only from survivors (verbatim text, no summary paraphrase).

After rewrite, subsequent turns append onto that smaller span list. Prefill charged for the reclaim is what gate / SWE tables call rewrite prefill.

### End-to-end picture

```text
turn N:  append tool dump (live span)
         emit / apply <<<DYNCLM_IGNORE tool-N >>>
         serve Chat with stubbed dead bodies  ← soft phase (DynCLM may stay here for many turns)

… carry soft-dead until β/γ/Δ + cost model say rewrite …

rewrite: drop dead spans          ← same methodology as CLM
         survivors = new context
         charge re-prefill of survivors
         continue session from that context
```

---

## DynCLM vs CLM: session context load

Both arms soft-ignore the same garbage. The difference is **how often** dead spans are physically dropped. Define a session of \(T\) turns where each turn:

1. Keeps a live core of \(L\) tokens (system + facts that never die).
2. Appends \(g\) obsolete tool tokens and marks them dead immediately.
3. May run a physical rewrite (policy-dependent).

Let \(C_t\) be the **tracker footprint after that turn’s policy** (live + any remaining soft-dead). Session **context load**:

\[
\Lambda \;=\; \sum_{t=1}^{T} C_t
\]

(Average context is \(\Lambda / T\).)

**CLM (immediate rewrite).** Rewrite every turn after the mark. Soft-dead never carries into the next turn:

\[
C_t^{\mathrm{CLM}} \;=\; L \qquad\Rightarrow\qquad \Lambda_{\mathrm{CLM}} \;=\; T\,L
\]

Rewrite count \(N_{\mathrm{CLM}} = T\). If each rewrite re-prefills about \(L\) survivor tokens, rewrite-prefill load is \(P_{\mathrm{CLM}} \approx T\,L\).

**DynCLM (rewrite every \(R\) turns).** Soft-dead accumulates for \(R-1\) turns, then a batched rewrite on turn \(R\) of each cycle resets to \(L\). Assume \(T = M R\). In a cycle, post-policy footprints are:

\[
C_j \;=\; L + j\,g \quad (j=1,\ldots,R-1), \qquad C_R \;=\; L
\]

Sum over one cycle:

\[
\sum_{j=1}^{R-1} (L + j g) + L \;=\; R L + g\,\frac{R(R-1)}{2}
\]

Over the session (\(M = T/R\) cycles):

\[
\Lambda_{\mathrm{Dyn}} \;=\; T\,L + T g\,\frac{R-1}{2}, \qquad N_{\mathrm{Dyn}} \;=\; \frac{T}{R}, \qquad P_{\mathrm{Dyn}} \approx \frac{T}{R}\,L
\]

**Ratios (the deferral premise in one line):**

\[
\frac{\Lambda_{\mathrm{Dyn}}}{\Lambda_{\mathrm{CLM}}} \;=\; 1 + \frac{g(R-1)}{2L}, \qquad
\frac{N_{\mathrm{Dyn}}}{N_{\mathrm{CLM}}} \;=\; \frac{1}{R}, \qquad
\frac{P_{\mathrm{Dyn}}}{P_{\mathrm{CLM}}} \;\approx\; \frac{1}{R}
\]

DynCLM may **raise** cumulative soft-dead footprint \(\Lambda\) while **cutting** rewrite events / rewrite-prefill by about \(R\). Where rewrite/prefill dominates wall or memory churn, that is the intended win over CLM.

**Numeric example.** \(L = 2000\), \(g = 3000\), \(T = 12\), DynCLM period \(R = 4\):

| | \(\Lambda = \sum C_t\) | Avg \(C\) | Rewrites \(N\) | Prefill \(P \approx N L\) |
|---|---:|---:|---:|---:|
| CLM (\(R=1\)) | \(12 \times 2000 = 24000\) | 2000 | 12 | 24000 |
| DynCLM (\(R=4\)) | \(24000 + 12\times 3000\times 3/2 = 78000\) | 6500 | 3 | 6000 |

\(\Lambda_{\mathrm{Dyn}} / \Lambda_{\mathrm{CLM}} = 3.25\), while prefill drops \(24000 \rightarrow 6000\) (\(= 1/R\)).

**Serving note.** On oMLX Chat, `ContextTextForServing` stubs dead bodies, so **billed prompt** between rewrites grows much slower than tracker total \(C_t\) (stubs ≪ \(g\)). The \(\Lambda\) formulas are the logical soft-dead footprint the cost model reasons about; gate tables report avg context and rewrite prefill.

**Live gate (empirical shape).** Decision-suite gate (8×40): DynCLM avg context 5410 vs CLM 3995 (CLM smaller), rewrite prefill 5340 vs 18504, rewrites/session 5.0 vs 17.0 - same tradeoff. Full results: [report.md](report.md).

---

## What the benchmarks prove

Live suites on oMLX are the proof attempt for the premise (plus quality vs raw / summarize). Do **not** mix suite deltas; each has its own reference baseline.

| Suite | Reference baseline | What it tests |
|---|---|---|
| Short gate | Raw | Mechanism + rewrite prefill vs CLM |
| Graded quality | Raw (short; no window reclaim) | Soft-ignore correctness (not a reclaim stress test) |
| Long-horizon A | Raw + recency | Late FACT canary + late context/wall vs raw |
| Long-horizon B | Summarize compact + recency | Same canary when baseline may summarize |
| SWE-Distill | Summarize compact + recency | Coding-snippet reclaim; `spec_survive` vs accuracy |

Headline numbers and ship table live in [report.md](report.md). Suite write-ups: [longhorizon-compaction-report.md](longhorizon-compaction-report.md), [swe-distill-report.md](swe-distill-report.md).

| Suite | Who marks | Who rewrites |
|---|---|---|
| Gate / session runner | Model prompted to emit markers (synthetic fallback offline) | DynCLM or immediate-rewrite policy |
| Long-horizon / SWE-Distill | Harness applies `FormatIgnore` on obsolete dumps | Same policies; baseline(-compact) never soft-marks |

Code: `internal/session/runner.go`, `internal/eval/longhorizon`, `internal/eval/swedistill`, `internal/eval/quality`, `internal/scheduler`, `internal/contextmgr`.
