# Superseded data - DO NOT CITE

Everything under `results/superseded/` is **offline-era or smoke-run data that is invalid
for claims**. It is kept for provenance only. The citable live evidence is the 3-arm
decision suite plus SWE-Distill, documented in [docs/report.md](../../docs/report.md)
and archived under `results/runs/` (git-ignored) with published cells under
`results/longhorizon/`, `results/longhorizon_compaction*/`, and `results/quality/`.

## Contents

| Path | What it is | Why it is superseded |
|---|---|---|
| `benchmarks/bench1.json`, `bench2.json`, `bench4.json`, `bench5_pareto.json` | Offline micro-benchmarks (5 trials/cell) from 2026-10-08 | Offline/synthetic era; `docs/analysis-report.md` declares synthetic/offline gate data invalid for claims. `bench5_pareto.json` also has an unmeasured all-zero baseline arm. |
| `benchmarks/ship_decision.json` | Early ship-decision summary (2026-10-08) | Stale: cites "gate re-prefill 38.6% below immediate rewrite"; the final live number is **-71.1%** (run `20261010T145959Z_decision`). |
| `gate/bench3.json` | Offline Bench3 marker-compliance curve | Offline era; shows idealized compliance (1.0/0.0) not reproduced live. |
| `live-smoke/` | 2-session live smoke validation (2026-10-14), incl. its Bench3 curve | Smoke test only (n=2/arm, PASSED gate check); never intended as evidence. |

## Notes

- `cmd/dynclm-eval` (legacy `make learning` / `make report` path) regenerates
  `results/benchmarks/*.json` when run. Those regenerated files are still offline
  micro-benchmarks and remain non-citable.
- The live gate copies consumed by `scripts/gen_charts.py` stay in place:
  `results/gate/report.json`, `results/quality/report.json`,
  `results/longhorizon/report.json`, `results/longhorizon_compaction_live/matrix_report.json`.
