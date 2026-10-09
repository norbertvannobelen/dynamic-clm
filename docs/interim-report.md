# DynCLM Interim Report (Phase A Gate)

Generated: 2026-10-08T23:11:21Z

**Gate verdict: PASS — Phase B continues automatically**

## Setup

- Hardware: `apple-m5-max`
- Model: `Qwen3.8-27B-4bit`
- Sessions/arm: 20 × 50 turns (synthetic agent for throughput)
- Live oMLX model: false for full grid; true for bounded smoke (below)
- Serving: oMLX `http://127.0.0.1:8000`

## Calibration (M5 Max, live oMLX)

| Metric | Value |
|---|---|
| r (ms/token) | 2.787109 |
| k_d (ms/token/turn) | 0.03761292 |
| C_fixed (ms) | 1111.000 |
| ρ = C_fixed/k_d | 29537.727 |
| Notes | microbenchmark via oMLX chat completions wall-clock |

## Per-arm results (full gate, 20×50)

| Arm | Success% | Avg re-prefill | Avg rewrites | Parse rate | Compliance% |
|---|---|---|---|---|---|
| baseline | 100.0 | 0 | 0.00 | 1.000 | 100.0 |
| immediate-rewrite | 100.0 | 28363 | 26.00 | 1.000 | 100.0 |
| dynclm | 100.0 | 17415 | 12.00 | 1.000 | 100.0 |

## Gate criteria G1–G4

- **G1 PASS (mechanism)**: parse_rate=1.000 rewrites=12.00 shrink_err=0.0% — PASS
- **G2 PASS (compliance)**: tail_marker_compliance=100.0% — PASS
- **G3 PASS (quality)**: dyn=100.0% baseline=100.0% immediate=100.0% — PASS
- **G4 PASS (cost)**: prefill_reduction=38.6% dyn_avg=17415 imm_avg=28363 — PASS

## Live oMLX smoke (2 sessions × 8 turns × 3 arms)

Confirms the same criteria against the real `Qwen3.8-27B-4bit` endpoint (~12 minutes wall-clock):

| Arm | Success% | Avg re-prefill | Avg rewrites |
|---|---|---|---|
| baseline | 100.0 | 0 | 0 |
| immediate-rewrite | 100.0 | 1156 | 6.5 |
| dynclm | 100.0 | 131 | 1.0 |

Live G1–G4: all PASS (prefill reduction **88.7%** on the short horizon). Artifacts: `results/live-smoke/`.

## oMLX cache probe (SCR readiness)

- `/health` → 200 OK
- `/v1/health` → 404 Not Found
- `/v1/cache` → 404 Not Found
- `/cache` → 404 Not Found
- `/v1/kv` → 404 Not Found

No span-level KV control API exposed; LogicalCacheBackend accounting used.

## Live graded quality suite (authoritative quality metric)

See [quality-report.md](quality-report.md). Re-run: `bin/dynclm-quality -model Qwen3.8-27B-4bit`.

| Arm | Accuracy | Δ vs baseline | Δ vs immediate |
|---|---|---|---|
| baseline | 94.4% (17/18) | — | −5.6 pp |
| immediate-rewrite | 100.0% (18/18) | +5.6 pp | — |
| dynclm | 100.0% (18/18) | **+5.6 pp** | **+0.0 pp** |

**Do not confuse with the 38.6% re-prefill cut** — that is cost, not quality. Quality change vs baseline is **+5.6 percentage points** on this 18-task live suite (tied with immediate rewrite). Not a SWE-bench paper number.

## Gaps

- No hybrid linear-attention model installed; SCR recurrent-state path not exercised.
- Physical KV relocation uses LogicalCacheBackend accounting (oMLX lacks span-level KV APIs).
- Gate G3 used a trivial success token; use the graded quality suite above for quality claims.

