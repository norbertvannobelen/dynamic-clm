#!/usr/bin/env python3
"""Generate SVG charts for DynCLM reports (stdlib only)."""

from __future__ import annotations

import json
import math
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
FIG = ROOT / "docs" / "figures"
FIG.mkdir(parents=True, exist_ok=True)

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


def esc(s: str) -> str:
    return (
        s.replace("&", "&amp;")
        .replace("<", "&lt;")
        .replace(">", "&gt;")
        .replace('"', "&quot;")
    )


def grouped_bars(path: Path, title: str, series: dict[str, float], ylabel: str) -> None:
    """series: arm -> value"""
    arms = [a for a in ["baseline", "baseline-compact", "dynclm", "immediate-rewrite"] if a in series]
    if not arms:
        arms = list(series.keys())
    w, h = 720, 420
    pad_l, pad_r, pad_t, pad_b = 70, 30, 50, 90
    plot_w = w - pad_l - pad_r
    plot_h = h - pad_t - pad_b
    vals = [series[a] for a in arms]
    vmax = max(vals) if vals else 1
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
        parts.append(
            f'<text x="{x + bar_w/2}" y="{y-6}" text-anchor="middle" font-family="Helvetica,Arial,sans-serif" font-size="11" fill="#111827">{v:.2g}</text>'
        )
        label = LABELS.get(arm, arm)
        parts.append(
            f'<text x="{x + bar_w/2}" y="{pad_t+plot_h+20}" text-anchor="middle" font-family="Helvetica,Arial,sans-serif" font-size="11" fill="#111827">{esc(label)}</text>'
        )

    parts.append("</svg>")
    path.write_text("\n".join(parts), encoding="utf-8")


def multi_line(path: Path, title: str, curves: dict[str, list[tuple[float, float]]], xlabel: str, ylabel: str) -> None:
    """curves: arm -> list of (x,y)"""
    w, h = 780, 440
    pad_l, pad_r, pad_t, pad_b = 70, 40, 50, 70
    plot_w = w - pad_l - pad_r
    plot_h = h - pad_t - pad_b
    all_x, all_y = [], []
    for pts in curves.values():
        for x, y in pts:
            all_x.append(x)
            all_y.append(y)
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
        parts.append(
            f'<rect x="{legend_x}" y="{legend_y + j*18 - 8}" width="12" height="12" fill="{color}"/>'
        )
        parts.append(
            f'<text x="{legend_x + 18}" y="{legend_y + j*18 + 2}" font-family="Helvetica,Arial,sans-serif" font-size="12" fill="#111827">{esc(LABELS.get(arm, arm))}</text>'
        )

    parts.append("</svg>")
    path.write_text("\n".join(parts), encoding="utf-8")


def from_longhorizon() -> None:
    p = ROOT / "results" / "longhorizon" / "report.json"
    if not p.exists():
        return
    rep = json.loads(p.read_text())
    arms = rep.get("arms", {})
    # bar charts late metrics
    grouped_bars(
        FIG / "lh_late_probe_accuracy.svg",
        "Long-horizon A: late probe accuracy",
        {a: arms[a]["late"]["probe_accuracy_pct"] for a in arms},
        "Accuracy (%)",
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

    # turn curves averaged across sessions
    cells = ROOT / "results" / "longhorizon" / "cells"
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
    p = ROOT / "results" / "gate" / "report.json"
    if not p.exists():
        return
    rep = json.loads(p.read_text())
    stats = rep.get("arm_stats", {})
    grouped_bars(
        FIG / "gate_context.svg",
        "Gate (short multi-turn): avg context length",
        {a: stats[a]["avg_context_tokens"] for a in stats},
        "Tokens",
    )
    grouped_bars(
        FIG / "gate_prefill.svg",
        "Gate (short multi-turn): avg rewrite prefill tokens",
        {a: stats[a]["avg_prefill_tokens"] for a in stats},
        "Tokens",
    )
    grouped_bars(
        FIG / "gate_prompt.svg",
        "Gate (short multi-turn): avg prompt tokens / session",
        {a: stats[a]["avg_prompt_tokens"] for a in stats},
        "Tokens",
    )


def from_quality() -> None:
    p = ROOT / "results" / "quality" / "report.json"
    if not p.exists():
        return
    rep = json.loads(p.read_text())
    arms = rep.get("arms", {})
    grouped_bars(
        FIG / "quality_accuracy.svg",
        "Short quality suite: graded accuracy",
        {a: arms[a]["accuracy_pct"] for a in arms},
        "Accuracy (%)",
    )


def from_compaction() -> None:
    candidates = [
        ROOT / "results" / "longhorizon_compaction_live" / "matrix_report.json",
        ROOT / "results" / "longhorizon_compaction" / "matrix_report.json",
    ]
    p = next((c for c in candidates if c.exists()), None)
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
        multi_line(
            FIG / f"compaction_{metric}_vs_limit.svg",
            f"Long-horizon B (compaction): {ylabel} vs max context",
            curves,
            "Max context (tokens)",
            ylabel,
        )


def main() -> None:
    from_longhorizon()
    from_gate()
    from_quality()
    from_compaction()
    print("wrote figures to", FIG)


if __name__ == "__main__":
    main()
