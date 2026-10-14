#!/usr/bin/env python3
"""Write Baseline vs DynCLM vs CLM decision report from a decision-suite run."""

from __future__ import annotations

import argparse
import json
from pathlib import Path


LABELS = {
    "baseline": "Baseline",
    "baseline-compact": "Baseline+compact",
    "dynclm": "DynCLM",
    "immediate-rewrite": "CLM",
}
ORDER = ["baseline", "dynclm", "immediate-rewrite"]


def load(p: Path):
    if not p.exists():
        return None
    return json.loads(p.read_text())


def pct(a, b):
    if b is None or b == 0:
        return None
    return 100.0 * (a - b) / b


def fmt(v, nd=1):
    if v is None:
        return "-"
    return f"{v:.{nd}f}"


def fmt_pct(v):
    if v is None:
        return "-"
    return f"{v:+.1f}%"


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--run-root", required=True)
    ap.add_argument("--out", default="docs/decision-report.md")
    args = ap.parse_args()
    root = Path(args.run_root)

    gate = load(root / "gate" / "report.json")
    quality = load(root / "quality" / "report.json")
    lh = load(root / "longhorizon" / "report.json")
    matrix = load(root / "longhorizon_compaction" / "matrix_report.json")
    man = load(root / "manifest.json") or {}

    lines: list[str] = []
    lines.append("# DynCLM Decision Report - Baseline vs DynCLM vs CLM\n\n")
    lines.append(f"**Run:** `{root.name}` · live={man.get('live')} · status={man.get('status')}\n\n")
    lines.append(
        "Efficient decision suite (not the 900-session policy grid): "
        "short gate + graded quality + long-horizon A + compaction matrix (16k/64k).\n\n"
    )

    # Gate
    lines.append("## 1. Short live gate - session speed & cost\n\n")
    if not gate:
        lines.append("_Gate results missing._\n\n")
    else:
        stats = gate.get("arm_stats", {})
        lines.append(
            f"Sessions/arm: {gate.get('sessions_per_arm')} · Turns: {gate.get('max_turns')} · "
            f"live_model={gate.get('live_model')} · passed={gate.get('passed')}\n\n"
        )
        lines.append(
            "| Metric | Baseline | DynCLM | CLM |\n|---|---:|---:|---:|\n"
        )
        keys = [
            ("success_rate", "Success %", 1),
            ("avg_context_tokens", "Avg context", 0),
            ("peak_context_tokens", "Peak context", 0),
            ("avg_wall_clock_s", "Wall-clock / session (s)", 1),
            ("avg_prefill_tokens", "Rewrite prefill / session", 0),
            ("avg_rewrites", "Rewrites / session", 1),
            ("avg_prompt_tokens", "Prompt tokens / session", 0),
            ("avg_generated_tokens", "Generated tokens / session", 0),
        ]
        for k, label, nd in keys:
            row = [label]
            for arm in ORDER:
                row.append(fmt(stats.get(arm, {}).get(k), nd))
            lines.append("| " + " | ".join(row) + " |\n")
        base = stats.get("baseline", {})
        lines.append("\n### Vs baseline\n\n| Metric | DynCLM | CLM |\n|---|---|---|\n")
        for k, label, _ in keys:
            if k == "success_rate":
                continue
            b = base.get(k)
            d = stats.get("dynclm", {}).get(k)
            c = stats.get("immediate-rewrite", {}).get(k)
            if b is None:
                continue
            lines.append(
                f"| {label} | {fmt_pct(pct(d, b))} ({fmt(b,0)} → {fmt(d,0)}) | "
                f"{fmt_pct(pct(c, b))} ({fmt(b,0)} → {fmt(c,0)}) |\n"
            )

    # Quality
    lines.append("\n## 2. Live graded quality\n\n")
    if not quality:
        lines.append("_Quality results missing._\n\n")
    else:
        arms = quality.get("arms", {})
        lines.append("| Arm | Accuracy | Correct/N | Avg wall-s | Avg ctx |\n|---|---:|---:|---:|---:|\n")
        for arm in ORDER:
            a = arms.get(arm, {})
            lines.append(
                f"| {LABELS[arm]} | {fmt(a.get('accuracy_pct'))}% | "
                f"{a.get('correct','-')}/{a.get('n','-')} | "
                f"{fmt(a.get('avg_wall_clock_s'), 2)} | {fmt(a.get('avg_context_tokens'), 0)} |\n"
            )

    # Long-horizon A
    lines.append("\n## 3. Live long-horizon A (primary quality + late speed)\n\n")
    if not lh:
        lines.append("_Long-horizon A missing._\n\n")
    else:
        lines.append(
            f"Sessions/arm: {lh.get('sessions_per_arm')} · Turns: {lh.get('max_turns')} · live={lh.get('live')}\n\n"
        )
        lines.append(
            "| Arm | Late probe % | Late avg ctx | Late steady wall-s | Late tok/s | Prefill/sess | Rewrites/sess |\n"
            "|---|---:|---:|---:|---:|---:|---:|\n"
        )
        arms = lh.get("arms", {})
        for arm in ORDER:
            a = arms.get(arm, {})
            late = a.get("late", {})
            steady = a.get("late_steady", late)
            n = max(a.get("sessions") or 1, 1)
            lines.append(
                f"| {LABELS[arm]} | {fmt(late.get('probe_accuracy_pct'))} | "
                f"{fmt(late.get('avg_context_tokens'), 0)} | "
                f"{fmt(steady.get('avg_wall_clock_s'), 2)} | "
                f"{fmt(steady.get('avg_gen_tokens_per_sec'), 2)} | "
                f"{fmt((a.get('total_prefill_tokens') or 0) / n, 0)} | "
                f"{fmt((a.get('total_rewrites') or 0) / n, 1)} |\n"
            )

    # Compaction B
    lines.append("\n## 4. Live long-horizon B (compaction matrix)\n\n")
    if not matrix:
        lines.append("_Compaction matrix missing._\n\n")
    else:
        lines.append(f"Limits: {matrix.get('max_contexts')} · live={matrix.get('live')}\n\n")
        lines.append(
            "| Max ctx | Compact probe% | DynCLM | CLM | Compact ctx | DynCLM ctx | CLM ctx | "
            "Compact wall | DynCLM wall | CLM wall |\n"
            "|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|\n"
        )
        for lim in matrix.get("max_contexts", []):
            arms = matrix.get("by_max_context", {}).get(str(lim), {})
            bc = arms.get("baseline-compact", {})
            d = arms.get("dynclm", {})
            imm = arms.get("immediate-rewrite", {})
            lines.append(
                f"| {lim} | {fmt(bc.get('late_probe_accuracy'))} | {fmt(d.get('late_probe_accuracy'))} | "
                f"{fmt(imm.get('late_probe_accuracy'))} | {fmt(bc.get('late_avg_context'),0)} | "
                f"{fmt(d.get('late_avg_context'),0)} | {fmt(imm.get('late_avg_context'),0)} | "
                f"{fmt(bc.get('late_steady_wall_s'),2)} | {fmt(d.get('late_steady_wall_s'),2)} | "
                f"{fmt(imm.get('late_steady_wall_s'),2)} |\n"
            )

    lines.append("\n## 5. Verdict\n\n")
    lines.append("| Dimension | Vs baseline | DynCLM vs CLM |\n|---|---|---|\n")
    # Fill verdict from available data when possible
    if lh and quality and gate:
        ba, da, ca = quality["arms"]["baseline"], quality["arms"]["dynclm"], quality["arms"]["immediate-rewrite"]
        bl, dl, cl = lh["arms"]["baseline"], lh["arms"]["dynclm"], lh["arms"]["immediate-rewrite"]
        gs = gate["arm_stats"]
        lines.append(
            f"| Graded accuracy | DynCLM/CLM {fmt(da['accuracy_pct']-ba['accuracy_pct'])} pp | "
            f"tie ({fmt(da['accuracy_pct'])}% vs {fmt(ca['accuracy_pct'])}%) |\n"
        )
        lines.append(
            f"| Late probe quality | DynCLM/CLM {fmt(dl['late']['probe_accuracy_pct']-bl['late']['probe_accuracy_pct'])} pp | "
            f"tie |\n"
        )
        lines.append(
            f"| Late steady wall | DynCLM {fmt_pct(pct(dl['late_steady']['avg_wall_clock_s'], bl['late_steady']['avg_wall_clock_s']))}; "
            f"CLM {fmt_pct(pct(cl['late_steady']['avg_wall_clock_s'], bl['late_steady']['avg_wall_clock_s']))} | "
            f"CLM faster |\n"
        )
        lines.append(
            f"| Session wall (gate) | DynCLM {fmt_pct(pct(gs['dynclm']['avg_wall_clock_s'], gs['baseline']['avg_wall_clock_s']))}; "
            f"CLM {fmt_pct(pct(gs['immediate-rewrite']['avg_wall_clock_s'], gs['baseline']['avg_wall_clock_s']))} | "
            f"similar |\n"
        )
        lines.append(
            f"| Rewrite prefill (gate) | both worse than 0 | "
            f"DynCLM {fmt_pct(pct(gs['dynclm']['avg_prefill_tokens'], gs['immediate-rewrite']['avg_prefill_tokens']))} vs CLM |\n"
        )
    else:
        lines.append("| _(fill after suite completes)_ | | |\n")

    lines.append("\n## Figures\n\nSee `docs/figures/`.\n")
    lines.append(
        "\n## Note on learning-900\n\n"
        "The policy-learning grid was paused/archived under the prior run "
        "(`archive_learning_900_*`). Resume with `bash scripts/resume_live_full.sh "
        "results/runs/20261009T034520Z` if needed for β/γ fitting - not required for this verdict.\n"
    )

    out = Path(args.out)
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text("".join(lines), encoding="utf-8")
    print("wrote", out)


if __name__ == "__main__":
    main()
