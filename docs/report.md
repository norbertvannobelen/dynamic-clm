# DynCLM Final Report

Generated: 2026-10-10 · Primary evidence: live decision suite `20261010T145959Z_decision`

Companion: [decision-report.md](decision-report.md) (tables from the same run).

## How to read this report

**Arms**

| Label in charts/tables | Meaning |
|---|---|
| **Baseline (raw)** | Append-only context; no markers; no rewrite; no compaction |
| **Baseline + compaction** | Append-only until `max_context`, then Codex-style rolling summary (keep system + recent raw; fold older spans into a budget-capped `[CONTEXT_SUMMARY]`) and re-prefill survivors |
| **DynCLM** | Soft-invalidation markers + cost-model deferred rewrite |
| **CLM (immediate rewrite)** | Soft-invalidation markers + rewrite every turn (paper-style upper bound on reclaim aggressiveness) |

Every efficiency claim is **versus the suite’s reference baseline** (raw for gate / quality / long-horizon A; compaction for long-horizon B), unless labeled otherwise.

---

## Executive summary

Live 3-arm decision suite on oMLX `Qwen3.8-27B-4bit` (Apple M5 Max): short gate + graded quality + long-horizon A + compaction matrix (16k / 64k).

| Suite | Headline |
|---|---|
| Short live gate (8×40) | **PASS** G1–G4. DynCLM avg ctx **−53.8%**, session wall **−9.0%** vs baseline; rewrite prefill **−71.1%** vs CLM |
| Graded quality (18 tasks) | Baseline **94.4%** → DynCLM/CLM **100%** (+5.6 pp) |
| Long-horizon A (3×24) | Late probe **0% → 100%** (DynCLM & CLM). DynCLM late ctx **−70.8%**; CLM fastest late wall |
| Long-horizon B (2×20 @ 16k/64k) | Summarize baseline late probe still **0%**; DynCLM/CLM **100%** at 16k/64k (recency trap under rolling summary - see §5) |
| SWE-Distill (6 coding tasks) | Codex-style summary baseline **50%** · DynCLM/CLM **100%** (+50 pp); baseline **spec survive 100%** (was 0% under blind FIFO) - see §6 |

**Bottom line.** DynCLM matches CLM on the quality axes that matter here (graded accuracy, late FACT probes, and SWE-Distill probes) while spending far fewer rewrites than always-rewrite CLM. On SWE-Distill with a paper-adjacent summarization baseline, DynCLM/CLM still win (**+50 pp**) even though the baseline now **retains** the early SPEC (`spec_survive` **100%**) - failures are recency/confusion under lossy summary, not FIFO wipeout.

**Ship recommendation:** **ship** with the explicit tradeoff that DynCLM pays reclaim prefill vs raw baseline (which never rewrites) and is usually slower than CLM on late wall while winning on N/prefill vs CLM.

---

## Figures (baseline · DynCLM · CLM)

### Long-horizon A (live, vs raw baseline)

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

### Long-horizon B (compaction matrix)

![Probe vs max context](figures/compaction_probe_vs_limit.svg)

![Context vs max context](figures/compaction_context_vs_limit.svg)

![Wall vs max context](figures/compaction_wall_vs_limit.svg)

![Tok/s vs max context](figures/compaction_toks_vs_limit.svg)

Regenerate with `make charts` (`scripts/gen_charts.py`, stdlib SVG - no matplotlib).

---

## 1. Calibration

| Metric | Value |
|---|---|
| r (ms/token) | 2.787 |
| k_d | 0.0376 |
| C_fixed (ms) | 1111 |
| ρ = C_fixed/k_d | 29538 |

Microbenchmark via oMLX chat-completions wall-clock on `Qwen3.8-27B-4bit`. Feeds the DynCLM cost-model scheduler (`β`, `γ`, `Δ`).

Artifact: `configs/calibration/m5max-qwen38.json`.

---

## 2. Short live gate (G1–G4)

**What it is.** Fixed 3-arm grid (baseline / immediate-rewrite / dynclm), **8 sessions × 40 turns**, **live** oMLX. Measures context accounting, marker parse rate, shrink error, rewrite prefill, and session wall under SCR-style logical backend (K=6).

**Pass criteria**

| ID | Name | Intent |
|---|---|---|
| G1 | Mechanism | Markers parse; rewrites fire; post-rewrite shrink within tolerance |
| G2 | Compliance | Tail/tool spans receive ignore markers as expected |
| G3 | Quality | DynCLM success ≥ baseline and ≥ immediate (harness task success) |
| G4 | Cost | DynCLM rewrite prefill **lower** than immediate-rewrite |

**Result (decision suite).** Gate **PASS** (G1–G4).

