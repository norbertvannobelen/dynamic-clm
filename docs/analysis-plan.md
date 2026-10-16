# Analysis plan (pre-registration)

**Registered:** 2026-10-15, before the powered re-runs described below. Any analysis that
deviates from this document must be labeled exploratory in the report.

Companion: [report.md](report.md) (results), [method.md](method.md) (protocols).

## Why this exists

The 2026-10-10/13 ship evidence (runs `20261010T145959Z_decision`,
`20261013T201630Z_lh_compaction_summary`, `20261013T190132Z_swe_distill_summary_baseline`)
established direction and mechanism but was under-powered on the decisive canary suites
(LH-A n=3 sessions, LH-B n=2 sessions, SWE-Distill n=6 tasks; quality margin = 1 task).
This plan fixes sample sizes, tests, and decision rules **before** collecting the
powered dataset, so the public write-up's claims do not depend on post-hoc choices.
The earlier runs are retained as an internal replication dataset and are reported as such.

## Design

Same harnesses, arms, models, hardware, and calibration as the ship evidence
(`configs/calibration/m5max-qwen38.json`; oMLX `Qwen3.8-27B-4bit`; Apple M5 Max).
Only sample sizes change. Session seeds/task order follow the existing harness defaults;
no run is excluded after inspection except for documented infrastructure failures
(oMLX crash, thermal throttle event logged by the harness), which are retried.

| Suite | Change | New n (per arm) |
|---|---|---|
| Short gate | sessions 8 -> 16 | 16 sessions x 40 turns |
| Long-horizon A | sessions 3 -> 8 | 8 sessions x 24 turns |
| Long-horizon B | sessions 2 -> 10 | 10 sessions x 20 turns, at both limits (16384, 65536) |
| SWE-Distill | tasks 6 -> 12 | 12 tasks x 28 turns |

## Endpoints

### Primary (ship decision rests on these)

1. **LH-A late FACT probe accuracy**, DynCLM vs raw baseline.
2. **LH-B late FACT probe accuracy**, DynCLM vs summarize-compact baseline, at each
   max-context limit separately.
3. **SWE-Distill final probe accuracy**, DynCLM vs summarize-compact baseline.
4. **Gate rewrite prefill per session**, DynCLM vs CLM (immediate rewrite).

### Secondary (reported with CIs; no ship decision rests on them)

- Gate session wall-clock and prompt tokens; LH late steady wall-clock and gen tok/s;
  average/peak context; SWE wall-clock and prefill; graded-quality accuracy
  (parity check only, not a reclaim test).

## Unit of analysis and tests

- **Session (or task) is the unit of analysis.** Probes within a session share a canary
  and are not independent. A session/task counts as **success iff all of its
  late-window (or final) probes are correct** - the strict criterion the prior reports
  implicitly used when quoting 100%.
- **Proportions (primary endpoints 1-3):** one-sided Fisher exact test on the
  session/task-level success counts, alpha = 0.05, alternative = DynCLM > reference
  baseline. Wilson score 95% CIs reported for each arm's success proportion.
  Probe-level proportions are reported descriptively with Wilson CIs and an explicit
  within-session-correlation caveat.
- **Continuous metrics (primary endpoint 4 and all secondary):** Welch two-sample
  t-test (two-sided, alpha = 0.05) plus percentile-bootstrap 95% CI (10,000 resamples)
  of the mean difference. Gate rewrite prefill DynCLM vs CLM is tested one-sided
  (alternative = DynCLM < CLM), matching hypothesis H1.
- **Multiplicity:** the four primary endpoints are a confirmatory family; significance
  is assessed per-endpoint at alpha = 0.05 with Holm correction across the family.
  Secondary endpoints are unadjusted and labeled exploratory.

## Power justification

- LH-B, n=10 vs prior direction (baseline 0%): if the true baseline per-session pass
  rate is <= 0.2 and DynCLM's is 1.0, P(reject) >= 0.99; the boundary outcome
  10/10 vs 0/10 gives Fisher p = 5.4e-6.
- SWE-Distill, n=12: decisive at alpha = 0.05 if baseline passes <= 8/12 when DynCLM
  passes 12/12 (Fisher one-sided p <= 0.047). Prior rate was 3/6.
- Gate rewrite prefill: effect is mechanism-driven and large (-71% at n=8, disjoint
  rewrite-count ranges 2-9 vs 14-20); n=16 gives Welch power > 0.99 for an effect of
  half that size.
- Gate wall-clock: achieving 80% power for the previously observed 9% difference would
  require n ~ 94/arm (~31 h), which is infeasible. Wall-clock is therefore
  **secondary**: reported with bootstrap CIs, and headline wording must not claim a
  wall-clock win unless the CI excludes 0.

## Extension rule

If exactly one primary endpoint is inconclusive (Fisher p in (0.05, 0.20] with the
point estimate favoring DynCLM), that suite may be extended once: LH-B to 15
sessions/arm, SWE-Distill to 18 tasks, gate to 24 sessions/arm. The extension is
pre-committed here; no second extension is permitted, and the extended analysis pools
all pre-registered cells (same methodology, same calibration).

## Ship criteria (carried forward verbatim from report.md)

| Criterion | Threshold under this plan |
|---|---|
| Late FACT probe >= baseline (LH-A, LH-B) | Fisher one-sided p < 0.05 (Holm-adjusted family) and point estimate >= baseline |
| SWE final probe >= summarize baseline | same |
| Graded quality >= raw baseline | point estimate >= baseline (parity check; underpowered by design, not a ship blocker) |
| Late-session context <= baseline (A, B) | point estimate <= baseline with bootstrap CI reported |
| Session wall (gate) <= raw baseline | secondary; CI must exclude 0 for any headline claim |
| Late steady wall <= raw baseline (LH-A) | secondary; reported, expected to favor CLM |
| Late steady wall <= summarize baseline (LH-B) | secondary; CI reported |
| Rewrite prefill <= raw baseline | expected to fail (raw pays 0); reported |
| Rewrite prefill <= CLM (gate) | Welch one-sided p < 0.05 |
| Learning-900 complete | optional, not required |
| Paper TerminalBench / BrowseComp / EdgeBench | not run; claims scoped accordingly |
| Physical SCR on oMLX | not available; logical accounting disclosed |

## Reporting requirements

Every headline percentage in README.md and docs/report.md carries its n and a 95% CI;
every decisive comparison carries the pre-registered p-value. A "Statistical methods"
section in report.md links back to this document. Superseded/offline data lives only
under `results/superseded/` and is never cited.

## Amendments

**2026-10-15 (before any interleaved data was collected): session-major round-robin
execution.** The gate, quality, and long-horizon harnesses previously executed
arm-major (`for arm { for session }`), so slow hardware drift within a run
(e.g. sustained thermal load) systematically biased wall-clock for whichever arm ran
last. An aborted arm-major run at n=16 (run `20261015T141933Z_decision`, 22/48 gate
cells complete) showed exactly this: a ~12% upward wall-clock drift across the run
(duration-matched to the Oct 10 evidence, which used the same arm order). The harnesses
now iterate session-major round-robin (`for session { for arm }`), matching
SWE-Distill's existing task-major design, so drift is distributed approximately evenly
across arms within each session block. The aborted run is retained on disk
(git-ignored) and excluded from analysis. No endpoint, test, n, or ship criterion
changes; this strengthens the secondary wall-clock endpoint's validity and leaves the
primary (non-wall-clock) endpoints unaffected.
