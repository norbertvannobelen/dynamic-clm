# DynCLM SWE-Distill - Extensive Report

Generated: 2026-10-13T20:08:08.358518Z · Model `Qwen3.8-27B-4bit` · Hardware `apple-m5-max` · live=True

**Disclaimer:** This is a SWE-Bench-**Lite-inspired** multi-turn coding distill under shared `max_context` with Baseline+compact / DynCLM / CLM. It is **not** an official SWE-Bench resolve-rate number and is not TerminalBench / Harbor.

## 1. Executive verdict

On this suite, DynCLM final-probe accuracy is **100.0%** (+50.0 pp vs Baseline+compact **50.0%**); CLM is **100.0%** (+50.0 pp). Baseline+compact retains the early SPEC (verbatim or in rolling summary) in **100%** of tasks; DynCLM/CLM keep it via soft-invalidation (100% / 100%). CLM pays the highest rewrite prefill; DynCLM sits between quality parity with CLM and much lower rewrite N than always-rewrite.

**Read.** Baseline is Codex-style **lossy summarization**. On this run it **retains** the early SPEC (`spec_survive` **100%**) but still scores **50%** because the prompt prefers the most recent non-ignored SPEC and wrong reviewer dumps remain live. DynCLM/CLM soft-ignore those dumps and score **100%** (+50 pp). See §2. Not official SWE-Bench resolve rate; not comparable to graded quality (+5.6 pp vs raw with no window reclaim).

## 2. How to read Baseline+compact accuracy

This suite measures **reclaim under window pressure** against summarize compaction, not single-shot coding skill.

1. **Plant**: turn 1 inserts the correct fix as `[SPEC]`.
2. **Flood**: later turns append plausible wrong SPECs plus padding so tokens exceed `max_context` (~1.4×).
3. **Rolling summary compact**: keep `sys` + recent raw; fold older spans into `[CONTEXT_SUMMARY]`; re-prefill.
4. **Recency instruction**: without ignore markers, prefer the most recent SPEC.
5. **Probe**: grade `ANSWER: <fragment>` against turn-1 truth.

`spec_survive` counts the expected fragment still present (verbatim span or inside the summary). Here it is **100%** for baseline, DynCLM, and CLM. Accuracy gaps are therefore **recency / confusion**, not “the SPEC was deleted.”

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
| tasks | **6** |
| cells | **18** |
| target wall | ~2h / arm |
| observed wall | Baseline+compact **0.90h** · DynCLM **0.34h** · CLM **0.44h** |

Wall came in under the 2h/arm budget while still forcing compaction (baseline avg compacts **7.3**/task) and DynCLM/CLM reclaim. Context is already saturating near `max_context`; stretching wall further would not change the reclaim physics.

Fixtures live under `testdata/swe_distill/` (paraphrased Lite-inspired excerpts with per-file attribution).

## 4. Headline results

Context columns: **final live** is decode-relevant tracker live tokens; **final total** is live+dead footprint; **final prompt** is oMLX `prompt_tokens` on the probe (marker arms still serialize ignored dead spans until rewrite). **Steady wall** excludes compact/rewrite turns when present.

| Arm | Accuracy | Correct/N | Avg final live | Avg final total | Avg final prompt | Peak live | Avg compacts | Avg rewrites | Avg prefill | Total wall (s) | Steady wall/task (s) | Wall (h) |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| Baseline+compact | 50.0% | 3/6 | 17540 | 17540 | 12013 | 16385 | 7.33 | 0.00 | 114387 | 3256.7 | 189.9 | 0.90 |
| DynCLM | 100.0% | 6/6 | 1939 | 8401 | 2524 | 2107 | 0.00 | 2.00 | 1238 | 1222.4 | 189.6 | 0.34 |
| CLM | 100.0% | 6/6 | 1064 | 1117 | 1454 | 3661 | 0.00 | 28.00 | 5074 | 1572.6 | 262.1 | 0.44 |

## 5. Vs Baseline+compact

| Metric | DynCLM | CLM |
|---|---|---|
| Accuracy Δ (pp) | +50.0 | +50.0 |
| Final live ctx % | -88.9% | -93.9% |
| Final total ctx % | -52.1% | -93.6% |
| Final prompt % | -79.0% | -87.9% |
| Total wall % | -62.5% | -51.7% |
| Steady wall % | -0.2% | +38.0% |
| Prefill Δ (avg) | -113149 | -109313 |

