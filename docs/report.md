# DynCLM Final Report

Generated: 2026-10-09

## How to read this report

**Arms**

| Label in charts/tables | Meaning |
|---|---|
| **Baseline (raw)** | Append-only context; no markers; no rewrite; no compaction |
| **Baseline + compaction** | Append-only until `max_context`, then drop oldest spans (system kept when possible) and re-prefill survivors |
| **DynCLM** | Soft-invalidation markers + cost-model deferred rewrite |
| **CLM (immediate rewrite)** | Soft-invalidation markers + rewrite every turn (paper-style upper bound on reclaim aggressiveness) |

Every efficiency claim is **versus the suite’s reference baseline** (raw for suite A / short tests; compaction for suite B), unless labeled otherwise.

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

Regenerate with `make charts` (`scripts/gen_charts.py`, stdlib SVG — no matplotlib).

---

## 1. Short tests (plan Phase A / initial harness)

These are the **initial** evaluation layers defined for the DynCLM plan: mechanism gate, live graded quality, calibration, and offline benchmarks. They do **not** by themselves prove the paper’s long-session claim; they establish that markers, rewrites, and basic quality work before long-horizon work.

### 1.1 Calibration (M3 / M5 Max)

| Metric | Value |
|---|---|
| r (ms/token) | 2.787 |
| k_d | 0.0376 |
| C_fixed (ms) | 1111 |
| ρ = C_fixed/k_d | 29538 |

Microbenchmark via oMLX chat-completions wall-clock on `Qwen3.8-27B-4bit`. Feeds the DynCLM cost-model scheduler (`β`, `γ`, `Δ`, rewrite when amortized re-prefill beats carrying dead tokens).

Artifact: `configs/calibration/m5max-qwen38.json`.

### 1.2 Multi-turn gate G1–G4 (synthetic agent)

**What it is.** Fixed 3-arm grid (baseline / immediate-rewrite / dynclm), 20 sessions × 50 turns, synthetic agent (no live decode). Measures context accounting, marker parse rate, shrink error, and rewrite prefill under SCR-style logical backend (K=6) because oMLX has no span KV APIs.

**Config.** `configs/gate.yaml` — `max_context: 4096` so `β·max_context` is reachable within 50 turns.

**Pass criteria**

| ID | Name | Intent |
|---|---|---|
| G1 | Mechanism | Markers parse; rewrites fire; post-rewrite shrink within tolerance |
| G2 | Compliance | Tail/tool spans receive ignore markers as expected |
| G3 | Quality | DynCLM success ≥ baseline and ≥ immediate (harness task success) |
| G4 | Cost | DynCLM rewrite prefill **lower** than immediate-rewrite (N-minimizing) |

**Result.** Gate **PASS**. DynCLM avg context **−78.8%** vs raw baseline; rewrite prefill **38.6% lower** than immediate-rewrite (17415 vs 28363). Wall-clock here is ~0 (synthetic) — not used for speed claims.

Details: [interim-report.md](interim-report.md), `results/gate/report.json`.

### 1.3 Live graded quality (single-shot / short multi-span)

**What it is.** 18 deterministic tasks (retention, constraint, recovery, coding, adversarial FACT/coding) run **live** on oMLX. Distractor spans are soft-invalidated for DynCLM/CLM; answers graded by bidirectional substring match after stripping CoT.

**Why it exists.** Checks that ignore markers help answer correctness when obsolete text conflicts with the authoritative FACT — without requiring a long growing session.

**Limitation.** Context length is essentially flat across arms (~162 tokens): **no multi-turn reclaim**, so it cannot show size→speed. Wall-clock can look *worse* for DynCLM (+44% here) because of marker/prompt overhead on tiny contexts.

**Result.** Accuracy 94.4% → **100%** (+5.6 pp) for DynCLM and CLM vs baseline.

Details: [quality-report.md](quality-report.md), `results/quality/report.json`.

### 1.4 Offline unit / mechanism benchmarks (M1–M2)

