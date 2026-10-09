# DynCLM Quality Report

Generated: 2026-10-09T00:46:50Z

- Model: `Qwen3.8-27B-4bit`
- Hardware: `apple-m5-max`
- Live oMLX: true
- Tasks: 18 (ground-truth graded)

## Quality (accuracy) — primary metric

This is **not** the 38.6% re-prefill figure. That is cost. Below is graded answer accuracy.

| Arm | Accuracy | Δ vs baseline (pp) | Δ vs immediate (pp) | Avg re-prefill | Avg rewrites |
|---|---|---|---|---|---|
| baseline | **94.4%** (17/18) | +0.0 | -5.6 | 0 | 0.00 |
| immediate-rewrite | **100.0%** (18/18) | +5.6 | +0.0 | 0 | 1.00 |
| dynclm | **100.0%** (18/18) | +5.6 | +0.0 | 0 | 0.00 |

### DynCLM quality change (headline)

- vs raw baseline: **+5.6 percentage points** (94.4% → 100.0%)
- vs immediate rewrite: **+0.0 percentage points** (100.0% → 100.0%)
- cost (re-prefill cut vs immediate): **0.0%**

### By category

| Category | baseline | immediate | dynclm |
|---|---|---|---|
| coding | 100.0% | 100.0% | 100.0% |
| constraint | 66.7% | 100.0% | 100.0% |
| recovery | 100.0% | 100.0% | 100.0% |
| retention | 100.0% | 100.0% | 100.0% |

## Interpretation

DynCLM **improved** quality vs baseline by 5.6 pp on this suite.
