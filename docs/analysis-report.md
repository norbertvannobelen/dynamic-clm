# DynCLM Live Analysis Report

**Status: live full pipeline in progress.**

Synthetic/offline gate and Phase B results are **invalid for claims** and are excluded from averages. A new archived live run is executing under `results/latest` → `results/runs/<id>/`.

## What changed

1. **Live required by default**: `--offline` disabled; synthetic only with `--allow-synthetic`.
2. **Learning bug fixed**: previously only the first model was live; all models/policies/sessions now call oMLX.
3. **Per-run archives**: each pipeline execution lands in `results/runs/<timestamp>/` with `manifest.json`; `results/latest` points at the active/most recent run.
4. **Analysis**: `scripts/gen_analysis_report.py` averages across archived **live** runs into this file when the pipeline finishes.

## Pipeline (`make live-full`)

| Step | Suite | Notes |
|---|---|---|
| 1 | Gate G1–G4 | 20×50×3 arms, **live** |
| 2 | Phase B learning | 900 sessions, **all 3 models live** |
| 3 | Quality | 18 tasks live |
| 4 | Long-horizon A | live |
| 5 | Long-horizon B | compaction matrix live |
| 6 | Charts + this report | regenerated |

Expected wall time: many hours (especially step 2).

## Already preserved live data

| Run | Contents |
|---|---|
| `results/runs/20261009T011146Z_prior_live` | Prior live quality + long-horizon A/B (gate/learning were synthetic → excluded) |

Monitor: `tail -f results/live-full.log` and `results/latest/progress.log`.

When complete, re-run: `make analysis` (also invoked at end of `scripts/run_live_full.sh`).
