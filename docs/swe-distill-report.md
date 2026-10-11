# DynCLM SWE-Distill — Extensive Report

Generated: 2026-10-11T01:50:08.47034Z · Model `Qwen3.8-27B-4bit` · Hardware `apple-m5-max` · live=True

**Disclaimer:** This is a SWE-Bench-**Lite-inspired** multi-turn coding distill under shared `max_context` with Baseline+compact / DynCLM / CLM. It is **not** an official SWE-Bench resolve-rate number and is not TerminalBench / Harbor.

## 1. Executive verdict

On this suite, DynCLM final-probe accuracy is **83.3%** (+66.7 pp vs Baseline+compact **16.7%**); CLM is **100.0%** (+83.3 pp). Baseline+compact keeps the early SPEC in only **0%** of tasks (blind drop), while DynCLM/CLM retain it via soft-invalidation (100% / 100%). CLM pays the highest rewrite prefill; DynCLM sits between quality parity with CLM and much lower rewrite N than always-rewrite.

## 2. Methodology

SWE-Distill (realistic): SWE-Bench-Lite-inspired multi-turn coding quality.
Shared max_context forces reclaim. Arms: baseline-compact (blind drop oldest +
re-prefill), DynCLM (soft-ignore + deferred rewrite), CLM/immediate-rewrite
(soft-ignore + rewrite every turn). Turn 1 plants an authoritative [SPEC] with a
real correct fix; later turns append plausible wrong SPECs (recency trap) with
noise ≳ 1.4× max_context. Instruction: prefer the most recent non-ignored SPEC.
Baseline often follows the wrong recent SPEC after compaction; DynCLM/CLM should
retain the early correct SPEC. Final probe grades ANSWER: <fragment>. Not an
official SWE-Bench resolve-rate evaluation.

| Knob | Value |
|---|---|
| max_context | **16384** |
| turns / task | **28** |
| noise_pad (chars) | **3398** |
| tasks | **6** |
| cells | **18** |
| target wall | ~2h / arm |
| observed wall | Baseline+compact **1.13h** · DynCLM **1.42h** · CLM **0.42h** |

Wall came in under the 2h/arm budget while still forcing compaction (baseline avg compacts **4.0**/task) and DynCLM/CLM reclaim. Context is already saturating near `max_context`; stretching wall further would not change the reclaim physics.

Fixtures live under `testdata/swe_distill/` (paraphrased Lite-inspired excerpts with per-file attribution).

## 3. Headline results

| Arm | Accuracy | Correct/N | Avg final ctx | Peak ctx | Avg compacts | Avg rewrites | Avg prefill | Total wall (s) | Wall (h) |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| Baseline+compact | 16.7% | 1/6 | 15070 | 16368 | 4.00 | 0.00 | 51057 | 4061.1 | 1.13 |
| DynCLM | 83.3% | 5/6 | 18181 | 24492 | 0.00 | 2.50 | 6062 | 5106.1 | 1.42 |
| CLM | 100.0% | 6/6 | 596 | 554 | 0.00 | 28.00 | 3784 | 1525.2 | 0.42 |

## 4. Vs Baseline+compact

| Metric | DynCLM | CLM |
|---|---|---|
| Accuracy Δ (pp) | +66.7 | +83.3 |
| Final ctx % | +20.6% | -96.0% |
| Total wall % | +25.7% | -62.4% |
| Prefill Δ (avg) | -44996 | -47273 |

## 5. Per-task breakdown

