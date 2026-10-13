# DynCLM vs compaction (what this repo implements)

DynCLM is still a **context manager**: it shrinks usable context under a budget. It is **not** “anti-compaction.” The useful distinction is *which reclaim protocol* you use and *how survivors are represented*.

## Same job, different mechanism

| | Codex-style summarization compaction | DynCLM / CLM (this repo) |
|---|---|---|
| **Trigger** | Usually over `max_context` / age of older turns | Stale spans marked dead; rewrite when the cost model fires (DynCLM) or on every edit (CLM / immediate-rewrite) |
| **Selection** | Fold older history into a short note; keep recent raw turns | Drop / soft-ignore **known-obsolete** spans; keep the rest |
| **Survivors** | Lossy paraphrase (`[CONTEXT_SUMMARY]` or similar) | **Verbatim** live spans after physical rewrite |
| **Prefix cache** | Breaks on compact (re-prefill survivors) | Soft-invalidation stays append-only until a batched rewrite |

So: summarization compaction is **lossy rollup of the past**; DynCLM/CLM are **selective physical edit** of dead weight.

## What “different” means in practice

1. **Information channel**: Compaction trades fidelity for length. DynCLM/CLM try to remove only dead material and keep critical text byte-for-byte.
2. **When reclaim happens**: Summarizers compact because the window is full. DynCLM can carry soft-dead tokens for a while (`β` / `γ` / `Δ`) to protect prefix reuse, then one rewrite. Immediate-rewrite CLM is the same reclaim idea without deferral.
3. **Who decides**: DynCLM/CLM mark known-obsolete spans. Summarization folds older turns into one note, which can bury or mix early facts with later wrong dumps.

## Arms in this repository

| Label | Meaning |
|---|---|
| **Baseline (raw)** | Append-only; no markers; no rewrite; no compaction (gate, graded quality, long-horizon A) |
| **Baseline + compaction** | **Codex-style** rolling summary when over budget (keep system + recent raw; fold older spans into a budget-capped `[CONTEXT_SUMMARY]`; re-prefill). Used in long-horizon B and SWE-Distill |
| **DynCLM** | Soft-invalidation markers + cost-model deferred rewrite |
| **CLM (immediate rewrite)** | Soft-invalidation markers + rewrite every turn (upper bound on reclaim aggressiveness) |

## How to read the live gaps (current methodology)

Do **not** mix suites: each uses a different reference baseline and pressure.

| Suite | Reference baseline | Current headline |
|---|---|---|
| Graded quality | Raw (short, no window reclaim) | **94.4% → 100%** (+5.6 pp) |
| Long-horizon A | Raw append-only + recency instruction | Late probe **0% → 100%** |
| Long-horizon B | Summarize compact + recency instruction | Late probe **0% → 100%** (baseline answers `WRONG_*`) |
| SWE-Distill | Summarize compact + recency instruction | **50% → 100%** (+50 pp); **spec survive 100%** on all arms |

On SWE-Distill the early SPEC **stays in context** for the summarize baseline (`spec_survive` 100%). DynCLM/CLM still win because soft-invalidation keeps the authoritative SPEC dominant while baseline follows recent wrong dumps. That is a **recency / confusion** gap with retention, not “baseline deleted the answer.”

Long-horizon B is a harder FACT canary under the same summarize compact: late probes stay **0%** for baseline because every turn plants a newer `WRONG_n` FACT and the prompt says to prefer the most recent.

## Bottom line

DynCLM differs from Codex-style compaction in **mechanism** (selective verbatim reclaim + scheduled rewrite vs lossy age-compressed notes). Live wins are suite-specific: small on short graded quality, large on long-horizon FACT probes, and **+50 pp** on SWE-Distill with full SPEC survival under summarization.

See: [report.md](report.md), [longhorizon-compaction-report.md](longhorizon-compaction-report.md), [swe-distill-report.md](swe-distill-report.md).
