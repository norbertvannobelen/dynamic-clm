# DynCLM Final Report

Generated: 2026-10-15 · Live oMLX `Qwen3.8-27B-4bit` · Apple M5 Max (128GB; oMLX ~8 GB in-memory cache)

Companions: [method.md](method.md) · [analysis-plan.md](analysis-plan.md) (pre-registration) · [decision-report.md](decision-report.md) · [verification.md](verification.md)

## What we are trying to achieve

**Premise.** CLM soft-marks stale spans and physically rewrites **every turn** that has dead text - selective verbatim reclaim, but constant rewrite/re-prefill cost. **DynCLM** uses that **same** mark → drop → survivors-as-new-context methodology, but keeps marks soft until a cost model says we are tight on budget/memory, then runs one batched rewrite. Where per-turn rewrite is expensive, DynCLM should cut rewrite load versus CLM while matching CLM on quality canaries that need selective ignore. Method + session-load math: **[method.md](method.md)**.

**Mac / bandwidth.** A competing view is that CLM is unsuitable for Mac because always-rewrite/re-prefill hits **memory-bandwidth** limits on unified memory. This report is the M5 Max side of that comparison: either CLM is viable here, or DynCLM's per-device calibration (`ρ`) is the lever for SKUs that do suffer (hypothesized: **M5 Pro**, base **M5**). Full framing: [method.md § Hardware framing](method.md#hardware-framing-mac--apple-silicon).

This report is the **live proof attempt** of that premise on oMLX / **Apple M5 Max** (plus quality vs raw append-only and vs Codex-style summarize compact):

1. Does DynCLM **work** (markers, rewrites, shrink) and stay cheaper on rewrite prefill than always-rewrite CLM?
2. Does it **keep late-session answers correct** when raw append-only or summarize+recency baselines fail FACT/SPEC canaries?
3. Does it **shrink context** (and sometimes wall-clock) enough to be worth the reclaim prefill vs a never-rewrite baseline?
4. On this Mac SKU, is **always-rewrite CLM** still usable - or do we need DynCLM's **hardware profile** so narrower-bandwidth chips can defer?

We are **not** reproducing the CLM paper's TerminalBench / BrowseComp / EdgeBench numbers, and SWE-Distill here is a Lite-inspired reclaim harness, not official SWE-Bench resolve rate.

## Iterations (how we got here)

Early work used a 900-cell policy grid (paused), then an efficient 3-arm decision suite (gate / graded quality / long-horizon A, plus an LH-B matrix under blind FIFO drop-oldest). We then changed the compact baseline to **Codex-style rolling summary** and re-ran long-horizon B and the SWE-Distill baseline under that protocol. The 2026-10-10/13 runs established direction but were under-powered (LH-A n=3, LH-B n=2, SWE n=6); on 2026-10-15 we pre-registered powered sample sizes, tests, and decision rules in **[analysis-plan.md](analysis-plan.md)** and re-ran every suite live at those sizes. One pre-registered amendment: harnesses execute session-major round-robin after an aborted arm-major run showed thermal wall-clock drift biasing the last arm. **Everything below is the powered dataset**; the Oct 10/13 runs are retained as an internal replication dataset and are not cited for headline claims.

| Powered evidence (cited here) | Run | n per arm |
|---|---|---|
| Gate, graded quality, long-horizon A, long-horizon B | `20261015T165854Z_decision` | 16 / 18 tasks / 8 / 10 |
| SWE-Distill (tasks 1-12) | `20261015T223506Z_swe_distill` | 12 tasks |
| SWE-Distill extension (tasks 13-18, pre-registered, pooled) | `20261016T005250Z_swe_distill_ext` | +6 tasks |
| Prior replication (not cited for claims) | `20261010T145959Z_decision`, `20261013T201630Z_lh_compaction_summary`, `20261013T190132Z_swe_distill_summary_baseline` | 8 / 2 / 6 |

---

## How to read

