#!/usr/bin/env python3
"""Generate SVG charts for DynCLM reports (stdlib only)."""

from __future__ import annotations

import json
import math
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from stats_utils import bootstrap_ci_mean, lh_pass_counts, wilson_ci  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]
FIG = ROOT / "docs" / "figures"
FIG.mkdir(parents=True, exist_ok=True)

# Canonical runs cited by docs/report.md (prefer these over mutable results/ copies).
DECISION_RUN = ROOT / "results" / "runs" / "20261015T165854Z_decision"
LH_B_SUMMARY_RUN = ROOT / "results" / "runs" / "20261015T165854Z_decision"

COLORS = {
    "baseline": "#6b7280",
    "baseline-compact": "#9ca3af",
    "dynclm": "#2563eb",
    "immediate-rewrite": "#dc2626",
    "clm": "#dc2626",
}
LABELS = {
    "baseline": "Baseline (raw)",
    "baseline-compact": "Baseline + compaction",
    "dynclm": "DynCLM",
    "immediate-rewrite": "CLM (immediate rewrite)",
    "clm": "CLM (immediate rewrite)",
}


def first_existing(*paths: Path) -> Path | None:
    for p in paths:
        if p is not None and p.exists():
            return p
    return None


def esc(s: str) -> str:
    return (
        s.replace("&", "&amp;")
        .replace("<", "&lt;")
        .replace(">", "&gt;")
        .replace('"', "&quot;")
    )


def grouped_bars(path: Path, title: str, series: dict[str, float], ylabel: str,
                 errors: dict[str, tuple[float, float]] | None = None) -> None:
    """series: arm -> value; errors: arm -> (lo, hi) absolute error-bar bounds"""
    arms = [a for a in ["baseline", "baseline-compact", "dynclm", "immediate-rewrite"] if a in series]
    if not arms:
        arms = list(series.keys())
    w, h = 720, 420
    pad_l, pad_r, pad_t, pad_b = 70, 30, 50, 90
    plot_w = w - pad_l - pad_r
    plot_h = h - pad_t - pad_b
    vals = [series[a] for a in arms]
    vmax = max(vals) if vals else 1
    if errors:
        vmax = max([vmax] + [hi for _, hi in errors.values()])
    if vmax <= 0:
        vmax = 1
    n = len(arms)
    bar_w = plot_w / max(n, 1) * 0.55
    gap = plot_w / max(n, 1)

    parts = [
        f'<svg xmlns="http://www.w3.org/2000/svg" width="{w}" height="{h}" viewBox="0 0 {w} {h}">',
        f'<rect width="100%" height="100%" fill="#fafafa"/>',
        f'<text x="{w/2}" y="28" text-anchor="middle" font-family="Helvetica,Arial,sans-serif" font-size="16" font-weight="700">{esc(title)}</text>',
        f'<text x="18" y="{pad_t + plot_h/2}" transform="rotate(-90 18,{pad_t + plot_h/2})" text-anchor="middle" font-family="Helvetica,Arial,sans-serif" font-size="12" fill="#374151">{esc(ylabel)}</text>',
        f'<line x1="{pad_l}" y1="{pad_t}" x2="{pad_l}" y2="{pad_t+plot_h}" stroke="#111827" stroke-width="1.5"/>',
        f'<line x1="{pad_l}" y1="{pad_t+plot_h}" x2="{pad_l+plot_w}" y2="{pad_t+plot_h}" stroke="#111827" stroke-width="1.5"/>',
    ]
    # grid
    for i in range(5):
        y = pad_t + plot_h * i / 4
        val = vmax * (1 - i / 4)
        parts.append(
            f'<line x1="{pad_l}" y1="{y}" x2="{pad_l+plot_w}" y2="{y}" stroke="#e5e7eb"/>'
        )
        parts.append(
            f'<text x="{pad_l-8}" y="{y+4}" text-anchor="end" font-family="Helvetica,Arial,sans-serif" font-size="11" fill="#6b7280">{val:.2g}</text>'
        )

    for i, arm in enumerate(arms):
        v = series[arm]
        bh = (v / vmax) * plot_h
        x = pad_l + gap * i + (gap - bar_w) / 2
        y = pad_t + plot_h - bh
        color = COLORS.get(arm, "#111827")
        parts.append(
            f'<rect x="{x}" y="{y}" width="{bar_w}" height="{bh}" fill="{color}" rx="3"/>'
        )
        if errors and arm in errors:
            lo, hi = errors[arm]
            y_lo = pad_t + plot_h - (min(lo, vmax) / vmax) * plot_h
            y_hi = pad_t + plot_h - (min(hi, vmax) / vmax) * plot_h
            xc = x + bar_w / 2
            parts.append(
                f'<line x1="{xc}" y1="{y_lo}" x2="{xc}" y2="{y_hi}" stroke="#111827" stroke-width="1.5"/>'
                f'<line x1="{xc-6}" y1="{y_lo}" x2="{xc+6}" y2="{y_lo}" stroke="#111827" stroke-width="1.5"/>'
                f'<line x1="{xc-6}" y1="{y_hi}" x2="{xc+6}" y2="{y_hi}" stroke="#111827" stroke-width="1.5"/>'
            )
        parts.append(
            f'<text x="{x + bar_w/2}" y="{y-6}" text-anchor="middle" font-family="Helvetica,Arial,sans-serif" font-size="11" fill="#111827">{v:.2g}</text>'
        )
        label = LABELS.get(arm, arm)
        parts.append(
            f'<text x="{x + bar_w/2}" y="{pad_t+plot_h+20}" text-anchor="middle" font-family="Helvetica,Arial,sans-serif" font-size="11" fill="#111827">{esc(label)}</text>'
        )

    parts.append("</svg>")
    path.write_text("\n".join(parts), encoding="utf-8")


