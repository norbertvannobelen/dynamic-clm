# DynCLM Decision Report - Baseline vs DynCLM vs CLM

**Run:** `20261010T145959Z_decision` · live=True · status=complete

Efficient decision suite (not the 900-session policy grid): short gate + graded quality + long-horizon A + compaction matrix (16k/64k).

## 1. Short live gate - session speed & cost

Sessions/arm: 8 · Turns: 40 · live_model=True · passed=True

| Metric | Baseline | DynCLM | CLM |
|---|---:|---:|---:|
| Success % | 100.0 | 100.0 | 100.0 |
| Avg context | 11714 | 5410 | 3995 |
| Peak context | 23009 | 12630 | 8556 |
| Wall-clock / session (s) | 402.3 | 365.9 | 352.5 |
| Rewrite prefill / session | 0 | 5340 | 18504 |
| Rewrites / session | 0.0 | 5.0 | 17.0 |
| Prompt tokens / session | 79161 | 96850 | 91271 |
| Generated tokens / session | 4755 | 4504 | 4386 |

### Vs baseline

| Metric | DynCLM | CLM |
|---|---|---|
| Avg context | -53.8% (11714 → 5410) | -65.9% (11714 → 3995) |
| Peak context | -45.1% (23009 → 12630) | -62.8% (23009 → 8556) |
| Wall-clock / session (s) | -9.0% (402 → 366) | -12.4% (402 → 352) |
| Rewrite prefill / session | - (0 → 5340) | - (0 → 18504) |
| Rewrites / session | - (0 → 5) | - (0 → 17) |
| Prompt tokens / session | +22.3% (79161 → 96850) | +15.3% (79161 → 91271) |
| Generated tokens / session | -5.3% (4755 → 4504) | -7.8% (4755 → 4386) |

## 2. Live graded quality

| Arm | Accuracy | Correct/N | Avg wall-s | Avg ctx |
|---|---:|---:|---:|---:|
| Baseline | 94.4% | 17/18 | 1.55 | 162 |
| DynCLM | 100.0% | 18/18 | 1.77 | 162 |
| CLM | 100.0% | 18/18 | 1.83 | 162 |

## 3. Live long-horizon A (primary quality + late speed)

Sessions/arm: 3 · Turns: 24 · live=True

| Arm | Late probe % | Late avg ctx | Late steady wall-s | Late tok/s | Prefill/sess | Rewrites/sess |
|---|---:|---:|---:|---:|---:|---:|
| Baseline | 0.0 | 6857 | 2.60 | 2.19 | 0 | 0.0 |
| DynCLM | 100.0 | 2000 | 3.78 | 1.74 | 144 | 1.0 |
| CLM | 100.0 | 375 | 1.99 | 2.36 | 2774 | 24.0 |

## 4. Live long-horizon B (compaction matrix)

Limits: [16384, 65536] · live=True · this decision-suite snapshot

| Max ctx | Compact probe% | DynCLM | CLM | Compact ctx | DynCLM ctx | CLM ctx | Compact wall | DynCLM wall | CLM wall |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 16384 | 0.0 | 100.0 | 100.0 | 14154 | 5154 | 320 | 6.23 | 4.18 | 1.75 |
| 65536 | 0.0 | 100.0 | 100.0 | 57914 | 19348 | 320 | 3.56 | 6.50 | 1.71 |

**Current methodology:** Baseline+compact is Codex-style rolling summary. Fresh matrix with that baseline: [longhorizon-compaction-report.md](longhorizon-compaction-report.md) (`20261013T201630Z_lh_compaction_summary`). Late probes remain **0% → 100%** (recency trap); cite that report for wall/ctx under summarize compact.

## 5. Verdict

| Dimension | Vs baseline | DynCLM vs CLM |
|---|---|---|
| Graded accuracy | DynCLM/CLM 5.6 pp | tie (100.0% vs 100.0%) |
| Late probe quality | DynCLM/CLM 100.0 pp | tie |
| Late steady wall | DynCLM +45.6%; CLM -23.2% | CLM faster |
| Session wall (gate) | DynCLM -9.0%; CLM -12.4% | similar |
| Rewrite prefill (gate) | both worse than 0 | DynCLM -71.1% vs CLM |

## Figures

See `docs/figures/`.

## Note on learning-900

The policy-learning grid was paused/archived under the prior run (`archive_learning_900_*`). Resume with `bash scripts/resume_live_full.sh results/runs/20261009T034520Z` if needed for β/γ fitting - not required for this verdict.
