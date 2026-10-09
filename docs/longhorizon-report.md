# DynCLM Long-Horizon Report

Generated: 2026-10-09T01:11:46Z

## Why this suite exists (methodology re-evaluation)

Long-horizon live suite: early authoritative FACT planted at turn 1; each later turn appends large obsolete tool noise (and competing pseudo-FACTs). Soft-invalidation markers are harness-applied on obsolete spans for DynCLM/immediate arms; the cost-model scheduler fires batched rewrites. Periodic probes ask for the early FACT. Metrics per turn: context length, prompt/generated tokens, wall-clock, gen tokens/s. Early vs late windows express the paper claim that smaller context improves late-session quality and generation performance vs raw append-only.

- Model: `Qwen3.8-27B-4bit` · Hardware: `apple-m5-max` · Live: true
- Sessions/arm: 2 · Turns: 20 · Probe every: 4
- Early window: turns 1–6 · Late window: turns 13–20


## Figures

![Late probe accuracy](figures/lh_late_probe_accuracy.svg)

![Late avg context](figures/lh_late_context.svg)

![Late steady wall-clock](figures/lh_late_steady_wall.svg)

![Late steady gen tok/s](figures/lh_late_tok_s.svg)

![Context vs turn](figures/lh_context_vs_turn.svg)

![Wall-clock vs turn](figures/lh_wall_vs_turn.svg)

## Late-session results (the paper-relevant window)

| Arm | Late probe accuracy | Late avg ctx | Late avg prompt | Late wall-s | Late steady wall-s | Late gen tok/s | Prefill/session | Rewrites/session |
|---|---|---|---|---|---|---|---|---|
| baseline | **0.0%** (0/4) | 5666 | 3408 | 3.96 | 3.96 | 1.13 | 0 | 0.0 |
| immediate-rewrite | **100.0%** (4/4) | 320 | 530 | 1.91 | 1.91 | 2.35 | 1772 | 20.0 |
| dynclm | **100.0%** (4/4) | 2024 | 1549 | 3.46 | 3.70 | 1.36 | 144 | 1.0 |

## Early vs late (DynCLM vs baseline)

| Window | Baseline probe% | DynCLM probe% | Baseline ctx | DynCLM ctx | Baseline wall-s | DynCLM wall-s | Baseline tok/s | DynCLM tok/s |
|---|---|---|---|---|---|---|---|---|
| early | 0.0 | 100.0 | 1248 | 1248 | 2.63 | 2.91 | 1.38 | 1.40 |
| late | 0.0 | 100.0 | 5666 | 2024 | 3.96 | 3.46 | 1.13 | 1.48 |

## DynCLM vs raw baseline (late-window metrics)

| Metric | Baseline | DynCLM | Δ | % change | Better? |
|---|---|---|---|---|---|
| accuracy_pct | 0.00 | 100.00 | +100.00 | +100.0% | yes |
| avg_context_tokens | 5666.00 | 2023.50 | -3642.50 | -64.3% | yes |
| peak_context_tokens | 6854.00 | 4822.00 | -2032.00 | -29.6% | yes |
| final_context_tokens | 5666.00 | 2023.50 | -3642.50 | -64.3% | yes |
| avg_dead_tokens | 0.00 | 0.00 | +0.00 | +0.0% | — |
| avg_live_tokens | 0.00 | 0.00 | +0.00 | +0.0% | — |
| avg_prefill_tokens | 0.00 | 144.00 | +144.00 | +100.0% | no |
| avg_prompt_tokens | 3407.50 | 1549.00 | -1858.50 | -54.5% | yes |
| avg_generated_tokens | 4.25 | 4.75 | +0.50 | +11.8% | no |
| avg_total_tokens | 3411.75 | 1553.75 | -1858.00 | -54.5% | yes |
| avg_rewrites | 0.00 | 1.00 | +1.00 | +100.0% | no |
| avg_dead_carried_turn_sum | 0.00 | 0.00 | +0.00 | +0.0% | — |
| avg_wall_clock_s | 3.96 | 3.70 | -0.26 | -6.5% | yes |
| total_wall_clock_s | 142.10 | 141.15 | -0.95 | -0.7% | yes |
| wall_clock_per_correct_s | 0.00 | 35.29 | +35.29 | +100.0% | no |
| generated_tokens_per_correct | 0.00 | 4.75 | +4.75 | +100.0% | no |
| prefill_tokens_per_correct | 0.00 | 144.00 | +144.00 | +100.0% | no |

## Headlines

- **Late quality vs baseline:** +100.0 pp (0.0% → 100.0%)
- **Late context vs baseline:** -64.3% (5666 → 2024)
- **Late wall-clock vs baseline (all late turns):** -12.7% (3.96s → 3.46s)
- **Late steady wall-clock vs baseline (exclude rewrite turns):** -6.5% (3.96s → 3.70s)
- **Late steady gen tok/s vs baseline:** +20.7% (1.13 → 1.36)
- **Immediate-rewrite late steady wall vs baseline:** -51.8% (3.96s → 1.91s) — upper bound if N is high

Note: gen tok/s uses completion_tokens/wall_clock (includes prefill). Steady-state excludes rewrite-event turns so generation-under-context is not mixed with rewrite spikes.