| Label | Meaning | Suites |
|---|---|---|
| **Baseline (raw)** | Append-only; no markers; no rewrite; no compaction | Gate, graded quality, long-horizon A |
| **Baseline + compaction** | Codex-style rolling summary over `max_context` (keep system + recent raw; fold older spans into `[CONTEXT_SUMMARY]`; re-prefill) | Long-horizon B, SWE-Distill |
| **DynCLM** | Soft-invalidation markers + cost-model deferred rewrite | All |
| **CLM (immediate rewrite)** | Soft-invalidation markers + rewrite every turn | All |

Compare each suite only to **its** reference baseline (raw or summarize-compact).

**Serving.** oMLX has no span-level KV API. Prefill uses `LogicalCacheBackend` (span cap K=6) - logical SCR-shaped accounting, not physical relocation. Long-horizon and SWE-Distill marker arms serve prompts via `ContextTextForServing` (dead bodies stubbed).

## Statistical methods

Pre-registered in **[analysis-plan.md](analysis-plan.md)** before the powered runs; deviations are labeled exploratory.

- **Unit of analysis: the session (or task).** A session/task counts as success iff **all** of its late-window (or final) probes are correct.
- **Proportions (primary endpoints 1-3):** one-sided Fisher exact test on session/task success counts, α = 0.05, alternative = DynCLM > reference baseline; Wilson score 95% CIs per arm.
- **Continuous (primary endpoint 4 + secondary):** Welch two-sample t-test plus percentile-bootstrap 95% CI (10,000 resamples) of the mean difference. Gate rewrite prefill vs CLM is one-sided (DynCLM < CLM), matching H1.
- **Multiplicity:** the four primary endpoints are a confirmatory family with **Holm correction**; secondary endpoints are unadjusted and exploratory.
- **Wall-clock discipline:** no wall-clock headline is claimed unless the 95% CI of the difference excludes 0.
- **Extension rule:** exactly one pre-committed extension (SWE-Distill 12 → 18 tasks) if a primary endpoint lands at Fisher p ∈ (0.05, 0.20] with the point estimate favoring DynCLM. **This rule was invoked for SWE-Distill and is complete** (§ 6); the pooled 18-task test is the decisive analysis.

## Executive summary

| Suite | Ref. baseline | Headline (powered n, 95% CIs) |
|---|---|---|
| Short live gate (16×40) | Raw | **PASS** G1–G4. Rewrite prefill **−71.5%** vs CLM (5,010 vs 17,571 tok/session, Welch one-sided p = 1.4e-16); avg ctx **−56.2%**; session wall −3.3% (CI includes 0 - **not claimed**) |
| Graded quality (18 tasks) | Raw (short; no window reclaim) | **94.4% → 100%** (+5.6 pp). Flat ~162-token context; parity check only |
| Long-horizon A (8×24) | Raw + recency | Late FACT probe **0/8 → 8/8** (Fisher p = 7.8e-05); DynCLM late ctx **−70.8%**; late wall **−0.26 s/turn** vs raw (bootstrap CI excludes 0) |
| Long-horizon B (10×20 @ 16k/64k) | Summarize + recency | Late FACT probe **0/10 → 10/10 at both limits** (Fisher p = 5.4e-06 each); DynCLM late wall beats compact baseline at both limits (CIs exclude 0) |
| SWE-Distill (18 tasks, pooled) | Summarize + recency | Final probe **7/18 → 15/18** (38.9% → 83.3%; Fisher p = 0.0076); `spec_survive` 100% on all arms; wall 1.91h → 0.86h |

**Bottom line.** All four pre-registered primary endpoints are decisive in DynCLM's favor after Holm correction: LH-A (p = 7.8e-05), LH-B at both limits (p = 5.4e-06), gate rewrite prefill vs CLM (p = 1.4e-16), and SWE-Distill (pooled 18-task p = 0.0076, after the 12-task run landed in the pre-committed extension band at p = 0.185). Wall-clock headlines are limited to comparisons whose bootstrap CI excludes 0 (long-horizon late wall, SWE suite wall), per the pre-registered rule; the gate session-wall difference (−3.3%) is not claimed. DynCLM still trails CLM's perfect 18/18 on SWE-Distill accuracy (15/18) - the deferral window costs three tasks that always-rewrite CLM saves.