def multi_line(path: Path, title: str, curves: dict[str, list[tuple[float, float]]], xlabel: str, ylabel: str,
               errors: dict[str, list[tuple[float, float, float]]] | None = None) -> None:
    """curves: arm -> list of (x,y); errors: arm -> list of (x, lo, hi)"""
    w, h = 780, 440
    pad_l, pad_r, pad_t, pad_b = 70, 40, 50, 70
    plot_w = w - pad_l - pad_r
    plot_h = h - pad_t - pad_b
    all_x, all_y = [], []
    for pts in curves.values():
        for x, y in pts:
            all_x.append(x)
            all_y.append(y)
    if errors:
        for pts in errors.values():
            for x, lo, hi in pts:
                all_x.append(x)
                all_y.extend([lo, hi])
    if not all_x:
        return
    xmin, xmax = min(all_x), max(all_x)
    ymin, ymax = min(0, min(all_y)), max(all_y)
    if xmax == xmin:
        xmax = xmin + 1
    if ymax == ymin:
        ymax = ymin + 1

    def sx(x: float) -> float:
        return pad_l + (x - xmin) / (xmax - xmin) * plot_w

    def sy(y: float) -> float:
        return pad_t + plot_h - (y - ymin) / (ymax - ymin) * plot_h

    parts = [
        f'<svg xmlns="http://www.w3.org/2000/svg" width="{w}" height="{h}" viewBox="0 0 {w} {h}">',
        f'<rect width="100%" height="100%" fill="#fafafa"/>',
        f'<text x="{w/2}" y="28" text-anchor="middle" font-family="Helvetica,Arial,sans-serif" font-size="16" font-weight="700">{esc(title)}</text>',
        f'<text x="18" y="{pad_t + plot_h/2}" transform="rotate(-90 18,{pad_t + plot_h/2})" text-anchor="middle" font-family="Helvetica,Arial,sans-serif" font-size="12" fill="#374151">{esc(ylabel)}</text>',
        f'<text x="{pad_l + plot_w/2}" y="{h-18}" text-anchor="middle" font-family="Helvetica,Arial,sans-serif" font-size="12" fill="#374151">{esc(xlabel)}</text>',
        f'<line x1="{pad_l}" y1="{pad_t}" x2="{pad_l}" y2="{pad_t+plot_h}" stroke="#111827" stroke-width="1.5"/>',
        f'<line x1="{pad_l}" y1="{pad_t+plot_h}" x2="{pad_l+plot_w}" y2="{pad_t+plot_h}" stroke="#111827" stroke-width="1.5"/>',
    ]
    for i in range(5):
        y = pad_t + plot_h * i / 4
        val = ymax - (ymax - ymin) * i / 4
        parts.append(f'<line x1="{pad_l}" y1="{y}" x2="{pad_l+plot_w}" y2="{y}" stroke="#e5e7eb"/>')
        parts.append(
            f'<text x="{pad_l-8}" y="{y+4}" text-anchor="end" font-family="Helvetica,Arial,sans-serif" font-size="11" fill="#6b7280">{val:.2g}</text>'
        )

    legend_x = pad_l + 10
    legend_y = pad_t + 14
    for j, (arm, pts) in enumerate(curves.items()):
        if not pts:
            continue
        color = COLORS.get(arm, "#111827")
        d = "M " + " L ".join(f"{sx(x):.1f},{sy(y):.1f}" for x, y in pts)
        parts.append(f'<path d="{d}" fill="none" stroke="{color}" stroke-width="2.5"/>')
        for x, y in pts:
            parts.append(f'<circle cx="{sx(x):.1f}" cy="{sy(y):.1f}" r="3" fill="{color}"/>')
        if errors and arm in errors:
            for x, lo, hi in errors[arm]:
                parts.append(
                    f'<line x1="{sx(x):.1f}" y1="{sy(lo):.1f}" x2="{sx(x):.1f}" y2="{sy(hi):.1f}" stroke="{color}" stroke-width="1.5"/>'
                    f'<line x1="{sx(x)-5:.1f}" y1="{sy(lo):.1f}" x2="{sx(x)+5:.1f}" y2="{sy(lo):.1f}" stroke="{color}" stroke-width="1.5"/>'
                    f'<line x1="{sx(x)-5:.1f}" y1="{sy(hi):.1f}" x2="{sx(x)+5:.1f}" y2="{sy(hi):.1f}" stroke="{color}" stroke-width="1.5"/>'
                )
        parts.append(
            f'<rect x="{legend_x}" y="{legend_y + j*18 - 8}" width="12" height="12" fill="{color}"/>'
        )
        parts.append(
            f'<text x="{legend_x + 18}" y="{legend_y + j*18 + 2}" font-family="Helvetica,Arial,sans-serif" font-size="12" fill="#111827">{esc(LABELS.get(arm, arm))}</text>'
        )

    parts.append("</svg>")
    path.write_text("\n".join(parts), encoding="utf-8")


