# DynCLM SWE-Distill Report

Generated: 2026-10-13T20:08:08Z

**Disclaimer:** SWE-Bench-**inspired** distill — not an official SWE-Bench resolve-rate evaluation.

- Model: `Qwen3.8-27B-4bit` · Hardware: `apple-m5-max` · Live: true
- max_context: **16384** · turns/task: **28** · noise_pad: **3398** chars
- Arms: Baseline+compact · DynCLM · CLM (immediate-rewrite)

## Methodology

SWE-Distill (realistic): SWE-Bench-Lite-inspired multi-turn coding quality + latency.
Shared max_context forces reclaim. Arms: baseline-compact (Codex-style rolling
summary: keep sys + recent raw; fold older spans into a budget-capped
[CONTEXT_SUMMARY] with early-requirements quota + newest-preferred notes;
re-prefill), DynCLM (soft-ignore + deferred rewrite), CLM/immediate-rewrite
(soft-ignore + rewrite every turn). Turn 1 plants an authoritative [SPEC] with a
real correct fix; later turns append plausible wrong SPECs with noise ≳ 1.4×
max_context. Instruction: prefer the most recent non-ignored SPEC. Chat serving
uses ContextTextForServing: dead spans are stubbed (no body) so DynCLM/CLM
prompts shrink without waiting for Rewrite — matching oMLX Chat (no KV
soft-ignore). Final probe grades ANSWER: <fragment>. Not official SWE-Bench.
Paper-adjacent contrast: lossy summary retention vs selective soft-invalidation,
not blind FIFO drop.

## Executive verdict

Context: **final live** = decode-relevant; **final total** = live+dead footprint; **final prompt** = oMLX probe `prompt_tokens` (marker arms still serialize dead spans). Wall includes reclaim turns; prefer live ctx over prompt when comparing DynCLM efficiency.

| Arm | Accuracy | Correct/N | Avg final live | Avg final total | Avg final prompt | Peak live | Avg reclaim events | Avg prefill | Total wall (s) | Steady wall/task (s) |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| Baseline+compact | 50.0% | 3/6 | 17540 | 17540 | 12013 | 16385 | 7.3 | 114387 | 3256.7 | 189.9 |
| DynCLM | 100.0% | 6/6 | 1939 | 8401 | 2524 | 2107 | 2.0 | 1238 | 1222.4 | 189.6 |
| CLM | 100.0% | 6/6 | 1064 | 1117 | 1454 | 3661 | 28.0 | 5074 | 1572.6 | 262.1 |

DynCLM accuracy Δ vs Baseline+compact: **+50.0 pp**. CLM: **+50.0 pp**.
DynCLM final live ctx vs Baseline+compact: **-88.9%**. CLM: **-93.9%**.

## Vs Baseline+compact

| Metric | DynCLM | CLM |
|---|---|---|
| Accuracy Δ (pp) | +50.0 | +50.0 |
| Final live ctx % | -88.9% | -93.9% |
| Final total ctx % | -52.1% | -93.6% |
| Final prompt % | -79.0% | -87.9% |
| Total wall % | -62.5% | -51.7% |
| Steady wall % | -0.2% | +38.0% |
| Prefill Δ (avg) | -113149 | -109313 |

## Per-task breakdown

| Instance | Expected | Baseline+compact | DynCLM | CLM | Ctx B/D/C |
|---|---|:-:|:-:|:-:|---|
| `astropy__units-compose-distill-06` | `simplify(num, den, unit.scale)` | FAIL | PASS | PASS | 18732 / 1863 / 554 |
| `django__queryset-exists-distill-01` | `self.query.has_results(using=self.db)` | FAIL | PASS | PASS | 16431 / 2158 / 3661 |
| `matplotlib__tight-layout-distill-04` | `if layout engine is not None: return` | PASS | PASS | PASS | 16438 / 2023 / 535 |
| `psf__requests-timeout-distill-02` | `timeout tuple must be (connect, read)` | FAIL | PASS | PASS | 18655 / 1762 / 553 |
| `pytest-dev__mark-parametrize-distill-05` | `ids callable must return str` | PASS | PASS | PASS | 18543 / 1923 / 546 |
| `sympy__is-polynomial-distill-03` | `except PolynomialError: return False` | PASS | PASS | PASS | 16441 / 1905 / 534 |

## Failure analysis

### Baseline+compact failures

- `astropy__units-compose-distill-06`/baseline-compact: got `We need answer user's current request. Need parse entire conversation. User init…` (spec_survived=true, compacts=7, rewrites=0)
- `django__queryset-exists-distill-01`/baseline-compact: got `ANSWER: len(list(self)) > 0` (spec_survived=true, compacts=9, rewrites=0)
- `psf__requests-timeout-distill-02`/baseline-compact: got `We need answer user's current request. Need parse conversation. User initial: ca…` (spec_survived=true, compacts=7, rewrites=0)

### DynCLM failures

_None._

### CLM failures

_None._

## Efficiency deep-dive

| Arm | Spec survive % | Avg compacts | Avg rewrites | Avg prefill |
|---|---:|---:|---:|---:|
| baseline-compact | 100.0 | 7.33 | 0.00 | 114387 |
| dynclm | 100.0 | 0.00 | 2.00 | 1238 |
| immediate-rewrite | 100.0 | 0.00 | 28.00 | 5074 |

Codex-style summarize baseline retains the SPEC in verbatim or summary form (spec survive 100.0%). DynCLM/CLM keep it via soft-invalidation (100.0% / 100.0%).

## Figures

Regenerate with `python3 scripts/gen_swe_distill_report.py --report <report.json>`.

- ![accuracy](figures/swe_probe_accuracy.svg)
- ![context](figures/swe_final_context.svg)
- ![wall](figures/swe_wall.svg)
- ![prefill](figures/swe_prefill.svg)
- ![per-task](figures/swe_per_task_accuracy.svg)

## Artifacts

- Arms present: baseline-compact, dynclm, immediate-rewrite
- Cells: 18
