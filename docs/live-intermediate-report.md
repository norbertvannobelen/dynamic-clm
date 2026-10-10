# DynCLM Live Intermediate Report

**Learning-900 run:** `results/runs/20261009T034520Z` — **paused & archived** at ~207/900  
**Snapshot:** `archive_learning_900_*` under that run · resume: `bash scripts/resume_live_full.sh results/runs/20261009T034520Z`  
**Active:** efficient **decision suite** (`make decision-suite` → `docs/decision-report.md`)  


Primary question for this note: **Baseline vs DynCLM vs CLM on speed and quality.**  
Secondary: what the 900-session grid is for, and whether a cheaper suite can answer the primary question.

---

## 1. Three-arm comparison (use this for Baseline → DynCLM → CLM)

### 1.1 Live gate — speed / cost (this run)

20 sessions × 50 turns × 3 arms, `Qwen3.8-27B-4bit`, `live_model: true`.  
Gate “success” = harness task completion (100% all arms) — **weak quality signal**.

| Metric | Baseline | DynCLM | CLM (immediate) |
|---|---:|---:|---:|
| Success | 100% | 100% | 100% |
| Avg context | 14,452 | 6,669 (**−54%**) | 4,985 (**−66%**) |
| Peak context | 28,382 | 15,359 | 10,753 |
| Wall-clock / session | 551 s | 468 s (**−15%**) | 461 s (**−16%**) |
| Rewrite prefill / session | 0 | 6,722 | 27,934 |
| Rewrites / session | 0 | 5.0 | 19.6 |

**Read:** Both reclaim arms beat baseline on context and session wall-clock. CLM is slightly smaller/faster than DynCLM but pays ~4× DynCLM’s rewrite prefill.

### 1.2 Live graded quality (prior archive; this run not yet re-run)

18 tasks, live answers:

| Arm | Accuracy | Wall / task |
|---|---:|---:|
| Baseline | 94.4% (17/18) | 0.95 s |
| DynCLM | **100%** (18/18) | 1.37 s |
| CLM | **100%** (18/18) | 1.35 s |

**Read:** DynCLM ≈ CLM on graded accuracy; both **+5.6 pp** vs baseline. Single-shot tasks do **not** show the context-size win.

### 1.3 Live long-horizon A (prior archive) — best quality+speed joint test

Recency-trap sessions; late window:

| Arm | Late probe accuracy | Late avg ctx | Late steady wall | Late tok/s |
|---|---:|---:|---:|---:|
| Baseline | **0%** | 5,666 | 3.96 s | 1.13 |
| DynCLM | **100%** | 2,024 | 3.70 s | 1.36 |
| CLM | **100%** | 320 | 1.91 s | 2.35 |

**Read:** This is the paper-relevant quality separation (0%→100%) plus steady-state speed. CLM is the speed upper bound; DynCLM keeps the quality win with far fewer rewrites.

### 1.4 Live long-horizon B — compaction baseline (prior archive)

At 8k/16k/32k/64k: compaction baseline late probes **0%**; DynCLM/CLM **100%**. DynCLM late context ~⅓ of compacting baseline; CLM ~260 tokens.

---

## 2. Learning grid status (not a 3-arm bake-off)

| | |
|---|---|
| Progress | **~207/900 (23%)** |
| Done | All `Qwen3.8` × coding + search (5 policies × 20); multi-agent started |
| Remaining | Rest of multi-agent + 2 other models × 3 tasks × 5 policies |
| Purpose (original plan §6.3) | Fit DynCLM **policy** response surface (β/γ/Δ × model × task), not Baseline vs CLM |

Sample (DynCLM-only, completed policy blocks on Qwen3.8): lower β → more rewrites / smaller avg context; all blocks 100% harness success so far. **Cannot** answer Baseline vs DynCLM vs CLM from this grid alone.

---

## 3. Can we use a more efficient benchmark for the 3-arm target?

**Yes — and we largely already have one.** The 900-session grid is the wrong tool for “Baseline vs DynCLM vs CLM on speed and quality.”

| Suite | Answers 3-arm speed? | Answers 3-arm quality? | Cost (order of magnitude) |
|---|---|---|---|
| Gate (20×50×3) | Yes (session wall, ctx, prefill) | Weak (harness success) | ~hours |
| Graded quality (18×3) | Weak (tiny contexts) | Yes (+5.6 pp) | ~minutes |
| Long-horizon A (e.g. 3×24×3) | Yes (late wall / tok/s) | **Yes** (late probes) | ~1–3 hours |
| Long-horizon B matrix | Yes under window limits | Yes vs compaction | ~few hours |
| Learning 900 | No (DynCLM policies only) | No | **~days–week** |

### Recommended “decision suite” (efficient)

To decide ship/positioning on **speed + quality** vs baseline and CLM, run (or re-run) only:

1. **Long-horizon A** — 3 sessions × 24 turns × 3 arms (primary quality + late speed)  
2. **Graded quality** — 18 × 3 (secondary accuracy)  
3. **Gate or a shortened gate** — 5–10 sessions × 30–50 turns × 3 arms (session-level cost: ctx, prefill, wall)  
4. Optional: **LH-B** at 2 limits (e.g. 16k + 64k) if compaction baseline matters  

**Skip or pause the remaining ~700 learning sessions** if the goal is only the 3-arm comparison. Keep learning only if you still want β/γ regime selection across models/tasks.

### Estimated savings

- Full remaining learning ≈ **70–100 hours** at current pace  
- Decision suite above ≈ **half a day** of live oMLX  
- You already have live gate (this run) + prior live quality/LH; a **fresh** decision suite in this run archive would mainly refresh quality/LH under the same software revision

---

## 4. Bottom line (from data in hand)

| Dimension | Winner vs baseline | DynCLM vs CLM |
|---|---|---|
| Late-session **quality** | DynCLM & CLM (0%→100% probes; +5.6 pp graded) | Tie on quality |
| **Speed** (late / session wall) | Both beat baseline; CLM fastest late | CLM faster; DynCLM close on gate wall |
| **Reclaim cost** | Both pay prefill; baseline pays 0 | **DynCLM much cheaper** (~76% less prefill than CLM on gate) |

**Recommendation:** Treat long-horizon A + graded quality + (short) gate as the efficient 3-arm benchmark. Treat the 900-grid as optional policy tuning, not as the path to Baseline/DynCLM/CLM verdicts.