def from_longhorizon() -> None:
    p = first_existing(
        DECISION_RUN / "longhorizon" / "report.json",
        ROOT / "results" / "longhorizon" / "report.json",
    )
    if p is None:
        return
    print("longhorizon charts from", p.relative_to(ROOT))
    rep = json.loads(p.read_text())
    arms = rep.get("arms", {})
    cells = first_existing(p.parent / "cells", ROOT / "results" / "longhorizon" / "cells")
    probe_errors = None
    if cells is not None:
        counts = lh_pass_counts(cells, int(rep.get("late_from_turn") or 0))
        if counts:
            probe_errors = {
                a: (100 * wilson_ci(k, n)[0], 100 * wilson_ci(k, n)[1])
                for a, (k, n) in counts.items()
            }
    # bar charts late metrics
    grouped_bars(
        FIG / "lh_late_probe_accuracy.svg",
        "Long-horizon A: late probe accuracy (session-level Wilson 95% CI)",
        {a: arms[a]["late"]["probe_accuracy_pct"] for a in arms},
        "Accuracy (%)",
        errors=probe_errors,
    )
    grouped_bars(
        FIG / "lh_late_context.svg",
        "Long-horizon A: late avg context length",
        {a: arms[a]["late"]["avg_context_tokens"] for a in arms},
        "Tokens",
    )
    grouped_bars(
        FIG / "lh_late_steady_wall.svg",
        "Long-horizon A: late steady wall-clock / turn",
        {
            a: arms[a].get("late_steady", arms[a]["late"]).get("avg_wall_clock_s", arms[a]["late"]["avg_wall_clock_s"])
            for a in arms
        },
        "Seconds",
    )
    grouped_bars(
        FIG / "lh_late_tok_s.svg",
        "Long-horizon A: late steady generation tok/s",
        {
            a: arms[a].get("late_steady", arms[a]["late"]).get("avg_gen_tokens_per_sec", arms[a]["late"]["avg_gen_tokens_per_sec"])
            for a in arms
        },
        "Tokens / second",
    )

    # turn curves averaged across sessions (same run as report.json)
    if cells is None:
        return
    print("longhorizon turn curves from", cells.relative_to(ROOT))
    curves_ctx: dict[str, list[tuple[float, float]]] = {}
    curves_wall: dict[str, list[tuple[float, float]]] = {}
    for arm_dir in sorted(cells.glob("*")):
        if not arm_dir.is_dir():
            continue
        arm = arm_dir.name
        acc: dict[int, list[float]] = {}
        wall: dict[int, list[float]] = {}
        for f in arm_dir.glob("*.json"):
            s = json.loads(f.read_text())
            for t in s.get("turns", []):
                turn = t["turn"]
                acc.setdefault(turn, []).append(t["context_tokens"])
                wall.setdefault(turn, []).append(t["wall_clock_s"])
        curves_ctx[arm] = [(k, sum(v) / len(v)) for k, v in sorted(acc.items())]
        curves_wall[arm] = [(k, sum(v) / len(v)) for k, v in sorted(wall.items())]
    multi_line(FIG / "lh_context_vs_turn.svg", "Long-horizon A: context length vs turn", curves_ctx, "Turn", "Context tokens")
    multi_line(FIG / "lh_wall_vs_turn.svg", "Long-horizon A: wall-clock vs turn", curves_wall, "Turn", "Seconds")


