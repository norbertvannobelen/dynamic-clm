# DynCLM Interim Report (Phase A Gate)

Generated: 2026-10-10T17:29:26Z

**Gate verdict: PASS - Phase B continues automatically**

## Setup

- Hardware: `apple-m5-max`
- Model: `Qwen3.8-27B-4bit`
- Sessions/arm: 8 × 40 turns
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
| baseline | 100.0 | 11714 | 23009 | 23009 | 0 | 79161 | 4755 | 402.32 | 0.00 |
| immediate-rewrite | 100.0 | 3995 | 8556 | 7856 | 18504 | 91271 | 4386 | 352.47 | 17.00 |
| dynclm | 100.0 | 5410 | 12630 | 11423 | 5340 | 96850 | 4504 | 365.94 | 5.00 |

## DynCLM vs raw baseline (primary comparison)

| Metric | Baseline | DynCLM | Δ | % change | Better? |
|---|---|---|---|---|---|
| accuracy_pct | 100.00 | 100.00 | +0.00 | +0.0% | yes |
| avg_context_tokens | 11714.10 | 5410.40 | -6303.70 | -53.8% | yes |
| peak_context_tokens | 23008.88 | 12630.38 | -10378.50 | -45.1% | yes |
| final_context_tokens | 23008.88 | 11423.12 | -11585.75 | -50.4% | yes |
| avg_dead_tokens | 14338.88 | 4504.62 | -9834.25 | -68.6% | yes |
| avg_live_tokens | 8670.00 | 6918.50 | -1751.50 | -20.2% | yes |
| avg_prefill_tokens | 0.00 | 5340.50 | +5340.50 | +100.0% | no |
| avg_prompt_tokens | 79161.38 | 96849.88 | +17688.50 | +22.3% | no |
| avg_generated_tokens | 4755.00 | 4503.50 | -251.50 | -5.3% | yes |
| avg_total_tokens | 83916.38 | 101353.38 | +17437.00 | +20.8% | no |
| avg_rewrites | 0.00 | 5.00 | +5.00 | +100.0% | no |
| avg_dead_carried_turn_sum | 299412.75 | 86175.25 | -213237.50 | -71.2% | yes |
| avg_wall_clock_s | 402.32 | 365.94 | -36.39 | -9.0% | yes |
| total_wall_clock_s | 3218.59 | 2927.50 | -291.09 | -9.0% | yes |
| wall_clock_per_correct_s | 402.32 | 365.94 | -36.39 | -9.0% | yes |
| generated_tokens_per_correct | 4755.00 | 4503.50 | -251.50 | -5.3% | yes |
| prefill_tokens_per_correct | 0.00 | 5340.50 | +5340.50 | +100.0% | no |

## Gate criteria G1–G4

- **G1 PASS (mechanism)**: parse_rate=1.000 rewrites=5.00 shrink_err=0.0% - PASS
- **G2 PASS (compliance)**: tail_marker_compliance=100.0% - PASS
- **G3 PASS (quality)**: dyn=100.0% baseline=100.0% immediate=100.0% - PASS
- **G4 PASS (cost)**: prefill_reduction=71.1% dyn_avg=5340 imm_avg=18504 - PASS

## oMLX cache probe (SCR readiness)

- `/v1/kv` → 404 Not Found
- `/health` → 200 OK
- `/v1/health` → 404 Not Found
- `/v1/cache` → 404 Not Found
- `/cache` → 404 Not Found

## Gaps

- No hybrid linear-attention model installed; SCR recurrent-state path not exercised.
- Physical KV relocation uses LogicalCacheBackend accounting (oMLX lacks span-level KV APIs).
