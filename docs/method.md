# How DynCLM works (in this repo)

DynCLM is a three-phase context manager: **append**, **soft-invalidation**, **deferred physical rewrite**.

| | DynCLM | CLM (immediate rewrite) | Codex-style summarize |
|---|---|---|---|
| Soft-ignore stale spans | yes | yes | no (folds age into a note) |
| When physical reclaim runs | cost model (`β`,`γ`,`Δ` + defer vs rewrite cost) | every turn with dead tokens | when over `max_context` |
| Survivors | verbatim live spans | verbatim live spans | lossy `[CONTEXT_SUMMARY]` + recent raw |
| Session rewrite count | low (batched) | high (every edit) | compact count (not marker rewrites) |

Same soft-invalidation vocabulary as CLM; DynCLM’s difference is **deferral**. Mechanism vs summarize: [dynclm-vs-compaction.md](dynclm-vs-compaction.md). Math sketch of DynCLM vs CLM context load: below.

## 1. Append (normal turns)

New system / user / assistant / tool text is recorded as **live spans** in a tracker (`internal/contextmgr`). The logical context is the ordered list of spans. Prefix reuse stays intact because history is still append-only.

## 2. Soft-invalidation (mark stale spans dead)

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

## 3. Deferred rewrite (new context from survivors)

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

When the policy fires, `Rewriter.Rewrite` (`internal/contextmgr/rewriter.go`):

1. Collect **live** spans (survivors) and **dead** spans.
2. Account re-prefill under logical SCR-shaped relocation (`LogicalCacheBackend`, span cap K=6) - not physical KV on oMLX.
3. **`RemoveDead()`** - drop dead spans from the tracker.
4. The remaining live spans **are** the updated context: same role as loading a new context file built only from survivors (verbatim text, no summary paraphrase).

After rewrite, subsequent turns append onto that smaller span list. Prefill charged for the reclaim is what gate / SWE tables call rewrite prefill.

## End-to-end picture

```text
turn N:  append tool dump (live span)
         emit / apply <<<DYNCLM_IGNORE tool-N >>>
         serve Chat with stubbed dead bodies  ← soft phase

… carry soft-dead until β/γ/Δ + cost model say rewrite …

rewrite: drop dead spans
         survivors = new context
         charge re-prefill of survivors
         continue session from that context
```

## DynCLM vs CLM: session context load (toy math)

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

**Ratios (the deferral tradeoff):**

\[
\frac{\Lambda_{\mathrm{Dyn}}}{\Lambda_{\mathrm{CLM}}} \;=\; 1 + \frac{g(R-1)}{2L}, \qquad
\frac{N_{\mathrm{Dyn}}}{N_{\mathrm{CLM}}} \;=\; \frac{1}{R}, \qquad
\frac{P_{\mathrm{Dyn}}}{P_{\mathrm{CLM}}} \;\approx\; \frac{1}{R}
\]

DynCLM **raises** \(\Lambda\) (carries soft-dead between batched rewrites) and **cuts** rewrite count / rewrite-prefill by about \(R\).

**Numeric example.** \(L = 2000\), \(g = 3000\), \(T = 12\), DynCLM period \(R = 4\):

| | \(\Lambda = \sum C_t\) | Avg \(C\) | Rewrites \(N\) | Prefill \(P \approx N L\) |
|---|---:|---:|---:|---:|
| CLM (\(R=1\)) | \(12 \times 2000 = 24000\) | 2000 | 12 | 24000 |
| DynCLM (\(R=4\)) | \(24000 + 12\times 3000\times 3/2 = 78000\) | 6500 | 3 | 6000 |

\(\Lambda_{\mathrm{Dyn}} / \Lambda_{\mathrm{CLM}} = 78000/24000 = 3.25 = 1 + 3000\cdot 3/(2\cdot 2000)\), while prefill drops \(24000 \rightarrow 6000\) (\(= 1/R\)).

**Serving note.** On oMLX Chat, `ContextTextForServing` stubs dead bodies, so **billed prompt** between rewrites grows much slower than tracker total \(C_t\) (stubs ≪ \(g\)). The \(\Lambda\) formulas are the logical soft-dead footprint the cost model reasons about; gate tables report avg context and rewrite prefill.

**Live gate (empirical shape).** Decision-suite gate (8×40): DynCLM avg context 5410 vs CLM 3995 (CLM smaller), rewrite prefill 5340 vs 18504, rewrites/session 5.0 vs 17.0 - same tradeoff.

## Where this shows up in evals

| Suite | Who marks | Who rewrites |
|---|---|---|
| Gate / session runner | Model prompted to emit markers (synthetic fallback in offline mode) | DynCLM or immediate-rewrite policy |
| Long-horizon / SWE-Distill | Harness applies `FormatIgnore` on obsolete tool/SPEC dumps | Same policies; baseline(-compact) never soft-marks |

Code entry points: `internal/session/runner.go`, `internal/eval/longhorizon`, `internal/eval/swedistill`, `internal/eval/quality`.
