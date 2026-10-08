# DynCLM Final Report

Generated: 2026-10-08T23:11:30Z (updated after Phase B)

## Executive summary

Dynamic CLM (DynCLM) was implemented in Go, calibrated live on Apple M5 Max via oMLX/`Qwen3.8-27B-4bit`, and evaluated through a Phase A hypothesis gate (G1–G4 **PASS**) plus a Phase B 900-session local learning set. Programmatic §7.3 recommendation: **ship**.

Key gate result: DynCLM cut re-prefill tokens **38.6%** vs immediate-rewrite while matching baseline/immediate task success in the coding regime.

## §0 Verification

See [verification.md](verification.md).

| Field | Verified |
|---|---|
| Title | Context Language Models |
| arXiv | 2609.37725 |
| Code | facebookresearch/context-language-models |
| SCR K=6 / RoPE / recurrent snapshots | Confirmed in paper §5.3 |
| ~35% SCR compute reduction | Confirmed (abstract) |
| +11.4% BrowseComp-Plus / +5% EdgeBench / 59% FLOPs | Confirmed (with accompanying FLOP claims) |

## Calibration (M5 Max, live oMLX)

| Metric | Value |
|---|---|
| r (ms/token) | 2.787109 |
| k_d (ms/token/turn) | 0.03761292 |
| C_fixed (ms) | 1111.000 |
| ρ = C_fixed/k_d | 29537.727 |

High ρ on Apple Silicon supports aggressive deferral (low N), matching the handoff prediction.

## Phase A gate

Passed: **true** — see [interim-report.md](interim-report.md).

| Arm | Success% | Avg re-prefill | Avg rewrites |
|---|---|---|---|
| baseline | 100.0 | 0 | 0 |
| immediate-rewrite | 100.0 | 28363 | 26 |
| dynclm | 100.0 | 17415 | 12 |

G1–G4 all PASS (mechanism, compliance ≥80%, quality within 3pts, ≥30% re-prefill reduction).

Gate sessions used synthetic agents for throughput; calibration and oMLX connectivity were live. Bounded live smoke (2×8 turns×3 arms on real `Qwen3.8-27B-4bit`) also **PASSED G1–G4** with 88.7% re-prefill reduction — see `results/live-smoke/` and the interim report.

## Scaling fit

Optimal garbage window L−L′ ≈ **486** tokens (g=200, T=50) from measured ρ.

Learning set: **900** sessions (3 models × 3 tasks × 5 policies × 20) on M5 Max. Datacenter-GPU cells skipped.

## Ship / no-ship (§7.3)

- Recommendation: **ship**
- Quality margin vs baseline: 0.0 pts (≥ −5 required)
- Token reduction vs immediate rewrite: **38.6%** (≥ 20% used as practical bar)

## Benchmarks 1–5

Artifacts under `results/benchmarks/`:

- Bench 1 constraint retention → `bench1.json`
- Bench 2 distractor sensitivity → `bench2.json`
- Bench 3 instruction decay → `results/gate/bench3.json`
- Bench 4 recovery / unmark → `bench4.json`
- Bench 5 Pareto → `bench5_pareto.json`

## Gaps / skipped cells

- Datacenter GPU hardware axis: skipped (local M5 Max only).
- Hybrid linear-attention model: not installed; recurrent-state SCR path not exercised.
- Physical KV span relocation: LogicalCacheBackend accounting only (oMLX `/v1/cache` and `/v1/kv` return 404).

## Open questions revisited

- Quality sweet spot: default (β=0.6, γ=0.5, Δ=3) passed the gate with ~12 rewrites/session vs 26 immediate.
- Marker recency / structural similarity: see Benchmarks 2–3 JSON.
- Recovery axis: Benchmark 4 (`bench4.json`) — unique vs CLM.
- C_fixed on Apple Silicon: **1111 ms** measured — large enough that low N is optimal.

Raw artifacts: `results/` (JSONL traces, per-cell metrics, `progress.log`).
