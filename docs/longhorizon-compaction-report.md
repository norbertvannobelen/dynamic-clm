# DynCLM Long-Horizon B — Compaction Matrix

> **Stale baseline methodology.** Table numbers below are from a live run that used **blind FIFO drop-oldest**. Baseline+compact now uses **Codex-style rolling summarization**; re-run the matrix before citing 0%→100% late-probe gaps.

Generated: 2026-10-10T17:46:57Z

Long-horizon suite B (compaction matrix): same session as A, but the raw baseline is replaced by baseline-compact — Codex-style rolling summary when context exceeds max_context (keep system + recent raw; fold older spans into `[CONTEXT_SUMMARY]`; re-prefill). DynCLM and immediate-rewrite (CLM) reclaim via soft-invalidation + rewrite. Matrix over max_context ∈ {8k,16k,32k,64k}. Per-column noise is sized so cumulative appends exceed that column's limit (~1.4×).

- Model: `Qwen3.8-27B-4bit` · Hardware: `apple-m5-max` · Live: true
- Sessions/arm: 2 · Turns: 20 · NoisePad: 4587 chars
- Max context limits: [16384 65536]

## Late probe accuracy (%) by max context

| Max context | baseline-compact | DynCLM | CLM (immediate) |
|---|---|---|---|
| 16384 | 0.0 | 100.0 | 100.0 |
| 65536 | 0.0 | 100.0 | 100.0 |

## Late avg context tokens by max context

| Max context | baseline-compact | DynCLM | CLM (immediate) |
|---|---|---|---|
| 16384 | 14154 | 5154 | 320 |
| 65536 | 57914 | 19348 | 320 |

## Late steady wall-clock (s) by max context

| Max context | baseline-compact | DynCLM | CLM (immediate) |
|---|---|---|---|
| 16384 | 6.23 | 4.18 | 1.75 |
| 65536 | 3.56 | 6.50 | 1.71 |

## Late steady gen tok/s by max context

| Max context | baseline-compact | DynCLM | CLM (immediate) |
|---|---|---|---|
| 16384 | 2.00 | 1.56 | 2.57 |
| 65536 | 1.22 | 1.11 | 2.63 |

## Figures

- ![probe](figures/compaction_probe_vs_limit.svg)
- ![context](figures/compaction_context_vs_limit.svg)
- ![wall](figures/compaction_wall_vs_limit.svg)
- ![tok/s](figures/compaction_toks_vs_limit.svg)
