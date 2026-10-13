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

1. **Information channel** — Compaction trades fidelity for length. DynCLM/CLM try to remove only dead material and keep critical text byte-for-byte. A paper-style win is better answers under *mild* information loss, not “the baseline forgot the SPEC.”

2. **When reclaim happens** — Summarizers compact because the window is full. DynCLM can carry soft-dead tokens for a while (`β` / `γ` / `Δ`) to protect prefix reuse, then one rewrite. That deferred-cost scheduling is the DynCLM-specific piece; immediate-rewrite CLM is the same reclaim idea without deferral.

3. **Who decides** — Ideal CLM marks “this tool dump / span is obsolete.” Summarization decides “older turns → one note,” which can garble or omit early facts that still matter late.

## Arms in this repository

| Label | Meaning |
|---|---|
| **Baseline (raw)** | Append-only; no markers; no rewrite; no compaction |
| **Baseline + compaction** | Paper-adjacent **Codex-style** rolling summary when over budget (keep system + recent raw; fold older spans into a budget-capped summary; re-prefill). Older eval artifacts may still reflect **blind FIFO drop-oldest** — treat those as a recency-trap baseline, not the current methodology |
| **DynCLM** | Soft-invalidation markers + cost-model deferred rewrite |
| **CLM (immediate rewrite)** | Soft-invalidation markers + rewrite every turn (upper bound on reclaim aggressiveness) |

## How to read measured gaps

- Against a **competent summarization** baseline, expect **modest** joint gains (roughly paper order: ~5–15 pp accuracy with lower prompt/prefill or estimated FLOPs) — same *order of magnitude* as graded quality here (**94.4% → 100%**, +5.6 pp vs raw), not an 80+ pp cliff.
- Against **blind oldest-drop**, late probes and SPEC survival can collapse toward 0% while DynCLM/CLM stay high. That shows a **recency trap**, not that DynCLM beat good Codex-style compaction by that margin.
- **Sanity check:** log whether critical early spans survive as **verbatim** (DynCLM/CLM) vs **only inside a summary** (compaction). Celebrate better answers under mild loss; do not celebrate a baseline that simply lost the authoritative text.

## Bottom line

DynCLM differs from Codex-style compaction in **mechanism and failure mode** (selective verbatim reclaim + scheduled rewrite vs lossy age-compressed notes). It remains in the compaction family. If a strong summarizer retains critical early facts, accuracy deltas should look paper-sized — not a designed 0%→100% retention cliff.

See also: [report.md](report.md) (arms and live suites), [longhorizon-compaction-report.md](longhorizon-compaction-report.md) (matrix vs compact baseline), [swe-distill-report.md](swe-distill-report.md) (coding retention under compact).