Internal packages under `internal/eval/benchmarks` and `contextmgr` tests cover marker parse/apply, dead-span tracking, rewriter SCR accounting, and compliance bench-3. These are CI-facing correctness checks, not live model scores.

---

## 2. Plan total run (Phase B learning grid) — status

**Plan design** (`configs/learning.yaml`):

| Axis | Values | Count |
|---|---|---|
| Models | Qwen3.8-27B-4bit, Qwen3.6-35B-A3B-4bit, North-Mini-Code-1.0-4bit | 3 |
| Task types | coding, search, multi-agent | 3 |
| Policies | β/γ grid (5 named policies) | 5 |
| Sessions / cell | 20 | 20 |
| **Nominal total** | | **3 × 3 × 5 × 20 = 900 sessions** |
| Turns / session | 50 | |
| max_context | 32768 | |
| `use_live_model` | true (as written) | |

**What actually ran.** Phase B did **not** complete as a full live 900-session oMLX grid. Artifacts under `results/learning/` are limited (`scaling_fit.json`, `trace.jsonl`); `results/phaseb-run.log` only records final-report generation. The automatic post-gate path continued into report writing using **offline/synthetic** session accounting and the short + long-horizon suites above — not 900 live multi-model cells.

**Implication.** Policy/`ρ` learning from the full grid is **incomplete**. Ship recommendation below rests on gate PASS + live quality + live long-horizon A (+ suite B matrix), not on a finished 900-cell live sweep.

To run the planned grid (expensive): `make learning` with oMLX up and `configs/learning.yaml` unchanged — expect multi-hour wall time on M5 Max.

---

## 3. Long-horizon A — current live suite (vs raw baseline)

**Claim under test.** In long sessions, obsolete tool noise accumulates; keeping context small via soft-invalidation + deferred rewrite improves (1) late-session probe quality and (2) steady-state generation speed vs raw append-only.

**Protocol**

1. Plant authoritative `[FACT]` at turn 1 (`CANARY_*`).
2. Each turn appends large obsolete tool noise + competing pseudo-FACT (`WRONG_t`).
3. DynCLM/CLM: harness applies `<<<DYNCLM_IGNORE>>>` on obsolete tool spans; cost model decides rewrite.
4. Baseline: no markers — instruction says use most recent FACT → recency trap.
5. Probes every N turns ask for the canary; grade exact token presence.
6. Metrics per turn: context length, prompt/generated tokens, wall-clock, gen tok/s.
7. Windows: early (first third) vs late (last third); **late_steady** excludes rewrite spikes (except immediate-rewrite, where every turn *is* steady state).

**Run.** Live oMLX `Qwen3.8-27B-4bit`, 2 sessions × 20 turns × 3 arms. See [longhorizon-report.md](longhorizon-report.md).

| Dimension | DynCLM vs baseline | CLM (immediate) vs baseline |
|---|---|---|
| Late probe quality | **+100 pp** (0% → 100%) | +100 pp |
| Late avg context | **−64.3%** (5666 → 2024) | −94% |
| Late avg prompt tokens | **−54.5%** | −84% |
| Late steady wall-clock | **−6.5%** (3.96s → 3.70s) | **−51.8%** |
| Late steady gen tok/s | **+20.7%** (1.13 → 1.36) | **+109%** |
| Rewrite prefill / session | worse (0 → 144) | worse (0 → 1772) |

CLM is the speed upper bound (always-tiny context, high N). DynCLM keeps most of the quality win and a large context cut with far fewer rewrites (~1 vs ~20/session).

---

## 4. Long-horizon B — compaction baseline × max context

**Motivation.** Production systems rarely keep unbounded raw context; they **compact** (truncate / summarize) when hitting a window. Suite A’s raw baseline is the scientific lower bound on quality; suite B asks whether DynCLM/CLM still win when the baseline is allowed to compact.

**Protocol (delta from A)**

- Arms: `baseline-compact`, `immediate-rewrite` (CLM), `dynclm`.
- When `baseline-compact` exceeds `max_context`, drop oldest spans (keep `sys` when possible) down to ~80% of budget; charge full re-prefill of survivors.
- Same `max_context` is the DynCLM scheduler ceiling (`β · max_context`).
- Matrix: **8192, 16384, 32768, 65536**.
- Per-column noise ≈ 1.4× that limit over the session so compaction **must** fire in every column.

