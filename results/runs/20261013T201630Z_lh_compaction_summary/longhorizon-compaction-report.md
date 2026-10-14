# DynCLM Long-Horizon B -  Compaction Matrix

Generated: 2026-10-13T20:16:31Z

Long-horizon suite B (compaction matrix): same session as A, but the raw baseline is replaced by baseline-compact -  a Codex-style rolling summary when context exceeds max_context (keep system + recent raw tail; fold older spans into a budget-capped [CONTEXT_SUMMARY] with an early-requirements quota plus newest-preferred notes; re-prefill survivors). DynCLM and immediate-rewrite (CLM) use the same max_context for the scheduler β threshold and reclaim via soft-invalidation + rewrite instead of lossy summarization. Matrix over max_context ∈ {8k,16k,32k,64k}. Per-column noise is sized so cumulative appends exceed that column's limit (~1.4×), ensuring baseline-compact must compact at every limit without forcing 64k-sized prompts into the 8k column.

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
| 16384 | 14502 | 5154 | 320 |
| 65536 | 58201 | 19348 | 320 |

## Late steady wall-clock (s) by max context

| Max context | baseline-compact | DynCLM | CLM (immediate) |
|---|---|---|---|
| 16384 | 4.04 | 1.45 | 1.15 |
| 65536 | 3.90 | 1.78 | 1.65 |

## Late steady gen tok/s by max context

| Max context | baseline-compact | DynCLM | CLM (immediate) |
|---|---|---|---|
| 16384 | 2.41 | 3.33 | 3.89 |
| 65536 | 1.12 | 2.74 | 2.74 |

## Figures

- ![probe](figures/compaction_probe_vs_limit.svg)
- ![context](figures/compaction_context_vs_limit.svg)
- ![wall](figures/compaction_wall_vs_limit.svg)
- ![tok/s](figures/compaction_toks_vs_limit.svg)