def from_gate() -> None:
    p = first_existing(
        DECISION_RUN / "gate" / "report.json",
        ROOT / "results" / "gate" / "report.json",
    )
    if p is None:
        return
    print("gate charts from", p.relative_to(ROOT))
    rep = json.loads(p.read_text())
    stats = rep.get("arm_stats", {})

    # per-session bootstrap CIs from the run's gate cells (cells/<hw>/<model>/<task>/<arm>/*.json)
    samples: dict[str, dict[str, list[float]]] = {}
    cells_root = p.parent.parent / "cells"
    if cells_root.exists():
        for f in sorted(cells_root.glob("*/*/*/*/*.json")):
            cell = json.loads(f.read_text())
            bucket = samples.setdefault(f.parent.name, {})
            for m in ("avg_context_tokens", "total_prefill_tokens", "total_prompt_tokens"):
                v = cell.get(m)
                if isinstance(v, (int, float)):
                    bucket.setdefault(m, []).append(float(v))

    def boot_errors(metric: str) -> dict[str, tuple[float, float]] | None:
        errs = {a: bootstrap_ci_mean(s[metric]) for a, s in samples.items() if s.get(metric)}
        return errs or None

    ci_note = " (bootstrap 95% CI)" if samples else ""
    grouped_bars(
        FIG / "gate_context.svg",
        f"Gate (short multi-turn): avg context length{ci_note}",
        {a: stats[a]["avg_context_tokens"] for a in stats},
        "Tokens",
        errors=boot_errors("avg_context_tokens"),
    )
    grouped_bars(
        FIG / "gate_prefill.svg",
        f"Gate (short multi-turn): avg rewrite prefill tokens{ci_note}",
        {a: stats[a]["avg_prefill_tokens"] for a in stats},
        "Tokens",
        errors=boot_errors("total_prefill_tokens"),
    )
    grouped_bars(
        FIG / "gate_prompt.svg",
        f"Gate (short multi-turn): avg prompt tokens / session{ci_note}",
        {a: stats[a]["avg_prompt_tokens"] for a in stats},
        "Tokens",
        errors=boot_errors("total_prompt_tokens"),
    )


