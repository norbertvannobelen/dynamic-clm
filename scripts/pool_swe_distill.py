#!/usr/bin/env python3
"""Pool SWE-Distill cells from multiple runs into one report.json.

Implements the pre-registered extension pooling rule (docs/analysis-plan.md):
cells from the base run and the extension run are merged (same methodology,
same calibration), arm aggregates and vs_baseline deltas are recomputed over
the pooled set, and the result is a drop-in report.json for
gen_swe_distill_report.py. Aggregate formulas mirror
internal/eval/swedistill/suite.go.
"""

from __future__ import annotations

import argparse
import json
import sys
from datetime import datetime, timezone
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from stats_utils import fisher_exact_greater, fmt_prop_ci  # noqa: E402

ARM_ORDER = ["baseline-compact", "dynclm", "immediate-rewrite"]


def cell_final_live(c: dict) -> int:
    return int(c.get("final_context") or 0)


def cell_peak_live(c: dict) -> int:
    pl = int(c.get("peak_live_context") or 0)
    return pl if pl > 0 else int(c.get("peak_context") or 0)


def cell_steady_wall(c: dict) -> float:
    # Mirror cellSteadyWall in internal/eval/swedistill/suite.go: prefer the
    # stored value; else sum non-reclaim turn wall; CLM rewrites every turn,
    # so an empty steady set falls back to total wall.
    stored = float(c.get("steady_wall_s") or 0)
    if stored > 0:
        return stored
    s = sum(float(t.get("wall_clock_s") or 0) for t in (c.get("turns") or [])
            if not t.get("rewrite_fired") and not t.get("compact_fired"))
    return s if s > 0 else float(c.get("total_wall_s") or 0)


def aggregate(cells: list[dict]) -> dict:
    n = len(cells)
    correct = sum(1 for c in cells if c.get("success"))
    surv = sum(1 for c in cells if c.get("spec_survived"))
    total_wall = sum(float(c.get("total_wall_s") or 0) for c in cells)
    return {
        "arm": cells[0]["arm"] if cells else "?",
        "n": n,
        "correct": correct,
        "accuracy_pct": 100.0 * correct / n if n else 0.0,
        "avg_final_context": sum(cell_final_live(c) for c in cells) / n,
        "avg_final_total": sum(float(c.get("final_total_context") or 0) for c in cells) / n,
        "avg_final_prompt": sum(float(c.get("final_prompt_tokens") or 0) for c in cells) / n,
        "peak_context": max(cell_peak_live(c) for c in cells),
        "peak_total_context": max(int(c.get("peak_context") or 0) for c in cells),
        "avg_compacts": sum(float(c.get("compacts") or 0) for c in cells) / n,
        "avg_rewrites": sum(float(c.get("rewrites") or 0) for c in cells) / n,
        "avg_prefill": sum(float(c.get("total_prefill") or 0) for c in cells) / n,
        "total_wall_s": total_wall,
        "avg_wall_s": total_wall / n,
        "avg_steady_wall_s": sum(cell_steady_wall(c) for c in cells) / n,
        "spec_survive_pct": 100.0 * surv / n if n else 0.0,
    }


def pct(a: float, b: float) -> float:
    return (100.0 * (a - b) / b) if b else 0.0


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--reports", nargs="+", required=True,
                    help="base report.json first, then extension report(s)")
    ap.add_argument("--out", required=True)
    args = ap.parse_args()

    reports = [json.loads(Path(p).read_text()) for p in args.reports]
    cells: list[dict] = []
    for r in reports:
        cells.extend(r.get("cells") or [])

    arms = {}
    for name in ARM_ORDER:
        ac = [c for c in cells if c.get("arm") == name]
        if ac:
            arms[name] = aggregate(ac)

    # tasks in first-appearance order
    seen: dict[str, dict] = {}
    for c in cells:
        iid = c.get("instance_id") or "?"
        tb = seen.setdefault(iid, {
            "instance_id": iid,
            "expected": c.get("expected") or "",
            "by_arm": {},
            "ctx_by_arm": {},
            "reclaim_events_by_arm": {},
        })
        arm = c.get("arm") or "?"
        tb["by_arm"][arm] = bool(c.get("success"))
        tb["ctx_by_arm"][arm] = cell_final_live(c)
        tb["reclaim_events_by_arm"][arm] = int(c.get("compacts") or 0) + int(c.get("rewrites") or 0)
    tasks = list(seen.values())

    base = arms.get("baseline-compact")
    vs = {}
    for name, a in arms.items():
        if base is None:
            break
        vs[name] = {
            "accuracy_delta_pp": a["accuracy_pct"] - base["accuracy_pct"],
            "ctx_pct": pct(a["avg_final_context"], base["avg_final_context"]),
            "total_ctx_pct": pct(a["avg_final_total"], base["avg_final_total"]),
            "prompt_pct": pct(a["avg_final_prompt"], base["avg_final_prompt"]),
            "wall_pct": pct(a["total_wall_s"], base["total_wall_s"]),
            "steady_wall_pct": pct(a["avg_steady_wall_s"], base["avg_steady_wall_s"]),
            "prefill_delta": a["avg_prefill"] - base["avg_prefill"],
        }

    meta = reports[0]
    pooled = {
        "generated_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%S.%fZ"),
        "model_id": meta.get("model_id"),
        "hardware_id": meta.get("hardware_id"),
        "live": all(r.get("live") for r in reports),
        "max_context": meta.get("max_context"),
        "max_turns": meta.get("max_turns"),
        "noise_pad": meta.get("noise_pad"),
        "methodology": (meta.get("methodology") or "")
        + " Pooled per pre-registered extension rule (docs/analysis-plan.md): "
        + " + ".join(args.reports) + ".",
        "arms": arms,
        "tasks": tasks,
        "cells": cells,
        "vs_baseline": vs,
    }
    out = Path(args.out)
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(json.dumps(pooled, indent=2) + "\n")

    # decisive pre-registered test on the pooled task-level counts
    d, b = arms["dynclm"], arms["baseline-compact"]
    p = fisher_exact_greater(d["correct"], d["n"] - d["correct"],
                             b["correct"], b["n"] - b["correct"])
    print(f"pooled n={d['n']} tasks/arm")
    for name in ARM_ORDER:
        a = arms[name]
        print(f"  {name:<18} {fmt_prop_ci(a['correct'], a['n'])}")
    print(f"Fisher one-sided dynclm>baseline-compact: p={p:.4f} "
          f"({'SIGNIFICANT' if p < 0.05 else 'not significant'} at alpha=0.05)")
    print(f"wrote {out}")


if __name__ == "__main__":
    main()
