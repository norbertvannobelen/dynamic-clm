# Dynamic CLM (DynCLM)

A context-management method for long agent sessions: obsolete context is marked dead with append-only soft-invalidation markers, and a cost-model scheduler fires a single batched physical rewrite when the deferred cost of carrying dead tokens exceeds the fixed cost of rewriting. This repository contains the Go implementation, the evaluation harnesses, and the live evidence collected on Apple Silicon (M5 Max, oMLX, `Qwen3.8-27B-4bit`).

## Abstract

Long agent sessions fill the context window with obsolete tool output, wrong drafts, and stale facts, so some mechanism has to reclaim that space. CLM (Context Language Models [1]) reclaims selectively and correctly - the model decides what dies and survivors stay verbatim - but pays a physical context rewrite and re-prefill on every edit. DynCLM tests whether that payment schedule can be changed without giving up the reclaim quality: keep CLM's mark, drop, verbatim-survivors methodology, but decouple the marking (append-only, zero re-prefill) from the physical drop (one batched rewrite, fired by a calibrated cost model). Measured live on an Apple M5 Max, DynCLM matched CLM on every quality canary while cutting rewrite events 3.4x-24x and rewrite prefill 71-95% depending on suite, and it beat both raw append-only and summarize-compact baselines on late-session fact retention. Method and math: [docs/method.md](docs/method.md) [2]. Full evaluation: [docs/report.md](docs/report.md) [3].

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

| Suite (live) | Reference | Headline result |
|---|---|---|
| Short gate, 8 sessions x 40 turns | Raw | **PASS** G1-G4. Avg context -53.8%, session wall -9.0%; rewrite prefill -71.1% vs CLM (5.0 vs 17.0 rewrites/session) |
| Graded quality, 18 tasks | Raw | 94.4% -> 100% (+5.6 pp); ties CLM |
| Long-horizon A, 3 x 24 turns | Raw + recency | Late FACT probe 0% -> 100%; late context -70.8%; 1 vs 24 rewrites/session vs CLM |
| Long-horizon B, 2 x 20 @ 16k/64k | Summarize compact | Late FACT probe 0% -> 100% at both limits; late wall beats the compact baseline |
| SWE-Distill, 6 tasks | Summarize compact | Final probe 50% -> 100% (+50 pp); `spec_survive` 100% on all arms; wall 0.90h -> 0.34h |

Suite write-ups: gate [6], quality [6], long-horizon A/B [6], SWE-Distill [6]; rolled-up tables in the decision report [5]; figures in [docs/figures/](docs/figures/).

## Conclusion

On the measured stack (M5 Max, oMLX, `Qwen3.8-27B-4bit`):

- **H1 supported.** Rewrite prefill fell 71.1% vs CLM at the gate (5,340 vs 18,504 tokens/session; 5.0 vs 17.0 rewrites), 94.8% in long-horizon A (1 vs 24 rewrites/session), and 75.6% in SWE-Distill (2 vs 28 rewrites/task). DynCLM carries a larger average context than CLM between rewrites, exactly as the session-load model predicts [2][3].
- **H2 supported.** Quality parity with CLM on every canary (100% ties on gate, graded quality, long-horizon A/B, SWE-Distill) and at or above the raw baseline everywhere; late FACT probes went 0% -> 100% against both raw and summarize-compact baselines [3].
- **H3 partially supported.** `ρ` was measured on M5 Max and drives the shipped scheduler; transfer to M5 Pro and base M5 profiles remains open [3][7].
- **Corollary resolved for this SKU.** Always-rewrite CLM ran without difficulty on M5 Max - it posted the fastest late-session wall-clock in long-horizon A - so a blanket "CLM is unsuitable for Mac" verdict is not supported here. DynCLM still removes about 71% of CLM's rewrite prefill at equal quality, and its per-device `ρ` profile is the lever for SKUs where the bandwidth ceiling may actually bind [3].

**Verdict:** ship DynCLM reclaim for this local oMLX/M5 Max stack. Honest tradeoffs: reclaim prefill is an added cost versus raw append-only (raw pays none); DynCLM is often slower than CLM on late steady wall-clock; total prompt tokens can exceed raw [3].

## Limitations

Single model and single machine for all ship evidence; M5 Pro / base M5 hardware profiles untested. Logical rather than physical cache accounting (no span-level KV API on oMLX). Canaries are not TerminalBench, BrowseComp-Plus, EdgeBench, or official SWE-Bench, and the CLM paper's numbers remain reproduction targets, not reproduced results [3][4]. The 900-cell policy-learning grid is paused at 207/900 and was not required for the verdict. Wall-clock and tok/s deltas on small n are directional. Full gap list: [docs/report.md](docs/report.md) [3].

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
8. Raw artifacts: `results/runs/20261010T145959Z_decision` (gate, quality, LH-A), `results/runs/20261013T201630Z_lh_compaction_summary` (LH-B), `results/runs/20261013T190132Z_swe_distill_summary_baseline` (SWE-Distill).

---

**Naming:** spell out "Dynamic CLM" or use **DynCLM** in writeups. Do not abbreviate as DCLM - that name is taken by DataComp-LM.