| Metric | Baseline | DynCLM | CLM |
|---|---:|---:|---:|
| Success % | 100.0 | 100.0 | 100.0 |
| Avg context | 11714 | 5410 | 3995 |
| Peak context | 23009 | 12630 | 8556 |
| Wall-clock / session (s) | 402.3 | 365.9 | 352.5 |
| Rewrite prefill / session | 0 | 5340 | 18504 |
| Rewrites / session | 0.0 | 5.0 | 17.0 |

DynCLM vs baseline: avg ctx **−53.8%**, wall **−9.0%**. DynCLM vs CLM: rewrite prefill **−71.1%** (5340 vs 18504).

Details: [interim-report.md](interim-report.md), `results/runs/20261010T145959Z_decision/gate/`.

---

## 3. Live graded quality

**What it is.** 18 deterministic tasks (retention, constraint, recovery, coding, adversarial FACT/coding) run **live** on oMLX. Distractor spans are soft-invalidated for DynCLM/CLM; answers graded by bidirectional substring match after stripping CoT.

**Limitation.** Context length is essentially flat across arms (~162 tokens): **no multi-turn reclaim**, so it cannot show size→speed.

| Arm | Accuracy | Correct/N | Avg wall-s | Avg ctx |
|---|---:|---:|---:|---:|
| Baseline | 94.4% | 17/18 | 1.55 | 162 |
| DynCLM | 100.0% | 18/18 | 1.77 | 162 |
| CLM | 100.0% | 18/18 | 1.83 | 162 |

Details: [quality-report.md](quality-report.md).

---

## 4. Long-horizon A - late quality + late speed

**Claim under test.** In long sessions, obsolete tool noise accumulates; soft-invalidation + deferred rewrite improves late-session probe quality and keeps context small vs raw append-only.

**Protocol**

1. Plant authoritative `[FACT]` at turn 1 (`CANARY_*`).
2. Each turn appends large obsolete tool noise + competing pseudo-FACT (`WRONG_t`).
3. DynCLM/CLM: harness applies `<<<DYNCLM_IGNORE>>>` on obsolete tool spans; cost model decides rewrite.
4. Baseline: no markers - instruction says use most recent FACT → recency trap.
5. Probes every N turns ask for the canary; grade exact token presence.
6. Windows: early vs late; **late_steady** excludes rewrite spikes (except immediate-rewrite).

**Run.** Live oMLX, **3 sessions × 24 turns × 3 arms**. See [longhorizon-report.md](longhorizon-report.md).

| Arm | Late probe % | Late avg ctx | Late steady wall-s | Late tok/s | Prefill/sess | Rewrites/sess |
|---|---:|---:|---:|---:|---:|---:|
| Baseline | 0.0 | 6857 | 2.60 | 2.19 | 0 | 0.0 |
| DynCLM | 100.0 | 2000 | 3.78 | 1.74 | 144 | 1.0 |
| CLM | 100.0 | 375 | 1.99 | 2.36 | 2774 | 24.0 |

DynCLM keeps the full late-quality win (+100 pp) with ~1 rewrite/session vs CLM’s 24; CLM is faster on late wall and holds the smallest context.

---

## 5. Long-horizon B - compaction baseline × max context

**Motivation.** Production systems compact when hitting a window. Suite B asks whether DynCLM/CLM still win when the baseline may compact.

**Protocol (delta from A)**: run `20261013T201630Z_lh_compaction_summary`

- Arms: `baseline-compact`, `immediate-rewrite` (CLM), `dynclm`.
- When over `max_context`, Codex-style rolling summary (keep `sys` + recent raw; fold older spans into `[CONTEXT_SUMMARY]`); charge full re-prefill of survivors.
- Matrix: **16384, 65536** (2 sessions × 20 turns).

| Max ctx | Compact probe% | DynCLM | CLM | Compact ctx | DynCLM ctx | CLM ctx | Compact wall | DynCLM wall | CLM wall |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 16384 | 0.0 | 100.0 | 100.0 | 14502 | 5154 | 320 | 4.04 | 1.45 | 1.15 |
| 65536 | 0.0 | 100.0 | 100.0 | 58201 | 19348 | 320 | 3.90 | 1.78 | 1.65 |

**Read.** Even with rolling summary (not blind FIFO), late probes stay **0%** for baseline: the session plants competing `[FACT] … WRONG_n` dumps and instructs “use the most recent FACT,” so the model answers `WRONG_*` while DynCLM/CLM soft-ignore obsolete dumps and keep **100%**. This suite is a stronger recency trap than SWE-Distill (where summarize baseline reached **50%** with **100%** SPEC survive). Quality-adjusted, DynCLM/CLM still dominate; compact baseline can look fine on wall while answering wrong.

Details: [longhorizon-compaction-report.md](longhorizon-compaction-report.md).

---

## 6. SWE-Distill - realistic coding quality (live)

