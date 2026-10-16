# DynCLM SWE-Distill - Extensive Report

Generated: 2026-10-16T00:51:46.083612Z · Model `Qwen3.8-27B-4bit` · Hardware `apple-m5-max` · live=True

**Disclaimer:** This is a SWE-Bench-**Lite-inspired** multi-turn coding distill under shared `max_context` with Baseline+compact / DynCLM / CLM. It is **not** an official SWE-Bench resolve-rate number and is not TerminalBench / Harbor.

## 1. Executive verdict

On this suite, DynCLM final-probe accuracy is **83.3%** (+25.0 pp vs Baseline+compact **58.3%**); CLM is **100.0%** (+41.7 pp). Baseline+compact retains the early SPEC (verbatim or in rolling summary) in **100%** of tasks; DynCLM/CLM keep it via soft-invalidation (100% / 100%). CLM pays the highest rewrite prefill; DynCLM sits between quality parity with CLM and much lower rewrite N than always-rewrite.

Pre-registered statistics (docs/analysis-plan.md), task-level pass, one-sided Fisher exact: DynCLM vs Baseline+compact **p=0.185**; CLM vs Baseline+compact **p=0.019**. Wilson 95% CIs in §4.

**Read.** Baseline is Codex-style **lossy summarization**. Quote accuracy with `spec_survive` (**100%** here). Gaps with high survive are recency/confusion under summarization, not deleted SPECs. Not official SWE-Bench; not the same as graded quality (+5.6 pp vs raw with no window reclaim). See §2.

## 2. How to read Baseline+compact accuracy

This suite measures **reclaim under window pressure** against summarize compaction, not single-shot coding skill.

1. **Plant**: turn 1 inserts the correct fix as `[SPEC]`.
2. **Flood**: later turns append plausible wrong SPECs plus padding so tokens exceed `max_context` (~1.4×).
3. **Rolling summary compact**: keep `sys` + recent raw; fold older spans into `[CONTEXT_SUMMARY]`; re-prefill.
4. **Recency instruction**: without ignore markers, prefer the most recent SPEC.
5. **Probe**: grade `ANSWER: <fragment>` against turn-1 truth.

`spec_survive` counts the expected fragment still present (verbatim or in the summary). DynCLM/CLM: **100%** / **100%**. Baseline: **100%**. When survive is high and accuracy is lower, the gap is recency/confusion, not deletion of the fix.

## 3. Methodology

SWE-Distill (realistic): SWE-Bench-Lite-inspired multi-turn coding quality + latency.
Shared max_context forces reclaim. Arms: baseline-compact (Codex-style rolling
summary: keep sys + recent raw; fold older spans into a budget-capped
[CONTEXT_SUMMARY] with early-requirements quota + newest-preferred notes;
re-prefill), DynCLM (soft-ignore + deferred rewrite), CLM/immediate-rewrite
(soft-ignore + rewrite every turn). Turn 1 plants an authoritative [SPEC] with a
real correct fix; later turns append plausible wrong SPECs with noise ≳ 1.4×
max_context. Instruction: prefer the most recent non-ignored SPEC. Chat serving
uses ContextTextForServing: dead spans are stubbed (no body) so DynCLM/CLM
prompts shrink without waiting for Rewrite - matching oMLX Chat (no KV
soft-ignore). Final probe grades ANSWER: <fragment>. Not official SWE-Bench.
Contrast: lossy summary + recency vs selective soft-invalidation.

| Knob | Value |
|---|---|
| max_context | **16384** |
| turns / task | **28** |
| noise_pad (chars) | **3398** |
| tasks | **12** |
| cells | **36** |
| target wall | ~2h / arm |
| observed wall | Baseline+compact **1.26h** · DynCLM **0.57h** · CLM **0.45h** |

Wall came in under the 2h/arm budget while still forcing compaction (baseline avg compacts **7.2**/task) and DynCLM/CLM reclaim. Context is already saturating near `max_context`; stretching wall further would not change the reclaim physics.

Fixtures live under `testdata/swe_distill/` (paraphrased Lite-inspired excerpts with per-file attribution).