**Ship:** **yes** - every pre-registered primary endpoint is significant on the powered dataset. Standing tradeoffs: reclaim prefill vs raw baseline; CLM remains the fastest and, on SWE-Distill, the most accurate arm; canaries are not TerminalBench / official SWE-Bench / paper FLOP claims.

---

## Figures

Regenerate with `make charts`.

### Long-horizon A (vs raw)

![Late probe accuracy](figures/lh_late_probe_accuracy.svg)

![Late avg context](figures/lh_late_context.svg)

![Late steady wall-clock](figures/lh_late_steady_wall.svg)

![Late steady gen tok/s](figures/lh_late_tok_s.svg)

![Context vs turn](figures/lh_context_vs_turn.svg)

![Wall-clock vs turn](figures/lh_wall_vs_turn.svg)

### Short suites

![Gate avg context](figures/gate_context.svg)

![Gate rewrite prefill](figures/gate_prefill.svg)

![Gate prompt tokens](figures/gate_prompt.svg)

![Quality accuracy](figures/quality_accuracy.svg)

### Long-horizon B (summarize matrix)

![Probe vs max context](figures/compaction_probe_vs_limit.svg)

![Context vs max context](figures/compaction_context_vs_limit.svg)

![Wall vs max context](figures/compaction_wall_vs_limit.svg)

![Tok/s vs max context](figures/compaction_toks_vs_limit.svg)

### SWE-Distill

![Probe accuracy](figures/swe_probe_accuracy.svg)

![Final context](figures/swe_final_context.svg)

![Wall-clock](figures/swe_wall.svg)

![Prefill](figures/swe_prefill.svg)

---

## 1. Calibration

| Metric | Value |
|---|---|
| r (ms/token) | 2.787 |
| k_d (ms/token/turn) | 0.0376 |
| C_fixed (ms) | 1111 |
| ρ = C_fixed/k_d | 29538 |

oMLX chat-completions microbenchmark on `Qwen3.8-27B-4bit` → DynCLM scheduler (`β`, `γ`, `Δ`). Artifact: `configs/calibration/m5max-qwen38.json`.

---

## 2. Short live gate (G1–G4)

Fixed 3-arm grid, **16 sessions × 40 turns**, live oMLX, session-major round-robin. Mechanism, marker compliance, harness task success, and rewrite-prefill cost under logical cache accounting.

| ID | Name | Intent |
|---|---|---|
| G1 | Mechanism | Markers parse; rewrites fire; post-rewrite shrink within tolerance |
| G2 | Compliance | Tail/tool spans receive ignore markers as expected |
| G3 | Quality | DynCLM harness task success ≥ baseline and ≥ immediate |
| G4 | Cost | DynCLM rewrite prefill **lower** than immediate-rewrite |

**Result:** **PASS** (G1–G4). All CIs are bootstrap 95% over sessions (n = 16/arm).

| Metric | Baseline | DynCLM | CLM |
|---|---:|---:|---:|
| Success % | 100 (16/16) | 100 (16/16) | 100 (16/16) |
| Avg context | 11785 | 5163 | 3905 |
| Peak context | 23482 | 12657 | 8369 |
| Wall-clock / session (s) | 371.7 [347.0, 395.0] | 359.6 [336.5, 383.8] | 339.9 [313.2, 368.3] |
| Rewrite prefill / session | 0 | 5010 [4061, 5991] | 17571 [16469, 18600] |
| Rewrites / session | 0.0 | 4.9 | 17.1 |
| Prompt tokens / session | 99579 | 90913 | 87609 |
| Total usage tokens / session | 104095 | 95331 | 91788 |

**Primary endpoint (G4):** DynCLM rewrite prefill **−71.5%** vs CLM; Welch one-sided **p = 1.4e-16** (Holm-adjusted family p = 7.0e-16). DynCLM vs baseline: avg ctx **−56.2%**, prompt tokens **−8.7%** (bootstrap CI of the difference [−11774, −5598] excludes 0), total usage **−8.4%** (CI excludes 0). Session wall-clock is **−3.3%** vs baseline but the CI [−46.1, +21.8] s includes 0 (Welch p = 0.51) - reported as a favorable trend, **not claimed** per the pre-registered rule. CLM is the fastest arm (−8.6% vs baseline, CI also includes 0).

---

