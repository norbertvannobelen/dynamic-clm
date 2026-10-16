# DynCLM Long-Horizon Report

Generated: 2026-10-15T22:01:12Z

## Why this suite exists (methodology re-evaluation)

Long-horizon suite A: early authoritative FACT planted at turn 1; each later turn appends large obsolete tool noise (and competing pseudo-FACTs). Soft-invalidation markers are harness-applied on obsolete spans for DynCLM/immediate (CLM) arms; the cost-model scheduler fires batched rewrites. Periodic probes ask for the early FACT. Metrics per turn: context length, prompt/generated tokens, wall-clock, gen tokens/s. Early vs late windows express the paper claim that smaller context improves late-session quality and generation performance vs raw append-only.

- Model: `Qwen3.8-27B-4bit` · Hardware: `apple-m5-max` · Live: true
- Sessions/arm: 8 · Turns: 24 · Probe every: 4
- Early window: turns 1–8 · Late window: turns 16–24

## Late-session results (the paper-relevant window)

| Arm | Late probe accuracy | Late avg ctx | Late avg prompt | Late wall-s | Late steady wall-s | Late gen tok/s | Prefill/session | Rewrites/session |
|---|---|---|---|---|---|---|---|---|
| baseline | **0.0%** (0/24) | 6857 | 4245 | 2.49 | 2.49 | 2.45 | 0 | 0.0 |
| immediate-rewrite | **100.0%** (24/24) | 375 | 618 | 1.50 | 1.50 | 3.12 | 2774 | 24.0 |
| dynclm | **100.0%** (24/24) | 2000 | 729 | 1.71 | 1.71 | 2.96 | 144 | 1.0 |

## Early vs late (DynCLM vs baseline)

| Window | baseline probe% | DynCLM probe% | baseline ctx | DynCLM ctx | baseline wall-s | DynCLM wall-s | baseline tok/s | DynCLM tok/s |
|---|---|---|---|---|---|---|---|---|
| early | 0.0 | 100.0 | 1586 | 1587 | 2.22 | 0.97 | 1.71 | 4.19 |
| late | 0.0 | 100.0 | 6857 | 2000 | 2.49 | 1.71 | 2.45 | 2.96 |

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
| avg_prompt_tokens | 4244.78 | 728.67 | -3516.11 | -82.8% | yes |
| avg_generated_tokens | 4.33 | 5.00 | +0.67 | +15.4% | no |
| avg_total_tokens | 4249.11 | 733.67 | -3515.44 | -82.7% | yes |
| avg_rewrites | 0.00 | 1.00 | +1.00 | +100.0% | no |
| avg_dead_carried_turn_sum | 0.00 | 0.00 | +0.00 | +0.0% | - |
| avg_wall_clock_s | 2.49 | 1.71 | -0.78 | -31.3% | yes |
| total_wall_clock_s | 435.09 | 268.07 | -167.02 | -38.4% | yes |
| wall_clock_per_correct_s | 0.00 | 11.17 | +11.17 | +100.0% | no |
| generated_tokens_per_correct | 0.00 | 5.00 | +5.00 | +100.0% | no |
| prefill_tokens_per_correct | 0.00 | 144.00 | +144.00 | +100.0% | no |

## Headlines

- **Late quality vs baseline:** +100.0 pp (0.0% → 100.0%)
- **Late context vs baseline:** -70.8% (6857 → 2000)
- **Late wall-clock vs baseline (all late turns):** -31.3% (2.49s → 1.71s)
- **Late steady wall-clock vs baseline (exclude rewrite/compact turns):** -31.3% (2.49s → 1.71s)
- **Late steady gen tok/s vs baseline:** +21.0% (2.45 → 2.96)
- **Immediate-rewrite (CLM) late steady wall vs baseline:** -39.6% (2.49s → 1.50s) - upper bound if N is high

Note: gen tok/s uses completion_tokens/wall_clock (includes prefill). Steady-state excludes rewrite/compact-event turns so generation-under-context is not mixed with reclaim spikes.