## 4. Headline results

Context columns: **final live** is decode-relevant tracker live tokens; **final total** is live+dead footprint; **final prompt** is oMLX `prompt_tokens` on the probe (marker arms still serialize ignored dead spans until rewrite). **Steady wall** excludes compact/rewrite turns when present.

| Arm | Accuracy [Wilson 95% CI] | Correct/N | Avg final live | Avg final total | Avg final prompt | Peak live | Avg compacts | Avg rewrites | Avg prefill | Total wall (s) | Steady wall/task (s) | Wall (h) |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| Baseline+compact | 58.3% [32.0, 80.7] | 7/12 | 17699 | 17699 | 12069 | 16385 | 7.17 | 0.00 | 111497 | 4543.9 | 139.0 | 1.26 |
| DynCLM | 83.3% [55.2, 95.3] | 10/12 | 2173 | 8822 | 2539 | 2159 | 0.00 | 2.00 | 1292 | 2047.5 | 159.4 | 0.57 |
| CLM | 100.0% [75.8, 100.0] | 12/12 | 1270 | 1270 | 1601 | 2163 | 0.00 | 28.00 | 8837 | 1607.5 | 134.0 | 0.45 |

## 5. Vs Baseline+compact

| Metric | DynCLM | CLM |
|---|---|---|
| Accuracy Δ (pp) | +25.0 | +41.7 |
| Final live ctx % | -87.7% | -92.8% |
| Final total ctx % | -50.2% | -92.8% |
| Final prompt % | -79.0% | -86.7% |
| Total wall % | -54.9% | -64.6% |
| Steady wall % | +14.7% | -3.6% |
| Prefill Δ (avg) | -110205 | -102659 |

## 6. Per-task breakdown

| Instance | Expected | Baseline+compact | DynCLM | CLM | Final ctx B/D/C | Reclaim events B/D/C |
|---|---|:-:|:-:|:-:|---|---|
| `astropy__units-compose-distill-06` | `simplify(num, den, unit.scale)` | PASS | PASS | PASS | 18595 / 2034 / 1569 | 8 / 2 / 28 |
| `django__queryset-exists-distill-01` | `self.query.has_results(using=self.db)` | FAIL | PASS | PASS | 16387 / 1891 / 722 | 7 / 2 / 28 |
| `matplotlib__tight-layout-distill-04` | `if layout engine is not None: return` | PASS | PASS | PASS | 16438 / 1942 / 906 | 9 / 2 / 28 |
| `numpy__clip-nan-propagate-distill-07` | `np.minimum(np.maximum(a, a_min), a_max)` | PASS | PASS | PASS | 18483 / 2152 / 2040 | 6 / 2 / 28 |
| `pandas__ffill-limit-distill-08` | `self.fillna(method="ffill", limit=limit)` | PASS | PASS | PASS | 18514 / 2023 / 1081 | 7 / 2 / 28 |
| `psf__requests-timeout-distill-02` | `timeout tuple must be (connect, read)` | PASS | PASS | PASS | 16400 / 1814 / 2219 | 9 / 2 / 28 |
| `pytest-dev__fixture-module-scope-distill-11` | `@pytest.fixture(scope="module")` | FAIL | PASS | PASS | 18842 / 2165 / 1442 | 7 / 2 / 28 |
| `pytest-dev__mark-parametrize-distill-05` | `ids callable must return str` | FAIL | PASS | PASS | 18501 / 2126 / 1053 | 5 / 2 / 28 |
| `requests__session-retry-adapter-distill-10` | `HTTPAdapter(max_retries=retry)` | PASS | PASS | PASS | 16431 / 2209 / 775 | 9 / 2 / 28 |
| `scipy__solve-assume-pos-distill-12` | `assume_a="pos"` | FAIL | FAIL | PASS | 18634 / 3602 / 840 | 7 / 2 / 28 |
| `sqlalchemy__lazy-raise-distill-09` | `lazy="raise"` | FAIL | FAIL | PASS | 18724 / 2047 / 1531 | 6 / 2 / 28 |
| `sympy__is-polynomial-distill-03` | `except PolynomialError: return False` | PASS | PASS | PASS | 16441 / 2066 / 1067 | 6 / 2 / 28 |