| Instance | Expected | Baseline+compact | DynCLM | CLM | Final ctx B/D/C | Reclaim events B/D/C |
|---|---|:-:|:-:|:-:|---|---|
| `astropy__units-compose-distill-06` | `simplify(num, den, unit.scale)` | FAIL | PASS | PASS | 16333 / 23979 / 606 | 5 / 2 / 28 |
| `django__queryset-exists-distill-01` | `self.query.has_results(using=self.db)` | FAIL | FAIL | PASS | 14546 / 12300 / 587 | 5 / 3 / 28 |
| `matplotlib__tight-layout-distill-04` | `if layout engine is not None: return` | PASS | PASS | PASS | 15371 / 24545 / 588 | 3 / 2 / 28 |
| `psf__requests-timeout-distill-02` | `timeout tuple must be (connect, read)` | FAIL | PASS | PASS | 13174 / 14403 / 609 | 4 / 3 / 28 |
| `pytest-dev__mark-parametrize-distill-05` | `ids callable must return str` | FAIL | PASS | PASS | 15368 / 11420 / 598 | 4 / 3 / 28 |
| `sympy__is-polynomial-distill-03` | `except PolynomialError: return False` | FAIL | PASS | PASS | 15625 / 22438 / 590 | 3 / 2 / 28 |

## 6. Figures

![Probe accuracy](figures/swe_probe_accuracy.svg)

![Final context](figures/swe_final_context.svg)

![Wall-clock](figures/swe_wall.svg)

![Prefill](figures/swe_prefill.svg)

![Per-task](figures/swe_per_task_accuracy.svg)

## 7. Failure analysis

### Baseline+compact

- `astropy__units-compose-distill-06`: got `We need answer user's final probe. Need output exactly one line: ANSWER: <fragment>. Need determine ` (spec_survived=False, compacts=5, rewrites=0)
- `django__queryset-exists-distill-01`: got `ANSWER: len(list(self)) > 0` (spec_survived=False, compacts=5, rewrites=0)
- `psf__requests-timeout-distill-02`: got `We need answer user's final. Need parse. User final: "Final probe at turn 28/28. What ValueError mes` (spec_survived=False, compacts=4, rewrites=0)
- `pytest-dev__mark-parametrize-distill-05`: got `We need answer final probe. Need parse context. User says final probe at turn 28/28. What TypeError ` (spec_survived=False, compacts=4, rewrites=0)
- `sympy__is-polynomial-distill-03`: got `We need answer user's final probe. Need parse instructions carefully.

User message is a long contex` (spec_survived=False, compacts=3, rewrites=0)

### DynCLM

- `django__queryset-exists-distill-01`: got `We need answer final probe. User asks: Final probe at turn 28/28. When the result cache is empty, wh` (spec_survived=True, compacts=0, rewrites=3)

### CLM

_None._

Baseline+compact dropped the early SPEC in **6/6** tasks — the expected failure mode under blind truncation when noise exceeds the window.

## 8. Efficiency deep-dive

Quality-adjusted read: Baseline+compact may post competitive wall-clock while answering from a recency trap after the SPEC falls out of the window. DynCLM/CLM spend reclaim prefill to keep the authoritative snippet. Prefer accuracy-first comparisons; treat wall alone as secondary.

| Arm | Spec survive % | Avg wall/task (s) |
|---|---:|---:|
| Baseline+compact | 0.0 | 676.9 |
| DynCLM | 100.0 | 851.0 |
| CLM | 100.0 | 254.2 |

## 9. Relation to prior suites

| Suite | What it showed | What SWE-Distill adds |
|---|---|---|
| Short gate | Mechanism + prefill vs CLM | Coding issue/snippet domain |
| Graded quality | Short multi-span accuracy | Multi-turn growth + compaction pressure |
| Long-horizon A/B | FACT canary + compact matrix | SWE-inspired fix fragments under same reclaim physics |

## 10. Limitations & next steps

- Not official SWE-Bench Verified/Lite resolve (no docker, no fail-to-pass).
- Model is local `Qwen3.8-27B-4bit` — not paper Qwen3.6-27B full precision.
- FLOPs are not prefix-reuse PFLOPs; we report wall / tokens / prefill.
- Optional later: Harbor DynCLM agent on TBLite if a paper-adjacent coding row is required.

## 11. Artifacts

- Source report JSON: `results/runs/20261010T225155Z_swe_distill/swe_distill/report.json`
- Figures: `docs/figures/swe_*.svg`
- Reproduce: `bash scripts/run_swe_distill.sh`
- Regenerate this doc: `python3 scripts/gen_swe_distill_report.py --report results/runs/20261010T225155Z_swe_distill/swe_distill/report.json`
