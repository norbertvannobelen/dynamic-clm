# DynCLM Long-Horizon Report

Generated: 2026-10-10T17:46:57Z

## Why this suite exists (methodology re-evaluation)

Long-horizon suite A: early authoritative FACT planted at turn 1; each later turn appends large obsolete tool noise (and competing pseudo-FACTs). Soft-invalidation markers are harness-applied on obsolete spans for DynCLM/immediate (CLM) arms; the cost-model scheduler fires batched rewrites. Periodic probes ask for the early FACT. Metrics per turn: context length, prompt/generated tokens, wall-clock, gen tokens/s. Early vs late windows express the paper claim that smaller context improves late-session quality and generation performance vs raw append-only.

- Model: `Qwen3.8-27B-4bit` · Hardware: `apple-m5-max` · Live: true
- Sessions/arm: 3 · Turns: 24 · Probe every: 4
- Early window: turns 1–8 · Late window: turns 16–24

## Late-session results (the paper-relevant window)

| Arm | Late probe accuracy | Late avg ctx | Late avg prompt | Late wall-s | Late steady wall-s | Late gen tok/s | Prefill/session | Rewrites/session |
|---|---|---|---|---|---|---|---|---|
| baseline | **0.0%** (0/9) | 6857 | 4245 | 2.60 | 2.60 | 2.19 | 0 | 0.0 |
| immediate-rewrite | **100.0%** (9/9) | 375 | 618 | 1.99 | 1.99 | 2.36 | 2774 | 24.0 |
| dynclm | **100.0%** (9/9) | 2000 | 1599 | 3.78 | 3.78 | 1.74 | 144 | 1.0 |

## Early vs late (DynCLM vs baseline)

| Window | baseline probe% | DynCLM probe% | baseline ctx | DynCLM ctx | baseline wall-s | DynCLM wall-s | baseline tok/s | DynCLM tok/s |
|---|---|---|---|---|---|---|---|---|
| early | 0.0 | 100.0 | 1586 | 1587 | 1.92 | 3.29 | 1.99 | 1.36 |
| late | 0.0 | 100.0 | 6857 | 2000 | 2.60 | 3.78 | 2.19 | 1.74 |

## DynCLM vs baseline (late-window metrics)

| Metric | baseline | DynCLM | Δ | % change | Better? |
|---|---|---|---|---|---|
| accuracy_pct | 0.00 | 100.00 | +100.00 | +100.0% | yes |
| avg_context_tokens | 6857.33 | 2000.00 | -4857.33 | -70.8% | yes |
| peak_context_tokens | 8216.00 | 4822.00 | -3394.00 | -41.3% | yes |
| final_context_tokens | 6857.33 | 2000.00 | -4857.33 | -70.8% | yes |
| avg_dead_tokens | 0.00 | 0.00 | +0.00 | +0.0% | - |
| avg_live_tokens | 0.00 | 0.00 | +0.00 | +0.0% | - |
| avg_prefill_tokens | 0.00 | 144.00 | +144.00 | +100.0% | no |
| avg_prompt_tokens | 4244.78 | 1598.67 | -2646.11 | -62.3% | yes |
| avg_generated_tokens | 4.33 | 5.00 | +0.67 | +15.4% | no |
| avg_total_tokens | 4249.11 | 1603.67 | -2645.44 | -62.3% | yes |
| avg_rewrites | 0.00 | 1.00 | +1.00 | +100.0% | no |
| avg_dead_carried_turn_sum | 0.00 | 0.00 | +0.00 | +0.0% | - |
| avg_wall_clock_s | 2.60 | 3.78 | +1.19 | +45.6% | no |
| total_wall_clock_s | 160.03 | 239.33 | +79.31 | +49.6% | no |
| wall_clock_per_correct_s | 0.00 | 26.59 | +26.59 | +100.0% | no |
| generated_tokens_per_correct | 0.00 | 5.00 | +5.00 | +100.0% | no |
| prefill_tokens_per_correct | 0.00 | 144.00 | +144.00 | +100.0% | no |

## Headlines

- **Late quality vs baseline:** +100.0 pp (0.0% → 100.0%)
- **Late context vs baseline:** -70.8% (6857 → 2000)
- **Late wall-clock vs baseline (all late turns):** +45.6% (2.60s → 3.78s)
- **Late steady wall-clock vs baseline (exclude rewrite/compact turns):** +45.6% (2.60s → 3.78s)
- **Late steady gen tok/s vs baseline:** -20.6% (2.19 → 1.74)
- **Immediate-rewrite (CLM) late steady wall vs baseline:** -23.2% (2.60s → 1.99s) - upper bound if N is high

Note: gen tok/s uses completion_tokens/wall_clock (includes prefill). Steady-state excludes rewrite/compact-event turns so generation-under-context is not mixed with reclaim spikes.
