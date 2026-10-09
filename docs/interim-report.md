# DynCLM Interim Report (Phase A Gate)

Generated: 2026-10-09T00:54:03Z

**Gate verdict: PASS — Phase B continues automatically**

## Setup

- Hardware: `apple-m5-max`
- Model: `Qwen3.8-27B-4bit`
- Sessions/arm: 20 × 50 turns
- Live oMLX model: false

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
| baseline | 100.0 | 13476 | 26385 | 26385 | 0 | 676050 | 454 | 0.00 | 0.00 |
| immediate-rewrite | 100.0 | 2409 | 4633 | 3244 | 28363 | 122706 | 454 | 0.00 | 26.00 |
| dynclm | 100.0 | 2853 | 4830 | 3707 | 17415 | 144915 | 454 | 0.00 | 12.00 |

## DynCLM vs raw baseline (primary comparison)

| Metric | Baseline | DynCLM | Δ | % change | Better? |
|---|---|---|---|---|---|
| accuracy_pct | 100.00 | 100.00 | +0.00 | +0.0% | yes |
| avg_context_tokens | 13475.64 | 2852.94 | -10622.70 | -78.8% | yes |
| peak_context_tokens | 26385.00 | 4830.00 | -21555.00 | -81.7% | yes |
| final_context_tokens | 26385.00 | 3707.00 | -22678.00 | -86.0% | yes |
| avg_dead_tokens | 23141.00 | 463.00 | -22678.00 | -98.0% | yes |
| avg_live_tokens | 3244.00 | 3244.00 | +0.00 | +0.0% | yes |
| avg_prefill_tokens | 0.00 | 17415.00 | +17415.00 | +100.0% | no |
| avg_prompt_tokens | 676050.00 | 144915.00 | -531135.00 | -78.6% | yes |
| avg_generated_tokens | 454.00 | 454.00 | +0.00 | +0.0% | yes |
| avg_total_tokens | 676504.00 | 145369.00 | -531135.00 | -78.5% | yes |
| avg_rewrites | 0.00 | 12.00 | +12.00 | +100.0% | no |
| avg_dead_carried_turn_sum | 589911.00 | 58776.00 | -531135.00 | -90.0% | yes |
| avg_wall_clock_s | 0.00 | 0.00 | -0.00 | -13.2% | yes |
| total_wall_clock_s | 0.01 | 0.01 | -0.00 | -13.2% | yes |
| wall_clock_per_correct_s | 0.00 | 0.00 | -0.00 | -13.2% | yes |
| generated_tokens_per_correct | 454.00 | 454.00 | +0.00 | +0.0% | yes |
| prefill_tokens_per_correct | 0.00 | 17415.00 | +17415.00 | +100.0% | no |

## Gate criteria G1–G4

- **G1 PASS (mechanism)**: parse_rate=1.000 rewrites=12.00 shrink_err=0.0% — PASS
- **G2 PASS (compliance)**: tail_marker_compliance=100.0% — PASS
- **G3 PASS (quality)**: dyn=100.0% baseline=100.0% immediate=100.0% — PASS
- **G4 PASS (cost)**: prefill_reduction=38.6% dyn_avg=17415 imm_avg=28363 — PASS

## oMLX cache probe (SCR readiness)

- `/health` → 200 OK
- `/v1/health` → 404 Not Found
- `/v1/cache` → 404 Not Found
- `/cache` → 404 Not Found
- `/v1/kv` → 404 Not Found

## Live graded quality vs baseline

See [quality-report.md](quality-report.md) (18 tasks, live `Qwen3.8-27B-4bit`).

| Metric | Baseline | DynCLM | vs baseline |
|---|---|---|---|
| Accuracy | 94.4% | 100% | **+5.6 pp** |
| Avg prompt tokens | 406 | 548 | +35% |
| Avg generated tokens | 6 | 6 | ~0% |
| Avg wall-clock | 0.95s | 1.37s | +44% |
| Avg context tokens | 162 | 162 | ~0% (single-shot) |

## Gaps

- No hybrid linear-attention model installed; SCR recurrent-state path not exercised.
- Physical KV relocation uses LogicalCacheBackend accounting (oMLX lacks span-level KV APIs).
- Gate wall-clock is synthetic (~0); use live quality wall-clock for timing, gate for context/prompt/prefill.
