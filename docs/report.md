# DynCLM Final Report

Generated: 2026-10-09T00:55:30Z

## How to read this report

**Reference arm = raw baseline** (append-only, no markers, no rewrites).  
Every efficiency claim below is DynCLM (or immediate-rewrite) **versus that baseline**, unless explicitly labeled “vs immediate rewrite.”

Two complementary suites:

1. **Multi-turn gate** (20 sessions × 50 turns, synthetic agent, full context growth) — best for **context length / prompt / rewrite-prefill** efficiency.
2. **Live graded quality** (18 ground-truth tasks on oMLX `Qwen3.8-27B-4bit`) — best for **answer quality**, plus single-request prompt/generated/wall-clock.

## Executive summary vs raw baseline

| Dimension | DynCLM vs baseline | Notes |
|---|---|---|
| **Quality (live graded accuracy)** | **+5.6 pp** (94.4% → 100%) | See [quality-report.md](quality-report.md) |
| **Avg context length (gate)** | **−78.8%** (13476 → 2853 tokens) | Rewrites reclaim dead tool spam |
| **Peak context length (gate)** | **−81.7%** (26385 → 4830) | |
| **Final context length (gate)** | **−86.0%** (26385 → 3707) | |
| **Prompt tokens (gate, est.)** | **−78.6%** (676050 → 144915) | Follows shorter context |
| **Generated tokens (gate, est.)** | **0%** (454 → 454) | Same reply style |
| **Rewrite prefill tokens (gate)** | **worse** (0 → 17415) | Baseline never rewrites |
| **Rewrites (gate)** | **worse** (0 → 12) | Expected cost of DynCLM |
| **Wall-clock (live quality)** | **+44%** (0.95s → 1.37s) | Ignore-marker prompts slightly longer |
| **Prompt tokens (live quality)** | **+35%** (406 → 548) | Markers add tokens in single-shot tasks |

**Bottom line:** vs raw baseline, DynCLM is **higher quality** on the live suite and **much leaner on context/prompt** in long multi-turn sessions, but it **pays rewrite prefill** the baseline never pays. It is **not** free vs doing nothing.

## Multi-turn gate — full metrics vs baseline

Source: `results/gate/report.json` / [interim-report.md](interim-report.md). Model config: `Qwen3.8-27B-4bit`, 20×50 turns.

### Absolute values

| Arm | Success% | Avg ctx | Peak ctx | Final ctx | Prefill | Prompt | Generated | Rewrites |
|---|---|---|---|---|---|---|---|---|
| baseline | 100.0 | 13476 | 26385 | 26385 | 0 | 676050 | 454 | 0 |
| immediate-rewrite | 100.0 | 2409 | 4633 | 3244 | 28363 | 122706 | 454 | 26 |
| dynclm | 100.0 | 2853 | 4830 | 3707 | 17415 | 144915 | 454 | 12 |

### DynCLM vs raw baseline

| Metric | Baseline | DynCLM | Δ | % change | Better? |
|---|---|---|---|---|---|
| accuracy_pct | 100.00 | 100.00 | +0.00 | +0.0% | yes (tie) |
| avg_context_tokens | 13475.64 | 2852.94 | −10622.70 | **−78.8%** | yes |
| peak_context_tokens | 26385.00 | 4830.00 | −21555.00 | **−81.7%** | yes |
| final_context_tokens | 26385.00 | 3707.00 | −22678.00 | **−86.0%** | yes |
| avg_dead_tokens | 23141.00 | 463.00 | −22678.00 | **−98.0%** | yes |
| avg_prompt_tokens | 676050.00 | 144915.00 | −531135.00 | **−78.6%** | yes |
| avg_generated_tokens | 454.00 | 454.00 | +0.00 | +0.0% | yes (tie) |
| avg_prefill_tokens | 0.00 | 17415.00 | +17415.00 | n/a | **no** |
| avg_rewrites | 0.00 | 12.00 | +12.00 | n/a | **no** |
| avg_dead_carried_turn_sum | 589911.00 | 58776.00 | −531135.00 | **−90.0%** | yes |

Vs **immediate rewrite** (secondary): DynCLM re-prefill is **38.6% lower** (17415 vs 28363) with fewer rewrites (12 vs 26).

## Live graded quality — full metrics vs baseline

Source: [quality-report.md](quality-report.md), `results/quality/report.json`. Live oMLX, 18 tasks.

| Arm | Accuracy | Avg ctx | Prefill | Prompt | Generated | Wall-s |
|---|---|---|---|---|---|---|
| baseline | 94.4% (17/18) | 162 | 0 | 406 | 6 | 0.95 |
| immediate-rewrite | 100% (18/18) | 162 | 0 | 548 | 6 | 1.35 |
| dynclm | 100% (18/18) | 162 | 0 | 548 | 6 | 1.37 |

| Metric | vs baseline |
|---|---|
| Quality | **+5.6 pp** |
| Generated tokens | ~0% |
| Prompt tokens | +35% (markers in prompt) |
| Wall-clock | +44% |
| Context length | ~0% (single-shot; no multi-turn reclaim) |

Single-shot quality tasks do **not** show the gate’s context-shrink win; they mostly measure whether ignore markers help answer correctness.

## Calibration (M5 Max)

| Metric | Value |
|---|---|
| r (ms/token) | 2.787 |
| k_d | 0.0376 |
| C_fixed (ms) | 1111 |
| ρ | 29538 |

## Ship / no-ship (§7.3)

- Recommendation: **ship** (with clear tradeoff statement)
- Quality ≥ baseline: **yes** (+5.6 pp live)
- Long-horizon context/prompt efficiency vs baseline: **yes** (~−79% context/prompt in gate)
- Pure rewrite-prefill vs baseline: **no** (baseline is 0)
- Pure wall-clock vs baseline on single-shot live tasks: **no** (+44%)

## Gaps

- Gate multi-turn wall-clock is near-zero (synthetic agent); live multi-turn wall-clock vs baseline still needed for server timing.
- Not SWE-bench / BrowseComp-Plus paper reproduction.
- oMLX has no span-level KV API; prefill is logical/SCR-accounting.

## Artifacts

- Gate: `results/gate/report.json`, [interim-report.md](interim-report.md)
- Quality: `results/quality/report.json`, [quality-report.md](quality-report.md)
- Verification: [verification.md](verification.md)
