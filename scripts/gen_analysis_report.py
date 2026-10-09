#!/usr/bin/env python3
"""Aggregate preserved live runs into docs/analysis-report.md."""

from __future__ import annotations

import argparse
import json
from pathlib import Path
from statistics import mean


def load_json(p: Path):
    if not p.exists():
        return None
    return json.loads(p.read_text())


def gate_rows(runs: list[Path]) -> list[dict]:
    rows = []
    for r in runs:
        rep = load_json(r / "gate" / "report.json")
        man = load_json(r / "manifest.json") or {}
        if not rep:
            continue
        if man.get("live") is False or rep.get("live_model") is False:
            continue
        stats = rep.get("arm_stats", {})
        rows.append(
            {
                "run_id": r.name,
                "live": bool(rep.get("live_model")),
                "passed": bool(rep.get("passed")),
                "baseline_ctx": stats.get("baseline", {}).get("avg_context_tokens"),
                "dyn_ctx": stats.get("dynclm", {}).get("avg_context_tokens"),
                "imm_ctx": stats.get("immediate-rewrite", {}).get("avg_context_tokens"),
                "dyn_prefill": stats.get("dynclm", {}).get("avg_prefill_tokens"),
                "imm_prefill": stats.get("immediate-rewrite", {}).get("avg_prefill_tokens"),
                "dyn_wall": stats.get("dynclm", {}).get("avg_wall_clock_s"),
                "base_wall": stats.get("baseline", {}).get("avg_wall_clock_s"),
            }
        )
    return rows


def lh_rows(runs: list[Path], sub: str) -> list[dict]:
    rows = []
    for r in runs:
        rep = load_json(r / sub / "report.json")
        if not rep or not rep.get("live"):
            continue
        arms = rep.get("arms", {})
        rows.append(
            {
                "run_id": r.name,
                "baseline_probe": (arms.get("baseline") or arms.get("baseline-compact") or {})
                .get("late", {})
                .get("probe_accuracy_pct"),
                "dyn_probe": arms.get("dynclm", {}).get("late", {}).get("probe_accuracy_pct"),
                "imm_probe": arms.get("immediate-rewrite", {}).get("late", {}).get("probe_accuracy_pct"),
                "baseline_ctx": (arms.get("baseline") or arms.get("baseline-compact") or {})
                .get("late", {})
                .get("avg_context_tokens"),
                "dyn_ctx": arms.get("dynclm", {}).get("late", {}).get("avg_context_tokens"),
                "dyn_wall": arms.get("dynclm", {}).get("late_steady", {}).get("avg_wall_clock_s"),
                "base_wall": (arms.get("baseline") or arms.get("baseline-compact") or {})
                .get("late_steady", {})
                .get("avg_wall_clock_s"),
                "imm_wall": arms.get("immediate-rewrite", {}).get("late_steady", {}).get("avg_wall_clock_s"),
            }
        )
    return rows


def avg(vals):
    vals = [v for v in vals if v is not None]
    return mean(vals) if vals else None