def from_quality() -> None:
    p = first_existing(
        DECISION_RUN / "quality" / "report.json",
        ROOT / "results" / "quality" / "report.json",
    )
    if p is None:
        return
    print("quality charts from", p.relative_to(ROOT))
    rep = json.loads(p.read_text())
    arms = rep.get("arms", {})
    q_errors = {
        a: (100 * wilson_ci(int(arms[a].get("correct") or 0), int(arms[a].get("n") or 0))[0],
            100 * wilson_ci(int(arms[a].get("correct") or 0), int(arms[a].get("n") or 0))[1])
        for a in arms
        if arms[a].get("n")
    } or None
    grouped_bars(
        FIG / "quality_accuracy.svg",
        "Short quality suite: graded accuracy (Wilson 95% CI)",
        {a: arms[a]["accuracy_pct"] for a in arms},
        "Accuracy (%)",
        errors=q_errors,
    )


def from_compaction() -> None:
    p = first_existing(
        LH_B_SUMMARY_RUN / "longhorizon_compaction" / "matrix_report.json",
        ROOT / "results" / "longhorizon_compaction_live" / "matrix_report.json",
        ROOT / "results" / "longhorizon_compaction" / "matrix_report.json",
    )
    if p is None:
        return
    rep = json.loads(p.read_text())
    print("compaction charts from", p.relative_to(ROOT))
    # late context by max_context for each arm
    by_limit = rep.get("by_max_context", {})
    for metric, ylabel, key in [
        ("probe", "Late probe accuracy (%)", "late_probe_accuracy"),
        ("context", "Late avg context tokens", "late_avg_context"),
        ("wall", "Late steady wall-clock (s)", "late_steady_wall_s"),
        ("toks", "Late steady gen tok/s", "late_steady_tok_s"),
    ]:
        curves: dict[str, list[tuple[float, float]]] = {}
        for limit_s, arms in sorted(by_limit.items(), key=lambda kv: int(kv[0])):
            limit = float(limit_s)
            for arm, m in arms.items():
                curves.setdefault(arm, []).append((limit, float(m.get(key, 0))))
        errs: dict[str, list[tuple[float, float, float]]] | None = None
        if metric == "probe":
            # session-level strict-pass Wilson CIs per limit (pre-registered unit)
            tmp: dict[str, list[tuple[float, float, float]]] = {}
            for limit_s, _arms in sorted(by_limit.items(), key=lambda kv: int(kv[0])):
                lim_dir = p.parent / f"c{limit_s}"
                lim_rep_path = lim_dir / "report.json"
                if not lim_rep_path.exists():
                    continue
                lim_rep = json.loads(lim_rep_path.read_text())
                counts = lh_pass_counts(lim_dir / "cells", int(lim_rep.get("late_from_turn") or 0))
                for arm, (k, n) in counts.items():
                    lo, hi = wilson_ci(k, n)
                    tmp.setdefault(arm, []).append((float(limit_s), 100 * lo, 100 * hi))
            errs = tmp or None
        multi_line(
            FIG / f"compaction_{metric}_vs_limit.svg",
            f"Long-horizon B (compaction): {ylabel} vs max context"
            + (" (session-level Wilson 95% CI)" if errs else ""),
            curves,
            "Max context (tokens)",
            ylabel,
            errors=errs,
        )


def main() -> None:
    from_longhorizon()
    from_gate()
    from_quality()
    from_compaction()
    print("wrote figures to", FIG)


if __name__ == "__main__":
    main()