## 7. Figures

![Probe accuracy](figures/swe_probe_accuracy.svg)

![Final context](figures/swe_final_context.svg)

![Wall-clock](figures/swe_wall.svg)

![Prefill](figures/swe_prefill.svg)

![Per-task](figures/swe_per_task_accuracy.svg)

## 8. Failure analysis

Baseline failures below should be read with `spec_survived`. When survive is true, the fix was still in context and the miss is recency/confusion. DynCLM/CLM show the model can answer when obsolete SPECs are ignored.

### Baseline+compact

- `django__queryset-exists-distill-01`: got `ANSWER: len(list(self)) > 0` (spec_survived=True, compacts=7, rewrites=0)
- `pytest-dev__fixture-module-scope-distill-11`: got `We need answer user's current request. Need output exactly one line: ANSWER: <fragment>. User: Final` (spec_survived=True, compacts=7, rewrites=0)
- `pytest-dev__mark-parametrize-distill-05`: got `We need answer user's final probe. Need output exactly one line: ANSWER: <fragment> no other text. N` (spec_survived=True, compacts=5, rewrites=0)
- `scipy__solve-assume-pos-distill-12`: got `We need answer user's current request. Need think in English? User English. Need final probe at turn` (spec_survived=True, compacts=7, rewrites=0)
- `sqlalchemy__lazy-raise-distill-09`: got `We need answer user's request. Need produce final exactly? User: "Final probe at turn 28/28. Which r` (spec_survived=True, compacts=6, rewrites=0)

### DynCLM

- `scipy__solve-assume-pos-distill-12`: got `We need answer user's final probe. Need parse instructions carefully. User provided a long conversat` (spec_survived=True, compacts=0, rewrites=2)
- `sqlalchemy__lazy-raise-distill-09`: got `ANSWER: raise` (spec_survived=True, compacts=0, rewrites=2)

### CLM

_None._

Early SPEC retained (verbatim or in summary) in **12/12** baseline tasks.

## 9. Efficiency deep-dive

Quality-adjusted read: Baseline+compact may look fast while answering from a lossy summary; DynCLM/CLM spend reclaim prefill to keep the authoritative snippet verbatim. Prefer accuracy-first comparisons; treat wall alone as secondary.

| Arm | Spec survive % | Avg wall/task (s) |
|---|---:|---:|
| Baseline+compact | 100.0 | 378.7 |
| DynCLM | 100.0 | 170.6 |
| CLM | 100.0 | 134.0 |

## 10. Relation to prior suites

| Suite | What it showed | What SWE-Distill adds |
|---|---|---|
| Short gate | Mechanism + prefill vs CLM | Coding issue/snippet domain |
| Graded quality | Short multi-span accuracy (no reclaim trap) | Multi-turn growth + compaction pressure |
| Long-horizon A/B | FACT canary + compact matrix | SWE-inspired fix fragments under same reclaim physics |

## 11. Limitations & next steps

- Not official SWE-Bench Verified/Lite resolve (no docker, no fail-to-pass).
- Baseline+compact accuracy is a **retention-under-compaction** metric; do not quote it as the model’s coding competence without the SPEC-survive context.
- Model is local `Qwen3.8-27B-4bit` - not paper Qwen3.6-27B full precision.
- FLOPs are not prefix-reuse PFLOPs; we report wall / tokens / prefill.
- Optional later: Harbor DynCLM agent on TBLite if a paper-adjacent coding row is required.

## 12. Artifacts

- Source report JSON: `results/runs/20261015T223506Z_swe_distill/swe_distill/report.json`
- Figures: `docs/figures/swe_*.svg`
- Reproduce: `bash scripts/run_swe_distill.sh`
- Regenerate this doc: `python3 scripts/gen_swe_distill_report.py --report results/runs/20261015T223506Z_swe_distill/swe_distill/report.json`
