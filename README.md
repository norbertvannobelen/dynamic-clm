# Dynamic CLM (DynCLM)

A context-management method for long agent sessions: obsolete context is marked dead with append-only soft-invalidation markers, and a cost-model scheduler fires a single batched physical rewrite when the deferred cost of carrying dead tokens exceeds the fixed cost of rewriting. This repository contains the Go implementation, the evaluation harnesses, and the live evidence collected on Apple Silicon (M5 Max, oMLX, `Qwen3.8-27B-4bit`).

## Abstract

Long agent sessions fill the context window with obsolete tool output, wrong drafts, and stale facts, so some mechanism has to reclaim that space. CLM (Context Language Models [1]) reclaims selectively and correctly - the model decides what dies and survivors stay verbatim - but pays a physical context rewrite and re-prefill on every edit. DynCLM tests whether that payment schedule can be changed without giving up the reclaim quality: keep CLM's mark, drop, verbatim-survivors methodology, but decouple the marking (append-only, zero re-prefill) from the physical drop (one batched rewrite, fired by a calibrated cost model). Measured live on an Apple M5 Max in powered, pre-registered benchmark runs, DynCLM cut rewrite events 3.5x-24x and rewrite prefill 71-95% depending on suite while holding quality parity with CLM on the gate and long-horizon canaries, and it beat both raw append-only and summarize-compact baselines on late-session fact retention (Holm-corrected Fisher p <= 7.8e-05 on every long-horizon endpoint). Method and math: [docs/method.md](docs/method.md) [2]. Full evaluation with confidence intervals and pre-registered tests: [docs/report.md](docs/report.md) [3].

## Background and motivation

**The reclaim problem is general.** Agent sessions accumulate obsolete context on any host. The two established reclaim families differ in *who decides what survives*: summarization/truncation folds old context into lossy notes under an external policy, while CLM lets the model itself mark known-dead spans and rewrites the context so survivors stay byte-for-byte verbatim [2].

**The cost of CLM is the rewrite schedule.** Every in-the-middle edit invalidates the prefix cache from the edit point onward, so the whole suffix - including unchanged survivors - is re-prefilled. A session with N edits pays N such rewrite/re-prefill events (the N-multiplier). Each rewrite also carries a fixed overhead `C_fixed` (KV span relocation, RoPE re-rotation, recurrent-state snapshots, diff computation, stop-the-world latency) that does not shrink as prefill gets faster. Rewrite count N is therefore a first-class cost multiplier, and on faster hardware the optimal N goes *down*, not up [2].

**The Mac angle is the cost of per-turn rewrite, not the staleness.** On unified-memory Apple Silicon, prefill is disproportionately expensive relative to decode - memory bandwidth is often the limit - which is why CLM is frequently called a poor fit for Mac. Testing that claim, and providing a mechanism if it holds, is the second motivation of this study: either always-rewrite CLM proves viable on this silicon, or reclaim timing must become hardware-aware.

**The observation behind DynCLM** is that the marking decision and the physical reclaim do not have to happen at the same time. The analogy is garbage collection: mark dead spans, keep running while the garbage is cheap to carry, then collect in one batch when pressure warrants it - instead of collecting on every allocation [2].

## Hypothesis

