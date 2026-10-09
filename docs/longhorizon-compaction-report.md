# DynCLM Long-Horizon B — Compaction Matrix

Generated: 2026-10-09T01:55:40Z

Long-horizon suite B (compaction matrix): same recency-trap session as A, but the raw baseline is replaced by baseline-compact — when context exceeds max_context it drops oldest spans (keeping only the system prompt when possible), forcing a full re-prefill of survivors. DynCLM and immediate-rewrite (CLM) use the same max_context for the scheduler β threshold and reclaim via soft-invalidation + rewrite instead of blind truncation. Matrix over max_context ∈ {8k,16k,32k,64k}. Per-column noise is sized so cumulative appends exceed that column's limit (~1.4×), ensuring baseline-compact must compact at every limit without forcing 64k-sized prompts into the 8k column.

- Model: `Qwen3.8-27B-4bit` · Hardware: `apple-m5-max` · Live: true
- Sessions/arm: 1 · Turns: 16 · NoisePad: 2867 chars
- Max context limits: [8192 16384 32768 65536]

## Late probe accuracy (%) by max context

| Max context | baseline-compact | DynCLM | CLM (immediate) |
|---|---|---|---|
| 8192 | 0.0 | 100.0 | 100.0 |
| 16384 | 0.0 | 100.0 | 100.0 |
| 32768 | 0.0 | 100.0 | 100.0 |
| 65536 | 0.0 | 100.0 | 100.0 |

## Late avg context tokens by max context

| Max context | baseline-compact | DynCLM | CLM (immediate) |
|---|---|---|---|
| 8192 | 6980 | 2488 | 263 |
| 16384 | 14169 | 4639 | 263 |
| 32768 | 29120 | 8938 | 263 |
| 65536 | 57804 | 17540 | 263 |

## Late steady wall-clock (s) by max context

| Max context | baseline-compact | DynCLM | CLM (immediate) |
|---|---|---|---|
| 8192 | 6.09 | 7.76 | 1.56 |
| 16384 | 11.88 | 7.58 | 1.82 |
| 32768 | 7.70 | 10.53 | 1.75 |
| 65536 | 3.94 | 13.52 | 1.83 |

## Late steady gen tok/s by max context

| Max context | baseline-compact | DynCLM | CLM (immediate) |
|---|---|---|---|
| 8192 | 0.76 | 0.72 | 2.93 |
| 16384 | 0.41 | 0.73 | 2.51 |
| 32768 | 0.62 | 0.51 | 2.60 |
| 65536 | 1.07 | 0.43 | 2.50 |

## Headlines (live)

- **Late quality:** compaction baseline **0%** at every limit; DynCLM and CLM **100%** (early FACT survives soft-invalidation/rewrite; blind truncate drops it).
- **Late context:** DynCLM holds ~35% of the compaction baseline’s late context at each limit; CLM stays ~260 tokens.
- **Late steady speed:** CLM is fastest at every limit (~1.6–1.8s/turn). DynCLM beats compaction on wall at 16k (−36%); at 8k/32k/64k compaction can look faster while answering wrong — speed without the FACT is not a win.
- **n=1 session/arm** — treat wall/tok/s as directional; quality/context pattern is stable.

## Figures

- ![probe](figures/compaction_probe_vs_limit.svg)
- ![context](figures/compaction_context_vs_limit.svg)
- ![wall](figures/compaction_wall_vs_limit.svg)
- ![tok/s](figures/compaction_toks_vs_limit.svg)