## 3. Live graded quality

18 deterministic tasks, live oMLX; distractors soft-invalidated for DynCLM/CLM; bidirectional substring grade after stripping CoT.

**Limitation:** context ~162 tokens on all arms - **no multi-turn reclaim**; short correctness check, not a size→speed test. Pre-registered as a parity check only (underpowered by design).

| Arm | Accuracy | Correct/N | Avg wall-s | Avg ctx |
|---|---:|---:|---:|---:|
| Baseline | 94.4% | 17/18 | 1.15 | 162 |
| DynCLM | 100.0% | 18/18 | 1.40 | 162 |
| CLM | 100.0% | 18/18 | 1.41 | 162 |

---

## 4. Long-horizon A - late quality + late speed

Plant `[FACT]` canary at turn 1; each turn appends obsolete tool noise + competing `WRONG_t`; DynCLM/CLM soft-ignore obsolete spans; baseline follows "most recent FACT." Probes grade canary presence. **8 sessions × 24 turns**; late window = turns 19-24; session success = all late probes correct.

| Arm | Late probe (session) | Wilson 95% CI | Late avg ctx | Late wall/turn (s) | Late tok/s | Prefill/sess | Rewrites/sess |
|---|---:|---:|---:|---:|---:|---:|---:|
| Baseline | 0/8 (0%) | [0.0, 32.4] | 6857 | 2.09 [1.92, 2.38] | 2.45 | 0 | 0.0 |
| DynCLM | 8/8 (100%) | [67.6, 100.0] | 2000 | 1.83 [1.79, 1.88] | 2.96 | 144 | 1.0 |
| CLM | 8/8 (100%) | [67.6, 100.0] | 375 | 1.57 [1.54, 1.62] | 3.12 | 2774 | 24.0 |

**Primary endpoint:** Fisher one-sided DynCLM > baseline **p = 7.8e-05** (Holm-adjusted family p = 1.6e-04); CLM identical. DynCLM: full late-quality win at 1 rewrite/session vs CLM's 24 (rewrite prefill −94.8% vs CLM). **Secondary (late wall):** DynCLM −0.26 s/turn vs raw baseline, bootstrap CI [−0.56, −0.07] excludes 0 (Welch p = 0.108); CLM −0.52 s/turn, CI [−0.81, −0.33], Welch p = 0.0075 - CLM remains fastest. Probe-level proportions (within-session correlated, descriptive only): baseline 0/48, DynCLM 24/24, CLM 24/24 late-window probes.

---

## 5. Long-horizon B - summarize compaction × max context

Same recency-trap session as A, but the reference baseline is **Codex-style rolling summary** when over `max_context`. Matrix **16384 / 65536**, **10 sessions × 20 turns** per arm per limit, full 3-arm live run. Late window = turns 15-20.

| Max ctx | Compact probe (session) | DynCLM | CLM | Fisher p (dyn, imm vs compact) |
|---|---:|---:|---:|---:|
| 16384 | 0/10 (0%) [0.0, 27.8] | 10/10 (100%) [72.2, 100.0] | 10/10 (100%) [72.2, 100.0] | 5.4e-06, 5.4e-06 |
| 65536 | 0/10 (0%) [0.0, 27.8] | 10/10 (100%) [72.2, 100.0] | 10/10 (100%) [72.2, 100.0] | 5.4e-06, 5.4e-06 |

**Primary endpoint:** significant at both limits (Holm-adjusted family p = 2.2e-05 each).

| Max ctx | Compact late ctx | DynCLM late ctx | CLM late ctx | Compact late wall (s) | DynCLM late wall (s) | CLM late wall (s) |
|---|---:|---:|---:|---:|---:|---:|
| 16384 | 14502 | 5154 | 320 | 2.45 [2.22, 2.85] | 1.53 [1.51, 1.55] | 1.39 [1.37, 1.40] |
| 65536 | 58201 | 19348 | 320 | 4.07 [3.15, 5.87] | 1.50 [1.48, 1.52] | 1.37 [1.35, 1.39] |