def fmt(v, nd=1):
    if v is None:
        return "—"
    return f"{v:.{nd}f}"


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--runs-root", default="results/runs")
    ap.add_argument("--out", default="docs/analysis-report.md")
    args = ap.parse_args()
    root = Path(args.runs_root)
    runs = sorted([p for p in root.iterdir() if p.is_dir()]) if root.exists() else []

    gates = gate_rows(runs)
    lh_a = lh_rows(runs, "longhorizon")
    # compaction matrix: prefer matrix_report
    lh_b = []
    for r in runs:
        m = load_json(r / "longhorizon_compaction" / "matrix_report.json")
        if not m or not m.get("live"):
            continue
        lh_b.append({"run_id": r.name, "matrix": m.get("by_max_context", {}), "limits": m.get("max_contexts", [])})

    quality = []
    for r in runs:
        rep = load_json(r / "quality" / "report.json")
        if not rep:
            continue
        # quality report may not flag live; include if arms present
        arms = rep.get("arms", {})
        if not arms:
            continue
        quality.append(
            {
                "run_id": r.name,
                "base_acc": arms.get("baseline", {}).get("accuracy_pct"),
                "dyn_acc": arms.get("dynclm", {}).get("accuracy_pct"),
                "imm_acc": arms.get("immediate-rewrite", {}).get("accuracy_pct"),
            }
        )

    lines = []
    lines.append("# DynCLM Live Analysis Report\n")
    lines.append(f"Aggregated over **{len(runs)}** preserved run(s) under `{args.runs_root}`.\n")
    lines.append("Only runs with live gate/long-horizon flags are averaged for those suites.\n")
    lines.append("\n## Runs\n\n| Run ID | Status | Live | Suites |\n|---|---|---|---|\n")
    for r in runs:
        man = load_json(r / "manifest.json") or {}
        lines.append(
            f"| `{r.name}` | {man.get('status','?')} | {man.get('live','?')} | {', '.join(man.get('suites') or [])} |\n"
        )

    lines.append("\n## Gate (live averages)\n\n")
    if not gates:
        lines.append("_No live gate runs archived yet._\n")
    else:
        lines.append(f"n={len(gates)} live gate run(s).\n\n")
        lines.append("| Metric | Mean |\n|---|---|\n")
        lines.append(f"| Baseline avg context | {fmt(avg([g['baseline_ctx'] for g in gates]), 0)} |\n")
        lines.append(f"| DynCLM avg context | {fmt(avg([g['dyn_ctx'] for g in gates]), 0)} |\n")
        lines.append(f"| CLM avg context | {fmt(avg([g['imm_ctx'] for g in gates]), 0)} |\n")
        lines.append(f"| DynCLM avg prefill | {fmt(avg([g['dyn_prefill'] for g in gates]), 0)} |\n")
        lines.append(f"| CLM avg prefill | {fmt(avg([g['imm_prefill'] for g in gates]), 0)} |\n")
        lines.append(f"| Baseline avg wall-s | {fmt(avg([g['base_wall'] for g in gates]), 2)} |\n")
        lines.append(f"| DynCLM avg wall-s | {fmt(avg([g['dyn_wall'] for g in gates]), 2)} |\n")
        lines.append("\nPer-run:\n\n| Run | pass | base ctx | dyn ctx | dyn wall-s |\n|---|---|---|---|---|\n")
        for g in gates:
            lines.append(
                f"| `{g['run_id']}` | {g['passed']} | {fmt(g['baseline_ctx'],0)} | {fmt(g['dyn_ctx'],0)} | {fmt(g['dyn_wall'],2)} |\n"
            )

    lines.append("\n## Quality (averages)\n\n")
    if not quality:
        lines.append("_No quality runs in archives yet._\n")
    else:
        lines.append(f"n={len(quality)}.\n\n| Arm | Mean accuracy % |\n|---|---|\n")
        lines.append(f"| Baseline | {fmt(avg([q['base_acc'] for q in quality]))} |\n")
        lines.append(f"| DynCLM | {fmt(avg([q['dyn_acc'] for q in quality]))} |\n")
        lines.append(f"| CLM | {fmt(avg([q['imm_acc'] for q in quality]))} |\n")

    lines.append("\n## Long-horizon A (live averages)\n\n")
    if not lh_a:
        lines.append("_No live long-horizon A runs archived yet._\n")
    else:
        lines.append(f"n={len(lh_a)}.\n\n| Metric | Mean |\n|---|---|\n")
        lines.append(f"| Baseline late probe % | {fmt(avg([x['baseline_probe'] for x in lh_a]))} |\n")
        lines.append(f"| DynCLM late probe % | {fmt(avg([x['dyn_probe'] for x in lh_a]))} |\n")
        lines.append(f"| CLM late probe % | {fmt(avg([x['imm_probe'] for x in lh_a]))} |\n")
        lines.append(f"| Baseline late ctx | {fmt(avg([x['baseline_ctx'] for x in lh_a]), 0)} |\n")
        lines.append(f"| DynCLM late ctx | {fmt(avg([x['dyn_ctx'] for x in lh_a]), 0)} |\n")
        lines.append(f"| DynCLM late steady wall-s | {fmt(avg([x['dyn_wall'] for x in lh_a]), 2)} |\n")
        lines.append(f"| CLM late steady wall-s | {fmt(avg([x['imm_wall'] for x in lh_a]), 2)} |\n")

    lines.append("\n## Long-horizon B (compaction matrix)\n\n")
    if not lh_b:
        lines.append("_No live compaction-matrix runs archived yet._\n")
    else:
        lines.append(f"n={len(lh_b)} matrix run(s). Latest `{lh_b[-1]['run_id']}`:\n\n")
        m = lh_b[-1]["matrix"]
        lines.append("| Max ctx | compact probe | DynCLM | CLM | compact ctx | DynCLM ctx |\n|---|---|---|---|---|---|\n")
        for lim in lh_b[-1]["limits"]:
            arms = m.get(str(lim), {})
            bc = arms.get("baseline-compact", {})
            d = arms.get("dynclm", {})
            imm = arms.get("immediate-rewrite", {})
            lines.append(
                f"| {lim} | {fmt(bc.get('late_probe_accuracy'))} | {fmt(d.get('late_probe_accuracy'))} | "
                f"{fmt(imm.get('late_probe_accuracy'))} | {fmt(bc.get('late_avg_context'),0)} | {fmt(d.get('late_avg_context'),0)} |\n"
            )

    lines.append("\n## Figures\n\nSee `docs/figures/` (regenerate with `make charts`).\n")
    lines.append("\n## Method note\n\n")
    lines.append(
        "Synthetic/offline historical artifacts under `results/gate` from before live enforcement "
        "are **not** mixed into averages unless a run archive marks `live: true`.\n"
    )

    out = Path(args.out)
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text("".join(lines), encoding="utf-8")
    print("wrote", out)


if __name__ == "__main__":
    main()
