# DynCLM Final Report

Generated: 2026-10-09T00:55:30Z

## How to read this report

**Reference arm = raw baseline** (append-only, no markers, no rewrites).  
Every efficiency claim below is DynCLM (or immediate-rewrite) **versus that baseline**, unless explicitly labeled “vs immediate rewrite.”

### Methodology re-evaluation

The paper claim is specifically about **long-running sessions**: smaller context → better late-session quality and faster generation. Prior suites were incomplete for that claim:

| Suite | What it showed | Gap |
|---|---|---|
| Gate (synthetic multi-turn) | Context/prefill accounting | No live decode latency or live late quality |
| Quality (live single-shot) | Graded accuracy | No growing context → cannot show size→quality/speed |
| **Long-horizon (new, live)** | Late probe accuracy, ctx, wall-clock, tok/s vs turn | Matches the paper setting |

See [longhorizon-report.md](longhorizon-report.md).

## Executive summary vs raw baseline

### Long-horizon live (paper-relevant; primary)

2 sessions × 20 turns × 3 arms on oMLX `Qwen3.8-27B-4bit`. Late window = turns 13–20.

| Dimension | DynCLM vs baseline | Immediate-rewrite vs baseline |
|---|---|---|
| **Late probe quality** | **+100 pp** (0% → 100%) | +100 pp (0% → 100%) |
| **Late avg context** | **−64.3%** (5666 → 2024) | −94% (5666 → 320) |
| **Late avg prompt tokens** | **−54.5%** (3408 → 1549) | −84% |
| **Late steady wall-clock** | **−6.5%** (3.96s → 3.70s) | **−51.8%** (3.96s → 1.91s) |
| **Late steady gen tok/s** | **+20.7%** (1.13 → 1.36) | **+109%** (1.13 → 2.35) |
| **Rewrite prefill / session** | worse (0 → 144) | worse (0 → 1772) |

Immediate-rewrite is the speed upper bound (always-tiny context, high N). DynCLM keeps most of the quality win and a large context cut with far fewer rewrite events (1 vs 20/session).

### Other suites (secondary)

| Dimension | DynCLM vs baseline | Notes |
|---|---|---|
| Single-shot quality accuracy | +5.6 pp | [quality-report.md](quality-report.md) |
| Gate avg context (synthetic) | −78.8% | Accounting only |
| Gate rewrite prefill | worse (0 → 17k) | Baseline never rewrites |

**Bottom line:** On the long-horizon live suite that matches the paper setting, DynCLM beats raw baseline on **late quality**, **context size**, **prompt tokens**, and **steady-state generation speed**, while paying a small rewrite-prefill cost.

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

- Recommendation: **ship** (tradeoff explicit)
- Late-session quality ≥ baseline: **yes** (+100 pp on adversarial long-horizon probes)
- Late-session context/prompt vs baseline: **yes** (−64% / −55%)
- Late steady generation speed vs baseline: **yes** (+21% tok/s, −6.5% wall)
- Rewrite prefill vs baseline: **no** (baseline is 0; DynCLM pays ~144 tokens/session here)
- Immediate-rewrite is faster still, but at ~20× rewrite events — DynCLM is the N-minimizing middle

## Gaps

- Gate multi-turn wall-clock is near-zero (synthetic agent); live multi-turn wall-clock vs baseline still needed for server timing.
- Not SWE-bench / BrowseComp-Plus paper reproduction.
- oMLX has no span-level KV API; prefill is logical/SCR-accounting.

## Artifacts

- **Long-horizon (primary):** `results/longhorizon/report.json`, [longhorizon-report.md](longhorizon-report.md)
- Gate: `results/gate/report.json`, [interim-report.md](interim-report.md)
- Quality: `results/quality/report.json`, [quality-report.md](quality-report.md)
- Verification: [verification.md](verification.md)