**Secondary (late wall):** DynCLM beats the compact baseline at 16k (−0.92 s/turn, CI [−1.33, −0.69], p = 0.001) and at 64k (−2.57 s/turn, CI [−4.38, −1.65], p = 0.018); CLM is fastest at both. Summarize baseline late probes stay **0%** (`WRONG_*` under "prefer most recent") while `spec`-style survival of the original fact remains in context - failures are recency confusion, not eviction.

Details: [longhorizon-compaction-report.md](longhorizon-compaction-report.md).

---

## 6. SWE-Distill - coding-snippet reclaim under window pressure

SWE-Bench-Lite-**inspired** distill (not official SWE-Bench / TerminalBench / Harbor). Plant correct `[SPEC]` at turn 1; flood wrong SPECs past shared `max_context=16384`; probe `ANSWER: <fragment>`. Baseline uses the same Codex-style summarize compact as LH-B. **18 tasks × 28 turns** (12 + 6 pre-registered extension, pooled), task-major round-robin; task success = final probe correct.

| Arm | Final-probe accuracy | Wilson 95% CI | Spec survive % | Avg final live ctx | Peak ctx | Avg prefill / task | Wall (h) |
|---|---:|---:|---:|---:|---:|---:|---:|
| Baseline+compact | **38.9%** (7/18) | [20.3, 61.4] | **100%** | 18037 | 16385 (cap) | 107715 | 1.91 |
| DynCLM | **83.3%** (15/18) | [60.8, 94.2] | **100%** | 2114 | 2159 | 1161 | 0.86 |
| CLM | **100.0%** (18/18) | [82.4, 100.0] | **100%** | 1557 | 2374 | 10687 | 0.71 |

