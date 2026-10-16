# DynCLM Decision Report - Baseline vs DynCLM vs CLM

**Run:** `20261015T165854Z_decision` · live=True · status=complete

Efficient decision suite (not the 900-session policy grid): short gate + graded quality + long-horizon A + compaction matrix (16k/64k).

## 1. Short live gate - session speed & cost

Sessions/arm: 16 · Turns: 40 · live_model=True · passed=True

| Metric | Baseline | DynCLM | CLM |
|---|---:|---:|---:|
| Success % | 100.0 | 100.0 | 100.0 |
| Avg context | 11785 | 5163 | 3905 |
| Peak context | 23482 | 12657 | 8369 |
| Wall-clock / session (s) | 371.7 | 359.6 | 339.9 |
| Rewrite prefill / session | 0 | 5010 | 17571 |
| Rewrites / session | 0.0 | 4.9 | 17.1 |
| Prompt tokens / session | 99579 | 90913 | 87609 |
| Generated tokens / session | 4516 | 4418 | 4180 |

### Vs baseline

| Metric | DynCLM | CLM |
|---|---|---|
| Avg context | -56.2% (11785 → 5163) | -66.9% (11785 → 3905) |
| Peak context | -46.1% (23482 → 12657) | -64.4% (23482 → 8369) |
| Wall-clock / session (s) | -3.3% (372 → 360) | -8.6% (372 → 340) |
| Rewrite prefill / session | - (0 → 5010) | - (0 → 17571) |
| Rewrites / session | - (0 → 5) | - (0 → 17) |
| Prompt tokens / session | -8.7% (99579 → 90913) | -12.0% (99579 → 87609) |
| Generated tokens / session | -2.2% (4516 → 4418) | -7.4% (4516 → 4180) |

### Uncertainty (per-session percentile-bootstrap 95% CI of the mean)

| Metric | Baseline | DynCLM | CLM |
|---|---:|---:|---:|
| Avg context | 11785 [11547, 12046] | 5163 [4792, 5514] | 3905 [3395, 4492] |
| Wall-clock / session (s) | 371.7 [347.0, 395.0] | 359.6 [336.5, 383.8] | 339.9 [313.2, 368.3] |
| Rewrite prefill / session | 0 [0, 0] | 5010 [4061, 5991] | 17571 [16468, 18600] |
| Prompt tokens / session | 99579 [97618, 101841] | 90913 [88865, 93158] | 87609 [85462, 89696] |

Rewrite prefill DynCLM vs CLM (primary, one-sided Welch t=-16.20, df=29.7): **p=1.4e-16**; mean diff -12562 tokens [bootstrap 95% CI -13991, -11051].

Session wall DynCLM vs baseline (secondary, two-sided Welch t=-0.67, df=30.0): **p=0.508**; mean diff -12.1s [bootstrap 95% CI -46.1, 21.8].

## 2. Live graded quality

| Arm | Accuracy [Wilson 95% CI] | Correct/N | Avg wall-s | Avg ctx |
|---|---:|---:|---:|---:|
| Baseline | 94.4% [74.2, 99.0] | 17/18 | 1.15 | 162 |
| DynCLM | 100.0% [82.4, 100.0] | 18/18 | 1.40 | 162 |
| CLM | 100.0% [82.4, 100.0] | 18/18 | 1.41 | 162 |

_Parity check only: ~162-token contexts, no multi-turn reclaim; single-task margins are not statistically meaningful._

## 3. Live long-horizon A (primary quality + late speed)

Sessions/arm: 8 · Turns: 24 · live=True

| Arm | Late probe % | Late avg ctx | Late steady wall-s | Late tok/s | Prefill/sess | Rewrites/sess |
|---|---:|---:|---:|---:|---:|---:|
| Baseline | 0.0 | 6857 | 2.49 | 2.45 | 0 | 0.0 |
| DynCLM | 100.0 | 2000 | 1.71 | 2.96 | 144 | 1.0 |
| CLM | 100.0 | 375 | 1.50 | 3.12 | 2774 | 24.0 |

**Session-level strict pass** (all late-window probes correct; pre-registered unit of analysis; Wilson 95% CI):

- Baseline: 0.0% [0.0, 32.4] (n=8)
- DynCLM: 100.0% [67.6, 100.0] (n=8)
- CLM: 100.0% [67.6, 100.0] (n=8)
- Fisher one-sided DynCLM vs Baseline: **p=7.8e-05**
- Fisher one-sided CLM vs Baseline: **p=7.8e-05**

_Probe-level percentages above pool correlated probes within sessions; the session-level counts are the confirmatory numbers._

## 4. Live long-horizon B (compaction matrix)

Limits: [16384, 65536] · live=True

| Max ctx | Compact probe% | DynCLM | CLM | Compact ctx | DynCLM ctx | CLM ctx | Compact wall | DynCLM wall | CLM wall |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 16384 | 0.0 | 100.0 | 100.0 | 14502 | 5154 | 320 | 2.11 | 1.52 | 1.34 |
| 65536 | 0.0 | 100.0 | 100.0 | 58201 | 19348 | 320 | 2.96 | 1.49 | 1.32 |

Session-level strict pass at max_context=16384 (Wilson 95% CI):

- Baseline+compact: 0.0% [0.0, 27.8] (n=10)
- DynCLM: 100.0% [72.2, 100.0] (n=10)
- CLM: 100.0% [72.2, 100.0] (n=10)
- Fisher one-sided DynCLM vs Baseline+compact: **p=5.4e-06**
- Fisher one-sided CLM vs Baseline+compact: **p=5.4e-06**


Session-level strict pass at max_context=65536 (Wilson 95% CI):

- Baseline+compact: 0.0% [0.0, 27.8] (n=10)
- DynCLM: 100.0% [72.2, 100.0] (n=10)
- CLM: 100.0% [72.2, 100.0] (n=10)
- Fisher one-sided DynCLM vs Baseline+compact: **p=5.4e-06**
- Fisher one-sided CLM vs Baseline+compact: **p=5.4e-06**


## 5. Verdict

| Dimension | Vs baseline | DynCLM vs CLM |
|---|---|---|
| Graded accuracy | DynCLM/CLM 5.6 pp | tie (100.0% vs 100.0%) |
| Late probe quality | DynCLM/CLM 100.0 pp | tie |
| Late steady wall | DynCLM -31.3%; CLM -39.6% | CLM faster |
| Session wall (gate) | DynCLM -3.3%; CLM -8.6% | similar |
| Rewrite prefill (gate) | both worse than 0 | DynCLM -71.5% vs CLM |

## Figures

See `docs/figures/`.

## Note on learning-900

The policy-learning grid was paused/archived under the prior run (`archive_learning_900_*`). Resume with `bash scripts/resume_live_full.sh results/runs/20261009T034520Z` if needed for β/γ fitting - not required for this verdict.
