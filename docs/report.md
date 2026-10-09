# DynCLM Final Report

Generated: 2026-10-09T00:47:00Z (updated with live quality suite)

## Executive summary

Dynamic CLM (DynCLM) was implemented in Go, calibrated live on Apple M5 Max via oMLX/`Qwen3.8-27B-4bit`, gated (G1–G4 **PASS**), and evaluated with a **ground-truth quality suite** separate from cost metrics.

| Metric | DynCLM result |
|---|---|
| **Quality vs raw baseline** | **+5.6 percentage points** (94.4% → 100.0% on 18 live graded tasks) |
| **Quality vs immediate rewrite** | **+0.0 pp** (tied at 100%) |
| **Cost: re-prefill vs immediate rewrite** | **−38.6%** (gate coding sessions) |

The 38.6% figure is **not** a quality improvement. It is a compute/cost reduction. Quality is reported in percentage points of graded accuracy.

## §0 Verification

See [verification.md](verification.md). CLM paper arXiv:2609.37725; SCR K=6 / RoPE / recurrent snapshots confirmed.

## Calibration (M5 Max, live oMLX)

| Metric | Value |
|---|---|
| r (ms/token) | 2.787109 |
| k_d (ms/token/turn) | 0.03761292 |
| C_fixed (ms) | 1111.000 |
| ρ = C_fixed/k_d | 29537.727 |

## Quality results (live, graded)

Full write-up: [quality-report.md](quality-report.md). Runner: `dynclm-quality`. Model: `Qwen3.8-27B-4bit` via oMLX. **18** tasks with known answers (retention, constraint, coding, recovery), including adversarial recency traps.

| Arm | Accuracy | Δ vs baseline | Δ vs immediate |
|---|---|---|---|
| baseline | 94.4% (17/18) | — | −5.6 pp |
| immediate-rewrite | 100.0% (18/18) | +5.6 pp | — |
| **dynclm** | **100.0% (18/18)** | **+5.6 pp** | **+0.0 pp** |

Interpretation: on this suite DynCLM **improves quality vs the raw baseline** and **matches** the immediate-rewrite (CLM-like) arm. Sample size is small (n=18); one task flip is 5.6 pp. This is **not** a published SWE-bench / BrowseComp-Plus result.

By category (DynCLM): coding 100%, constraint 100%, recovery 100%, retention 100%. Baseline slipped on adversarial constraint (recency trap without ignore markers).

## Cost results (gate)

| Arm | Avg re-prefill tokens | Avg rewrites |
|---|---|---|
| baseline | 0 | 0 |
| immediate-rewrite | 28363 | 26 |
| dynclm | 17415 | 12 |

DynCLM re-prefill **38.6% below** immediate rewrite. Live smoke (2×8 turns): **88.7%** below on a short horizon.

## Phase A gate

Passed: **true** — [interim-report.md](interim-report.md). G3 originally used a trivial success token; **authoritative quality** is the live graded suite above.

## Scaling fit

Optimal garbage window L−L′ ≈ **486** tokens (g=200, T=50). Learning set: 900 local sessions. Datacenter GPU skipped.

## Ship / no-ship (§7.3)

- Recommendation: **ship**
- Quality vs baseline: **+5.6 pp** (requirement: not below baseline − 5)
- Quality vs immediate rewrite: **tied**
- Re-prefill vs immediate rewrite: **−38.6%**

## Gaps

- Not SWE-bench Verified / BrowseComp-Plus / EdgeBench end-to-end (those paper suites were not reproduced here).
- n=18 live graded tasks — directional, not a large-N benchmark.
- No hybrid linear-attention model; LogicalCacheBackend only (oMLX has no span-level KV API).

## Open questions

- Larger-N quality curve vs deferral window length still open.
- Whether +5.6 pp holds on SWE-bench-style agent tasks is untested.

Raw artifacts: `results/quality/`, `results/gate/`, `results/live-smoke/`.
