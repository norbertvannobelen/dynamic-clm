# DynCLM Quality & Efficiency Report

Generated: 2026-10-09T00:55:14Z

- Model: `Qwen3.8-27B-4bit`
- Hardware: `apple-m5-max`
- Live oMLX: true
- Tasks: 18 (ground-truth graded)
- **Reference arm: raw baseline** (all deltas are vs baseline unless noted)

## Summary table (all arms)

| Arm | Accuracy | Avg ctx | Peak ctx | Prefill | Prompt | Generated | Wall-s | Rewrites |
|---|---|---|---|---|---|---|---|---|
| baseline | **94.4%** (17/18) | 162 | 298 | 0 | 406 | 6 | 0.95 | 0.00 |
| immediate-rewrite | **100.0%** (18/18) | 162 | 298 | 0 | 548 | 6 | 1.35 | 1.00 |
| dynclm | **100.0%** (18/18) | 162 | 298 | 0 | 548 | 6 | 1.37 | 0.00 |

## DynCLM vs raw baseline (full comparison)

| Metric | Baseline | DynCLM | Δ | % change | Better than baseline? |
|---|---|---|---|---|---|
| accuracy_pct | 94.44 | 100.00 | +5.56 | +5.9% | yes |
| avg_context_tokens | 161.61 | 161.61 | +0.00 | +0.0% | yes |
| peak_context_tokens | 298.00 | 298.00 | +0.00 | +0.0% | yes |
| final_context_tokens | 161.61 | 161.61 | +0.00 | +0.0% | yes |
| avg_dead_tokens | 0.00 | 146.56 | +146.56 | +100.0% | no |
| avg_live_tokens | 161.61 | 15.06 | -146.56 | -90.7% | yes |
| avg_prefill_tokens | 0.00 | 0.00 | +0.00 | +0.0% | — |
| avg_prompt_tokens | 405.72 | 548.28 | +142.56 | +35.1% | no |
| avg_generated_tokens | 5.89 | 5.89 | +0.00 | +0.0% | yes |
| avg_total_tokens | 411.61 | 554.17 | +142.56 | +34.6% | no |
| avg_rewrites | 0.00 | 0.00 | +0.00 | +0.0% | — |
| avg_dead_carried_turn_sum | 0.00 | 0.00 | +0.00 | +0.0% | — |
| avg_wall_clock_s | 0.95 | 1.37 | +0.42 | +44.1% | no |
| total_wall_clock_s | 17.13 | 24.68 | +7.55 | +44.1% | no |
| wall_clock_per_correct_s | 1.01 | 1.37 | +0.36 | +36.1% | no |
| generated_tokens_per_correct | 6.24 | 5.89 | -0.35 | -5.6% | yes |
| prefill_tokens_per_correct | 0.00 | 0.00 | +0.00 | +0.0% | — |

## Immediate-rewrite vs raw baseline

| Metric | Baseline | Immediate | Δ | % change | Better than baseline? |
|---|---|---|---|---|---|
| accuracy_pct | 94.44 | 100.00 | +5.56 | +5.9% | yes |
| avg_context_tokens | 161.61 | 161.61 | +0.00 | +0.0% | yes |
| peak_context_tokens | 298.00 | 298.00 | +0.00 | +0.0% | yes |
| final_context_tokens | 161.61 | 15.06 | -146.56 | -90.7% | yes |
| avg_dead_tokens | 0.00 | 146.56 | +146.56 | +100.0% | no |
| avg_live_tokens | 161.61 | 15.06 | -146.56 | -90.7% | yes |
| avg_prefill_tokens | 0.00 | 0.00 | +0.00 | +0.0% | — |
| avg_prompt_tokens | 405.72 | 548.28 | +142.56 | +35.1% | no |
| avg_generated_tokens | 5.89 | 5.89 | +0.00 | +0.0% | yes |
| avg_total_tokens | 411.61 | 554.17 | +142.56 | +34.6% | no |
| avg_rewrites | 0.00 | 1.00 | +1.00 | +100.0% | no |
| avg_dead_carried_turn_sum | 0.00 | 0.00 | +0.00 | +0.0% | — |
| avg_wall_clock_s | 0.95 | 1.35 | +0.40 | +42.3% | no |
| total_wall_clock_s | 17.13 | 24.38 | +7.24 | +42.3% | no |
| wall_clock_per_correct_s | 1.01 | 1.35 | +0.35 | +34.4% | no |
| generated_tokens_per_correct | 6.24 | 5.89 | -0.35 | -5.6% | yes |
| prefill_tokens_per_correct | 0.00 | 0.00 | +0.00 | +0.0% | — |

## Headlines

- **Quality vs baseline:** +5.6 pp (94.4% → 100.0%)
- **Quality vs immediate rewrite:** +0.0 pp
- **Wall-clock vs baseline:** +44.1% (0.95s → 1.37s avg)
- **Prompt tokens vs baseline:** +35.1% (406 → 548 avg)
- **Generated tokens vs baseline:** +0.0% (6 → 6 avg)
- **Context length (avg) vs baseline:** +0.0% (162 → 162)
- **Rewrite prefill vs baseline:** 0 → 0 (baseline has none)
- **Rewrite prefill vs immediate:** cut 0.0%

### By category (accuracy)

| Category | baseline | immediate | dynclm |
|---|---|---|---|
| coding | 100.0% | 100.0% | 100.0% |
| constraint | 66.7% | 100.0% | 100.0% |
| recovery | 100.0% | 100.0% | 100.0% |
| retention | 100.0% | 100.0% | 100.0% |

## Interpretation

DynCLM **improved** quality vs baseline by 5.6 pp on this suite.

Efficiency vs baseline depends on the metric: rewrite prefill is an *extra* cost vs raw append-only; prompt/context/wall-clock deltas above show whether the end-to-end request is leaner or heavier.