**Primary endpoint: decisive after the pre-registered extension.** The initial 12-task run landed at 10/12 vs 7/12, Fisher one-sided **p = 0.185** - inside the pre-committed extension band (0.05, 0.20] with the point estimate favoring DynCLM - so the suite was extended once to 18 tasks (6 new fixtures: pandas merge-validate, numpy unique-first-index, sqlalchemy selectinload, requests stream-download, pytest capfd, scipy minimize-bounds), exactly as [the plan](analysis-plan.md#extension-rule) allows. On the extension tasks the summarize baseline went 0/6 and DynCLM 5/6. **Pooled: 15/18 vs 7/18, Fisher one-sided p = 0.0076** (Holm-adjusted family p = 0.0076) - significant. No second extension is permitted by the plan; none was needed.

Baseline failures (11/18) are recency/confusion with the fix still present (`spec_survive` 100% on all arms). **DynCLM trails CLM here:** its 3/18 failures (`sqlalchemy-lazy-raise`, `scipy-solve-assume-pos`, `numpy-unique-first-index`) are all tasks CLM passes - carrying soft-invalidated wrong SPECs across the deferral window occasionally lets a distractor win the final probe - while using ~9.2x less rewrite prefill than CLM (1,161 vs 10,687 tokens/task). **Secondary (wall):** DynCLM −209 s/task vs the compact baseline, bootstrap CI [−232, −185] excludes 0 (Welch p = 8.9e-18); CLM is +30 s/task faster than DynCLM, CI [+10, +50] (p = 0.0075).

Full write-up: [swe-distill-report.md](swe-distill-report.md).

---

## 7. Learning-900 (optional / incomplete)

Plan (`configs/learning.yaml`): 3 models × 3 tasks × 5 policies × 20 sessions = **900** cells (β/γ sweep, not the 3-arm decision instrument). Paused at **207/900**; only `Qwen3.8-27B-4bit` cells on disk. Not required for ship.

```bash
bash scripts/resume_live_full.sh results/runs/20261009T034520Z
```

---

## 8. Relation to the CLM paper

Paper ([arXiv:2609.37725](https://arxiv.org/abs/2609.37725)) coding-adjacent evals: TerminalBench / TBLite, EdgeBench-10, Software World, BrowseComp-Plus, ContextBench - **not** SWE-Bench Verified/Lite for CLM quality. This harness is a local oMLX soft-invalidation vs raw/summarize/immediate-rewrite study with logical cache accounting. See [verification.md](verification.md).

---

## Ship / no-ship

| Criterion | Result (powered) |
|---|---|
| Late FACT probe ≥ baseline (LH-A) | **yes** - 0/8 → 8/8, Fisher p = 7.8e-05 (Holm-adj 1.6e-04) |
| Late FACT probe ≥ summarize baseline (LH-B, both limits) | **yes** - 0/10 → 10/10 at 16k and 64k, Fisher p = 5.4e-06 each (Holm-adj 2.2e-05) |
| SWE final probe ≥ summarize baseline | **yes** - pooled 18-task 15/18 vs 7/18, Fisher p = 0.0076 (Holm-adj 0.0076), after the pre-committed extension |
| Graded quality ≥ raw baseline | **yes** (100% vs 94.4%; parity check, underpowered by design) |
| Late-session context ≤ baseline (A, B) | **yes** (−70.8% LH-A; −64.5% / −66.8% LH-B) |
| Session wall (gate) ≤ raw baseline | **not claimed** (−3.3%, CI [−46.1, +21.8] s includes 0; secondary) |
| Late steady wall ≤ raw baseline (LH-A) | **yes** (−0.26 s/turn, bootstrap CI [−0.56, −0.07] excludes 0; secondary, Welch p = 0.108) |
| Late steady wall ≤ summarize baseline (LH-B) | **yes** (CIs exclude 0 at both limits; secondary) |
| Rewrite prefill ≤ raw baseline | **no** (raw pays 0) |
| Rewrite prefill ≤ CLM (gate) | **yes** (−71.5%, Welch one-sided p = 1.4e-16) |
| Learning-900 complete | **no** (optional) |
| Paper TerminalBench / BrowseComp / EdgeBench | **not run** |
| Physical SCR on oMLX | **not available** |

**Ship** DynCLM reclaim for this local oMLX stack: all four pre-registered primary endpoints are significant on the powered dataset (Holm-corrected), quality parity with CLM holds on gate and both long-horizon suites, and the SWE-Distill endpoint cleared its pre-committed extension (pooled p = 0.0076). Honest tradeoffs: reclaim prefill is an added cost vs raw (raw pays 0); CLM is the fastest arm on late wall and the only arm at 100% on SWE-Distill (DynCLM 15/18) - the deferral window is a real, measured accuracy cost on that suite, bought back at ~9x less rewrite prefill than CLM.

---

## Gaps

- DynCLM trails CLM on SWE-Distill accuracy (15/18 vs 18/18); the three failures are deferral-window distractor wins, not mechanism failures.
- Learning-900 incomplete (207/900; one model only).
- Not TerminalBench / BrowseComp-Plus / EdgeBench / official SWE-Bench.
- Logical cache accounting only; no hybrid recurrent-state SCR path.
- Single model and single machine for all ship evidence (**M5 Max** only; no M5 Pro / base M5 hardware-profile comparison yet).
- Gate session wall-clock is underpowered by ~6x (n ≈ 94/arm needed for the observed effect); reported with CIs only.
- Graded quality is not a reclaim test; gate G3 is harness success, not graded answers.

---

## Artifacts

| What | Where |
|---|---|
| Method (premise + protocols + math) | [method.md](method.md) |
| Pre-registration (endpoints, tests, n, extension rule) | [analysis-plan.md](analysis-plan.md) |
| Powered gate / quality / LH-A / LH-B | `results/runs/20261015T165854Z_decision/` |
| Powered SWE-Distill (tasks 1-12) | `results/runs/20261015T223506Z_swe_distill/` |
| SWE-Distill extension + pooled 18-task report | `results/runs/20261016T005250Z_swe_distill_ext/` (`pooled-report.json`) · [swe-distill-report.md](swe-distill-report.md) |
| Prior replication dataset | `results/runs/20261010T145959Z_decision/`, `results/runs/20261013T201630Z_lh_compaction_summary/`, `results/runs/20261013T190132Z_swe_distill_summary_baseline/` |
| Figures | `docs/figures/*.svg` |
| Decision tables | [decision-report.md](decision-report.md) |
| LH-A detail | [longhorizon-report.md](longhorizon-report.md) |
| Gate / quality docs | [interim-report.md](interim-report.md), [quality-report.md](quality-report.md) |
| Learning archive | `results/runs/20261009T034520Z/archive_learning_900_*` |