**H1 (deferral cuts rewrite cost).** Deferring the physical rewrite behind a cost model cuts rewrite events and rewrite prefill by roughly the deferral factor R relative to always-rewrite CLM, in exchange for carrying a larger average context between rewrites. This follows from the session-load math in [method.md](docs/method.md#dynclm-vs-clm-session-context-load) [2].

**H2 (quality survives the deferral window).** Carrying soft-invalidated spans across the deferral window does not drop answer quality below the raw append-only baseline, and stays within a small margin of CLM on selective-ignore canaries.

**H3 (reclaim timing is a hardware profile).** The break-even point is governed by `ρ = C_fixed / k_d` (fixed rewrite overhead over per-token-per-turn decode cost), measurable per device by microbenchmark. Reclaim timing can therefore be calibrated per hardware SKU rather than assumed; on narrower-bandwidth SKUs the optimal deferral window grows [2].

**Corollary (the Mac claim).** Running both arms on Apple Silicon directly tests the "CLM is a poor fit for Mac" claim on the measured SKU.

## Approach in brief

Three phases, append-only until the cost model fires [2]:

1. **Append** - normal operation; prefix cache fully preserved.
2. **Soft invalidation** - the session appends `<<<DYNCLM_IGNORE span-ID>>>` markers (revocable via `<<<DYNCLM_UNMARK span-ID>>>`). Still append-only; dead span bodies are stubbed out of served prompts, so billed prompt size shrinks before any rewrite.
3. **Deferred rewrite** - one batched physical drop of all dead spans when the policy fires. Survivors become the new context verbatim: the same physical step CLM performs, batched.

The trigger policy fires when context exceeds `β * max_context`, dead/context exceeds `γ`, at least `Δ` turns passed since the last rewrite, and the cost model prefers rewriting (`C_fixed + r * S` vs `T * k_d * dead`). Defaults: `β=0.6, γ=0.5, Δ=3`. Calibration measures `r`, `k_d`, `C_fixed` per device; on M5 Max: `r = 2.787 ms/token`, `k_d = 0.0376 ms/token/turn`, `C_fixed = 1111 ms`, `ρ ≈ 29,538` [7].

## Evaluation

**Stack.** Apple M5 Max (128 GB) running oMLX (OpenAI-compatible) with `Qwen3.8-27B-4bit`. oMLX exposes no span-level KV API, so rewrite accounting uses logical SCR-shaped span accounting (cap K=6) rather than physical KV relocation [3].

**Arms.** Raw append-only baseline; baseline + Codex-style rolling-summary compaction; CLM (soft-invalidation + rewrite on every edit); DynCLM (soft-invalidation + cost-model deferred rewrite). Each suite is compared only against its own reference baseline [3].

All headline numbers come from powered, pre-registered runs ([analysis plan](docs/analysis-plan.md)); proportions carry Wilson 95% CIs and decisive comparisons carry pre-registered Fisher/Welch p-values.

| Suite (live, powered n) | Reference | Headline result |
|---|---|---|
| Short gate, 16 sessions x 40 turns | Raw | **PASS** G1-G4. Rewrite prefill **-71.5%** vs CLM (5,010 vs 17,571 tok/session, Welch p = 1.4e-16; 4.9 vs 17.1 rewrites); avg context -56.2% |
| Graded quality, 18 tasks | Raw | 94.4% -> 100% (+5.6 pp); ties CLM (parity check) |
| Long-horizon A, 8 x 24 turns | Raw + recency | Late FACT probe 0/8 -> 8/8 (Fisher p = 7.8e-05); late context -70.8%; 1 vs 24 rewrites/session vs CLM |
| Long-horizon B, 10 x 20 @ 16k/64k | Summarize compact | Late FACT probe 0/10 -> 10/10 at both limits (Fisher p = 5.4e-06 each); late wall beats the compact baseline (CI excludes 0) |
| SWE-Distill, 18 tasks (12 + 6 pre-registered extension, pooled) | Summarize compact | Final probe 7/18 -> 15/18 (38.9% -> 83.3%; Fisher p = 0.0076); `spec_survive` 100% on all arms; wall 1.91h -> 0.86h (CI excludes 0) |

Suite write-ups: gate [6], quality [6], long-horizon A/B [6], SWE-Distill [6]; rolled-up tables in the decision report [5]; figures in [docs/figures/](docs/figures/).

## Conclusion

On the measured stack (M5 Max, oMLX, `Qwen3.8-27B-4bit`):

- **H1 supported.** Rewrite prefill fell 71.5% vs CLM at the gate (5,010 vs 17,571 tokens/session, Welch one-sided p = 1.4e-16; 4.9 vs 17.1 rewrites), 94.8% in long-horizon A (1 vs 24 rewrites/session), and 89.1% in SWE-Distill (1,161 vs 10,687 tokens/task; 2 vs 28 rewrites). DynCLM carries a larger average context than CLM between rewrites, exactly as the session-load model predicts [2][3].
- **H2 supported vs baselines; CLM keeps an edge on SWE-Distill.** DynCLM beat every reference baseline on every quality canary on the powered dataset: late FACT probes went 0% -> 100% against both raw and summarize-compact baselines (Holm-corrected Fisher p <= 7.8e-05), and SWE-Distill went 7/18 -> 15/18 vs the summarize baseline (pooled Fisher p = 0.0076 after the pre-registered extension). Quality parity with CLM holds on gate, graded quality, and long-horizon A/B (100% ties); on SWE-Distill, CLM's 18/18 beats DynCLM's 15/18 - the deferral window occasionally lets a soft-invalidated distractor win the probe [3].
- **H3 partially supported.** `ρ` was measured on M5 Max and drives the shipped scheduler; transfer to M5 Pro and base M5 profiles remains open [3][7].
- **Corollary resolved for this SKU.** Always-rewrite CLM ran without difficulty on M5 Max - it posted the fastest late-session wall-clock in every long-horizon suite - so a blanket "CLM is unsuitable for Mac" verdict is not supported here. DynCLM still removes about 71% of CLM's rewrite prefill at equal quality on the long-horizon canaries, and its per-device `ρ` profile is the lever for SKUs where the bandwidth ceiling may actually bind [3].

**Verdict:** ship DynCLM reclaim for this local oMLX/M5 Max stack. All four pre-registered primary endpoints are significant on the powered dataset after Holm correction (gate rewrite prefill p = 1.4e-16; LH-A p = 7.8e-05; LH-B p = 5.4e-06 at both limits; SWE-Distill pooled p = 0.0076). Honest tradeoffs: reclaim prefill is an added cost versus raw append-only (raw pays none); CLM remains the fastest arm on late steady wall-clock and the only arm at 100% on SWE-Distill; the gate session-wall difference favors DynCLM numerically but its confidence interval includes zero, so no wall-clock claim is made there [3].

## Limitations

Single model and single machine for all ship evidence; M5 Pro / base M5 hardware profiles untested. Logical rather than physical cache accounting (no span-level KV API on oMLX). Canaries are not TerminalBench, BrowseComp-Plus, EdgeBench, or official SWE-Bench, and the CLM paper's numbers remain reproduction targets, not reproduced results [3][4]. The 900-cell policy-learning grid is paused at 207/900 and was not required for the verdict. DynCLM trails CLM on SWE-Distill accuracy (15/18 vs 18/18). Gate session wall-clock is underpowered by ~6x for the observed effect, so wall-clock claims are limited to suites whose bootstrap CI excludes zero. Full gap list: [docs/report.md](docs/report.md) [3].

## Reproducing this study

Requires a running oMLX server with `Qwen3.8-27B-4bit`; set `OMLX_API_KEY` (or rely on `~/.omlx/settings.json`). The ship evidence is the 3-arm decision suite plus SWE-Distill:

```bash
make build && make test
make decision-suite   # gate + graded quality + long-horizon A/B
make swe-distill      # SWE-Distill reclaim harness
make charts           # regenerate docs/figures/*.svg
```

Full command list and suite descriptions: [docs/method.md](docs/method.md#commands) [2]. Raw run artifacts live under `results/runs/` [8].

## Repository layout

- `cmd/` - suite entry points (gate, calibrate, quality, longhorizon, swedistill, eval, baseline)
- `internal/scheduler` - trigger policy, cost model, device calibration, online adaptation
- `internal/contextmgr` - marker parser, dead-span tracker, rewriter, cache accounting
- `internal/eval` - gate, quality, long-horizon A/B, SWE-Distill, ablations
- `internal/session`, `internal/tracing`, `internal/metrics` - session runner, decision/counterfactual tracing, comparison metrics
- `internal/backend/omlx` - oMLX server client
- `configs/` - policies and per-device calibration (`ρ`)
- `docs/` - method, reports, figures
- `results/` - raw run artifacts (large run archives under `results/runs/` are git-ignored; published cells and benchmarks are tracked)

## References

1. Shao et al., *Context Language Models*, [arXiv:2609.37725](https://arxiv.org/abs/2609.37725); code: [facebookresearch/context-language-models](https://github.com/facebookresearch/context-language-models). Paper claims (35% server-side compute reduction from SCR, +11.4% BrowseComp-Plus, +5% EdgeBench at 59% fewer FLOPs) are treated as reproduction targets, not as results of this repo.
2. [docs/method.md](docs/method.md) - premise, reclaim protocols, cost model and session-load math, per-suite purpose, commands.
3. [docs/report.md](docs/report.md) - final live evaluation on M5 Max / oMLX: headlines, per-suite tables, ship criteria, gaps.
4. [docs/verification.md](docs/verification.md) - independent verification of the CLM paper identity and public claims.
5. [docs/decision-report.md](docs/decision-report.md) - rolled-up 3-arm decision tables (gate, quality, long-horizon A/B).
6. Suite reports: [interim-report](docs/interim-report.md) (gate + calibration), [quality-report](docs/quality-report.md), [longhorizon-report](docs/longhorizon-report.md) (A), [longhorizon-compaction-report](docs/longhorizon-compaction-report.md) (B), [swe-distill-report](docs/swe-distill-report.md).
7. [configs/calibration/m5max-qwen38.json](configs/calibration/m5max-qwen38.json) - measured `r`, `k_d`, `C_fixed`, `ρ` for the ship stack.
8. Raw artifacts (powered, pre-registered): `results/runs/20261015T165854Z_decision` (gate, quality, LH-A, LH-B), `results/runs/20261015T223506Z_swe_distill` (SWE tasks 1-12), `results/runs/20261016T005250Z_swe_distill_ext` (SWE tasks 13-18 + pooled report). Earlier Oct 10/13 runs are retained as a prior replication dataset. Pre-registration: [docs/analysis-plan.md](docs/analysis-plan.md).

---

**Naming:** spell out "Dynamic CLM" or use **DynCLM** in writeups. Do not abbreviate as DCLM - that name is taken by DataComp-LM.