## 6. Per-task breakdown

| Instance | Expected | Baseline+compact | DynCLM | CLM | Final ctx B/D/C | Reclaim events B/D/C |
|---|---|:-:|:-:|:-:|---|---|
| `astropy__units-compose-distill-06` | `simplify(num, den, unit.scale)` | FAIL | PASS | PASS | 18732 / 1863 / 554 | 7 / 2 / 28 |
| `django__queryset-exists-distill-01` | `self.query.has_results(using=self.db)` | FAIL | PASS | PASS | 16431 / 2158 / 3661 | 9 / 2 / 28 |
| `matplotlib__tight-layout-distill-04` | `if layout engine is not None: return` | PASS | PASS | PASS | 16438 / 2023 / 535 | 9 / 2 / 28 |
| `psf__requests-timeout-distill-02` | `timeout tuple must be (connect, read)` | FAIL | PASS | PASS | 18655 / 1762 / 553 | 7 / 2 / 28 |
| `pytest-dev__mark-parametrize-distill-05` | `ids callable must return str` | PASS | PASS | PASS | 18543 / 1923 / 546 | 6 / 2 / 28 |
| `sympy__is-polynomial-distill-03` | `except PolynomialError: return False` | PASS | PASS | PASS | 16441 / 1905 / 534 | 6 / 2 / 28 |

## 7. Figures

![Probe accuracy](figures/swe_probe_accuracy.svg)

![Final context](figures/swe_final_context.svg)

![Wall-clock](figures/swe_wall.svg)

![Prefill](figures/swe_prefill.svg)

![Per-task](figures/swe_per_task_accuracy.svg)

## 8. Failure analysis

Baseline failures below have `spec_survived=true`: the fix was still in context, but the model followed a wrong/recent dump or failed to extract the fragment. DynCLM/CLM show the model can answer when obsolete SPECs are ignored.

### Baseline+compact

- `astropy__units-compose-distill-06`: got `We need answer user's current request. Need parse entire conversation. User initial: "You are a care` (spec_survived=True, compacts=7, rewrites=0)
- `django__queryset-exists-distill-01`: got `ANSWER: len(list(self)) > 0` (spec_survived=True, compacts=9, rewrites=0)
- `psf__requests-timeout-distill-02`: got `We need answer user's current request. Need parse conversation. User initial: careful coding assista` (spec_survived=True, compacts=7, rewrites=0)

### DynCLM

_None._

### CLM

_None._

Early SPEC retained (verbatim or in summary) in **6/6** baseline tasks.

## 9. Efficiency deep-dive

Quality-adjusted read: Baseline+compact may look fast while answering from a lossy summary; DynCLM/CLM spend reclaim prefill to keep the authoritative snippet verbatim. Prefer accuracy-first comparisons; treat wall alone as secondary.

| Arm | Spec survive % | Avg wall/task (s) |
|---|---:|---:|
| Baseline+compact | 100.0 | 542.8 |
| DynCLM | 100.0 | 203.7 |
| CLM | 100.0 | 262.1 |

## 10. Relation to prior suites

| Suite | What it showed | What SWE-Distill adds |
|---|---|---|
| Short gate | Mechanism + prefill vs CLM | Coding issue/snippet domain |
| Graded quality | Short multi-span accuracy (no reclaim trap) | Multi-turn growth + compaction pressure |
| Long-horizon A/B | FACT canary + compact matrix | SWE-inspired fix fragments under same reclaim physics |

## 11. Limitations & next steps

- Not official SWE-Bench Verified/Lite resolve (no docker, no fail-to-pass).
- Quote accuracy **with** `spec_survive`: here survive is 100% and accuracy is 50%, so the gap is recency under summarization, not coding skill alone.
- Model is local `Qwen3.8-27B-4bit` - not paper Qwen3.6-27B full precision.
- FLOPs are not prefix-reuse PFLOPs; we report wall / tokens / prefill.
- Optional later: Harbor DynCLM agent on TBLite if a paper-adjacent coding row is required.

## 12. Artifacts

- Source report JSON: `results/runs/20261013T190132Z_swe_distill_summary_baseline/swe_distill/report.json`
- Figures: `docs/figures/swe_*.svg`
- Reproduce: `bash scripts/run_swe_distill.sh`
- Regenerate this doc: `python3 scripts/gen_swe_distill_report.py --report results/runs/20261013T190132Z_swe_distill_summary_baseline/swe_distill/report.json`