**Live matrix** — 1 session × 16 turns × 3 arms × 4 limits on oMLX `Qwen3.8-27B-4bit` (`results/longhorizon_compaction_live/`). Full tables: [longhorizon-compaction-report.md](longhorizon-compaction-report.md).

| Max context | compact probe% | DynCLM | CLM | compact late ctx | DynCLM | CLM | compact wall-s | DynCLM | CLM |
|---|---|---|---|---|---|---|---|---|---|
| 8192 | 0 | 100 | 100 | 6980 | 2488 | 263 | 6.09 | 7.76 | **1.56** |
| 16384 | 0 | 100 | 100 | 14169 | 4639 | 263 | 11.88 | **7.58** | **1.82** |
| 32768 | 0 | 100 | 100 | 29120 | 8938 | 263 | 7.70 | 10.53 | **1.75** |
| 65536 | 0 | 100 | 100 | 57804 | 17540 | 263 | 3.94 | 13.52 | **1.83** |

Blind compaction drops the early FACT → late probes **0%**; DynCLM/CLM keep **100%**. DynCLM late context is ~1/3 of the compacting baseline; CLM is the speed upper bound. Compaction can post a lower wall-clock while answering wrong — quality-adjusted, DynCLM/CLM dominate.

Offline replica (2×24 turns, accounting only) remains under `results/longhorizon_compaction/` for regression.

Commands:

```bash
# Suite A
make longhorizon

# Suite B offline (fast)
./bin/dynclm-longhorizon -mode b -offline -fresh -sessions 2 -turns 24

# Suite B live
./bin/dynclm-longhorizon -mode b -sessions 1 -turns 16 -limits 8192,16384,32768,65536 \
  -results results/longhorizon_compaction_live

make charts
```

---

## Executive summary

| Suite | vs reference | Headline |
|---|---|---|
| Gate (short, synthetic) | raw baseline | Context −79%; G1–G4 PASS; prefill vs CLM −39% |
| Quality (short, live) | raw baseline | Accuracy +5.6 pp; no context-size win |
| Phase B 900-grid | — | **Not fully run live** |
| Long-horizon A (live) | raw baseline | Late quality +100 pp; ctx −64%; steady tok/s +21% |
| Long-horizon B (live matrix) | compaction baseline | Late probes 0%→100%; ctx ~1/3 of compact; CLM fastest wall |

**Bottom line.** On the paper-relevant long-horizon setting, DynCLM beats raw baseline on late quality, context, and steady generation speed, and still beats a compacting baseline on late quality while holding context well below the window. Immediate-rewrite (CLM) is faster still at much higher rewrite N.

---

## Ship / no-ship (§7.3)

- Recommendation: **ship** (tradeoff explicit)
- Late-session quality ≥ baseline: **yes** (suite A + B)
- Late-session context/prompt vs baseline: **yes**
- Late steady generation speed vs baseline: **yes** (suite A live)
- Rewrite prefill vs baseline: **no** (baseline is 0; DynCLM pays reclaim cost)
- Full plan 900-session live grid: **outstanding**

## Gaps

- Phase B 900-session live learning grid not completed.
- Not SWE-bench / BrowseComp-Plus paper reproduction.
- oMLX has no span-level KV API; prefill is logical/SCR-accounting.
- Suite B live used n=1 session/arm; wall/tok/s are directional.

## Artifacts

- **Figures:** `docs/figures/*.svg`
- **Long-horizon A:** `results/longhorizon/report.json`, [longhorizon-report.md](longhorizon-report.md)
- **Long-horizon B:** `results/longhorizon_compaction/`, [longhorizon-compaction-report.md](longhorizon-compaction-report.md)
- Gate: `results/gate/report.json`, [interim-report.md](interim-report.md)
- Quality: `results/quality/report.json`, [quality-report.md](quality-report.md)
- Verification: [verification.md](verification.md)