Run `20261013T190132Z_swe_distill_summary_baseline` on `Qwen3.8-27B-4bit`, shared `max_context=16384`. **Baseline+compact** re-run live under Codex-style rolling summary; **DynCLM/CLM** cells reused from `20261010T225155Z_swe_distill` (unchanged reclaim path). Full write-up: [swe-distill-report.md](swe-distill-report.md).

Baseline keeps `sys` + recent raw and folds older spans into a budget-capped `[CONTEXT_SUMMARY]` (early-requirements quota + newest-preferred notes). Old blind-FIFO run was Baseline **16.7%** / **0%** survive - do not cite that as current.

| Arm | Final-probe accuracy | Spec survive % | Avg final live | Avg final prompt | Wall (h) |
|---|---:|---:|---:|---:|---:|
| Baseline+summary | **50.0%** (3/6) | **100%** | 17540 | 12013 | 0.90 |
| DynCLM | 100.0% (6/6) | **100%** | 1939 | 2524 | **0.34** |
| CLM | 100.0% (6/6) | **100%** | 1064 | 1454 | 0.44 |

DynCLM/CLM **+50 pp** vs summarize baseline (parity with each other). Spec survive is **100%** on all arms - baseline failures are wrong/confused answers with the fix still in context, not dropped SPECs. Live ctx **−89%** (DynCLM) vs baseline; DynCLM wall still under CLM on the reused cells.

## 7. Learning-900 policy grid (optional / incomplete)

**Plan design** (`configs/learning.yaml`): 3 models × 3 task types × 5 policies × 20 sessions = **900** cells - a **DynCLM policy / βγ sweep**, not the 3-arm Baseline/DynCLM/CLM decision instrument.

**Status.** Paused for disk and archived under `results/runs/20261009T034520Z/archive_learning_900_20261010T145859Z` (~207/900 cells before pause). **Not required** for the ship verdict above.

Resume if needed:

```bash
bash scripts/resume_live_full.sh results/runs/20261009T034520Z
```

---

## 8. Relation to the CLM paper’s coding benchmarks

The CLM paper ([arXiv:2609.37725](https://arxiv.org/abs/2609.37725)) does **not** evaluate on SWE-Bench Verified / Lite / Multilingual for CLM quality. Coding-adjacent paper evals are:

| Paper benchmark | Role |
|---|---|
| **TerminalBench 2.1** + **TBLite** | Terminal-coding quality/efficiency (shared **Mini-SWE-Agent** backbone) |
| **EdgeBench-10** | 12-hour single-repo optimization |
| **Software World** | 24-hour multi-repo agent swarm |
| BrowseComp-Plus | Deep research (primary accuracy/FLOP claim) |
| ContextBench | Diagnostic context-management tasks |

Our harness measures soft-invalidation + deferred rewrite vs raw / compact / immediate-rewrite on local oMLX; it is **not** a TerminalBench or SWE-Bench reproduction. See [verification.md](verification.md).

---

## Ship / no-ship

| Criterion | Result |
|---|---|
| Late-session quality ≥ baseline | **yes** (A + B) |
| Late-session context vs baseline | **yes** |
| Session wall (gate) vs baseline | **yes** (−9%) |
| Late steady wall vs baseline (A) | **no** on this run (DynCLM slower; CLM faster) |
| Rewrite prefill vs baseline | **no** (baseline is 0) |
| Rewrite prefill vs CLM | **yes** (−71% on gate) |
| Full 900-session live grid | **outstanding** (optional) |
| Paper TerminalBench / BrowseComp reproduction | **not run** |

Recommendation: **ship** for the DynCLM reclaim policy (quality parity with CLM, much lower rewrite N than CLM, clear win vs raw/compact baselines on late probes).

---

## Gaps

- Learning-900 policy grid incomplete (archived mid-run).
- Not TerminalBench / BrowseComp-Plus / EdgeBench paper reproduction.
- oMLX has no span-level KV API; prefill is logical/SCR-accounting.
- Decision-suite LH-B used 2 limits (16k/64k), not the older 4-limit exploratory matrix.
- Late wall/tok/s on small n are directional.

---

## Artifacts

| What | Where |
|---|---|
| Decision suite run | `results/runs/20261010T145959Z_decision/` |
| Decision report | [decision-report.md](decision-report.md) |
| SWE-Distill | [swe-distill-report.md](swe-distill-report.md) · `results/runs/20261013T190132Z_swe_distill_summary_baseline/` |
| Figures | `docs/figures/*.svg` |
| Long-horizon A/B | [longhorizon-report.md](longhorizon-report.md), [longhorizon-compaction-report.md](longhorizon-compaction-report.md) · LH-B re-run `results/runs/20261013T201630Z_lh_compaction_summary/` |
| Gate / quality | [interim-report.md](interim-report.md), [quality-report.md](quality-report.md) |
| Learning archive | `results/runs/20261009T034520Z/archive_learning_900_*` |
| Paper verification | [verification.md](verification.md) |
