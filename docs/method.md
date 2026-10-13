# How DynCLM works (in this repo)

DynCLM is a three-phase context manager: **append**, **soft-invalidation**, **deferred physical rewrite**. Contrast with Codex-style summarize compact: [dynclm-vs-compaction.md](dynclm-vs-compaction.md).

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

## Where this shows up in evals

| Suite | Who marks | Who rewrites |
|---|---|---|
| Gate / session runner | Model prompted to emit markers (synthetic fallback in offline mode) | DynCLM or immediate-rewrite policy |
| Long-horizon / SWE-Distill | Harness applies `FormatIgnore` on obsolete tool/SPEC dumps | Same policies; baseline(-compact) never soft-marks |

Code entry points: `internal/session/runner.go`, `internal/eval/longhorizon`, `internal/eval/swedistill`, `internal/eval/quality`.
