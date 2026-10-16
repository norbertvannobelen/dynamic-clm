# DynCLM Interim Report (Phase A Gate)

Generated: 2026-10-15T21:44:35Z

**Gate verdict: PASS - Phase B continues automatically**

## Setup

- Hardware: `apple-m5-max`
- Model: `Qwen3.8-27B-4bit`
- Sessions/arm: 16 × 40 turns
- Live oMLX model: true

## Calibration (M5 Max)

| Metric | Value |
|---|---|
| r (ms/token) | 2.787109 |
| k_d (ms/token/turn) | 0.03761292 |
| C_fixed (ms) | 1111.000 |
| ρ = C_fixed/k_d | 29537.727 |
| Notes | microbenchmark via oMLX chat completions wall-clock |

## Per-arm results

| Arm | Success% | Avg ctx | Peak ctx | Final ctx | Prefill | Prompt | Generated | Wall-s | Rewrites |
|---|---|---|---|---|---|---|---|---|---|
| baseline | 100.0 | 11785 | 23482 | 23482 | 0 | 99579 | 4516 | 371.72 | 0.00 |
| immediate-rewrite | 100.0 | 3905 | 8369 | 7585 | 17571 | 87609 | 4180 | 339.92 | 17.12 |
| dynclm | 100.0 | 5163 | 12657 | 10562 | 5010 | 90913 | 4418 | 359.63 | 4.88 |

## DynCLM vs raw baseline (primary comparison)

| Metric | Baseline | DynCLM | Δ | % change | Better? |
|---|---|---|---|---|---|
| accuracy_pct | 100.00 | 100.00 | +0.00 | +0.0% | yes |
| avg_context_tokens | 11784.95 | 5163.04 | -6621.91 | -56.2% | yes |
| peak_context_tokens | 23481.81 | 12657.25 | -10824.56 | -46.1% | yes |
| final_context_tokens | 23481.81 | 10562.31 | -12919.50 | -55.0% | yes |
| avg_dead_tokens | 16557.25 | 3103.94 | -13453.31 | -81.3% | yes |
| avg_live_tokens | 6924.56 | 7458.38 | +533.81 | +7.7% | no |
| avg_prefill_tokens | 0.00 | 5009.56 | +5009.56 | +100.0% | no |
| avg_prompt_tokens | 99578.75 | 90912.62 | -8666.12 | -8.7% | yes |
| avg_generated_tokens | 4515.88 | 4418.44 | -97.44 | -2.2% | yes |
| avg_total_tokens | 104094.62 | 95331.06 | -8763.56 | -8.4% | yes |
| avg_rewrites | 0.00 | 4.88 | +4.88 | +100.0% | no |
| avg_dead_carried_turn_sum | 347725.44 | 79867.50 | -267857.94 | -77.0% | yes |
| avg_wall_clock_s | 371.72 | 359.63 | -12.09 | -3.3% | yes |
| total_wall_clock_s | 5947.58 | 5754.13 | -193.45 | -3.3% | yes |
| wall_clock_per_correct_s | 371.72 | 359.63 | -12.09 | -3.3% | yes |
| generated_tokens_per_correct | 4515.88 | 4418.44 | -97.44 | -2.2% | yes |
| prefill_tokens_per_correct | 0.00 | 5009.56 | +5009.56 | +100.0% | no |

## Gate criteria G1–G4

- **G1 PASS (mechanism)**: parse_rate=1.000 rewrites=4.88 shrink_err=0.0% - PASS
- **G2 PASS (compliance)**: tail_marker_compliance=100.0% - PASS
- **G3 PASS (quality)**: dyn=100.0% baseline=100.0% immediate=100.0% - PASS
- **G4 PASS (cost)**: prefill_reduction=71.5% dyn_avg=5010 imm_avg=17571 - PASS

## oMLX cache probe (SCR readiness)

- `/cache` → 404 Not Found
- `/v1/kv` → 404 Not Found
- `/health` → 200 OK
- `/v1/health` → 404 Not Found
- `/v1/cache` → 404 Not Found

## Gaps

- No hybrid linear-attention model installed; SCR recurrent-state path not exercised.
- Physical KV relocation uses LogicalCacheBackend accounting (oMLX lacks span-level KV APIs).
