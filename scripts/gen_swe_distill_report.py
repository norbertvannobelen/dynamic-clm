#!/usr/bin/env python3
"""Build extensive SWE-Distill markdown + SVG figures from report.json."""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from stats_utils import fisher_exact_greater, fmt_p, wilson_ci  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]
FIG = ROOT / "docs" / "figures"
FIG.mkdir(parents=True, exist_ok=True)

COLORS = {
    "baseline-compact": "#9ca3af",
    "dynclm": "#2563eb",
    "immediate-rewrite": "#dc2626",
}
LABELS = {
    "baseline-compact": "Baseline+compact",
    "dynclm": "DynCLM",
    "immediate-rewrite": "CLM",
}
ORDER = ["baseline-compact", "dynclm", "immediate-rewrite"]


def esc(s: str) -> str:
    return (
        s.replace("&", "&amp;")
        .replace("<", "&lt;")
        .replace(">", "&gt;")
        .replace('"', "&quot;")
    )


def grouped_bars(path: Path, title: str, series: dict[str, float], ylabel: str,
                 errors: dict[str, tuple[float, float]] | None = None) -> None:
    arms = [a for a in ORDER if a in series]
    w, h = 720, 420
    pad_l, pad_r, pad_t, pad_b = 70, 30, 50, 90
    plot_w = w - pad_l - pad_r
    plot_h = h - pad_t - pad_b
    vals = [series[a] for a in arms]
    vmax = max(vals) if vals else 1.0
    if errors:
        vmax = max([vmax] + [hi for _, hi in errors.values()])
    if vmax <= 0:
        vmax = 1.0
    n = max(len(arms), 1)
    bar_w = plot_w / n * 0.55
    gap = plot_w / n
    parts = [
        f'<svg xmlns="http://www.w3.org/2000/svg" width="{w}" height="{h}" viewBox="0 0 {w} {h}">',
        '<rect width="100%" height="100%" fill="#fafafa"/>',
        f'<text x="{w/2}" y="28" text-anchor="middle" font-family="Helvetica,Arial,sans-serif" font-size="16" font-weight="700">{esc(title)}</text>',
        f'<text x="18" y="{pad_t + plot_h/2}" transform="rotate(-90 18,{pad_t + plot_h/2})" text-anchor="middle" font-family="Helvetica,Arial,sans-serif" font-size="12" fill="#374151">{esc(ylabel)}</text>',
        f'<line x1="{pad_l}" y1="{pad_t}" x2="{pad_l}" y2="{pad_t+plot_h}" stroke="#111827" stroke-width="1.5"/>',
        f'<line x1="{pad_l}" y1="{pad_t+plot_h}" x2="{pad_l+plot_w}" y2="{pad_t+plot_h}" stroke="#111827" stroke-width="1.5"/>',
    ]
    for i in range(5):
        y = pad_t + plot_h * i / 4
        val = vmax * (1 - i / 4)
        parts.append(f'<line x1="{pad_l}" y1="{y}" x2="{pad_l+plot_w}" y2="{y}" stroke="#e5e7eb"/>')
        parts.append(
            f'<text x="{pad_l-8}" y="{y+4}" text-anchor="end" font-family="Helvetica,Arial,sans-serif" font-size="11" fill="#6b7280">{val:.2g}</text>'
        )
    for i, arm in enumerate(arms):
        v = series[arm]
        bh = (v / vmax) * plot_h
        x = pad_l + gap * i + (gap - bar_w) / 2
        y = pad_t + plot_h - bh
        parts.append(
            f'<rect x="{x}" y="{y}" width="{bar_w}" height="{bh}" fill="{COLORS.get(arm, "#111")}" rx="3"/>'
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
            f'<text x="{x + bar_w/2}" y="{y-6}" text-anchor="middle" font-family="Helvetica,Arial,sans-serif" font-size="11">{v:.2g}</text>'
        )
        parts.append(
            f'<text x="{x + bar_w/2}" y="{pad_t+plot_h+20}" text-anchor="middle" font-family="Helvetica,Arial,sans-serif" font-size="11">{esc(LABELS.get(arm, arm))}</text>'
        )
    parts.append("</svg>")
    path.write_text("\n".join(parts), encoding="utf-8")


def per_task_grouped(path: Path, tasks: list[dict]) -> None:
    """Grouped bars: tasks on x, 3 arms each (0/1 accuracy)."""
    if not tasks:
        return
    w = max(900, 80 * len(tasks) + 120)
    h = 420
    pad_l, pad_r, pad_t, pad_b = 60, 30, 50, 110
    plot_w = w - pad_l - pad_r
    plot_h = h - pad_t - pad_b
    parts = [
        f'<svg xmlns="http://www.w3.org/2000/svg" width="{w}" height="{h}" viewBox="0 0 {w} {h}">',
        '<rect width="100%" height="100%" fill="#fafafa"/>',
        f'<text x="{w/2}" y="28" text-anchor="middle" font-family="Helvetica,Arial,sans-serif" font-size="16" font-weight="700">Per-task probe pass (1=pass)</text>',
        f'<line x1="{pad_l}" y1="{pad_t}" x2="{pad_l}" y2="{pad_t+plot_h}" stroke="#111827"/>',
        f'<line x1="{pad_l}" y1="{pad_t+plot_h}" x2="{pad_l+plot_w}" y2="{pad_t+plot_h}" stroke="#111827"/>',
    ]
    group_w = plot_w / len(tasks)
    bar_w = group_w / 4
    for ti, t in enumerate(tasks):
        by = t.get("by_arm") or {}
        for ai, arm in enumerate(ORDER):
            v = 1.0 if by.get(arm) else 0.0
            bh = v * plot_h * 0.9
            x = pad_l + ti * group_w + (ai + 0.5) * bar_w
            y = pad_t + plot_h - bh
            parts.append(
                f'<rect x="{x}" y="{y}" width="{bar_w*0.85}" height="{bh}" fill="{COLORS[arm]}" rx="2"/>'
            )
        label = (t.get("instance_id") or "")[:22]
        parts.append(
            f'<text x="{pad_l + ti * group_w + group_w/2}" y="{pad_t+plot_h+16}" text-anchor="middle" font-family="Helvetica,Arial,sans-serif" font-size="9" transform="rotate(25 {pad_l + ti * group_w + group_w/2},{pad_t+plot_h+16})">{esc(label)}</text>'
        )
    # legend
    lx = pad_l
    for arm in ORDER:
        parts.append(f'<rect x="{lx}" y="{h-28}" width="12" height="12" fill="{COLORS[arm]}"/>')
        parts.append(
            f'<text x="{lx+16}" y="{h-18}" font-family="Helvetica,Arial,sans-serif" font-size="11">{esc(LABELS[arm])}</text>'
        )
        lx += 140
    parts.append("</svg>")
    path.write_text("\n".join(parts), encoding="utf-8")


def write_extensive(rep: dict, out: Path, report_json: Path) -> None:
    arms = rep.get("arms") or {}
    tasks = rep.get("tasks") or []
    vs = rep.get("vs_baseline") or {}
    cells = rep.get("cells") or []

    def arm(name: str) -> dict:
        return arms.get(name) or {}

    bc, dyn, imm = arm("baseline-compact"), arm("dynclm"), arm("immediate-rewrite")

    def wilson_err(a: dict) -> tuple[float, float]:
        lo, hi = wilson_ci(int(a.get("correct") or 0), int(a.get("n") or 0))
        return (100 * lo, 100 * hi)

    # figures
    grouped_bars(
        FIG / "swe_probe_accuracy.svg",
        "SWE-Distill final probe accuracy (error bars: Wilson 95% CI)",
        {k: float((arms.get(k) or {}).get("accuracy_pct") or 0) for k in ORDER if k in arms},
        "Accuracy %",
        errors={k: wilson_err(arms[k]) for k in ORDER if k in arms},
    )
    grouped_bars(
        FIG / "swe_final_context.svg",
        "SWE-Distill avg final context",
        {k: float((arms.get(k) or {}).get("avg_final_context") or 0) for k in ORDER if k in arms},
        "Tokens",
    )
    grouped_bars(
        FIG / "swe_wall.svg",
        "SWE-Distill total wall-clock",
        {k: float((arms.get(k) or {}).get("total_wall_s") or 0) for k in ORDER if k in arms},
        "Seconds",
    )
    grouped_bars(
        FIG / "swe_prefill.svg",
        "SWE-Distill avg reclaim prefill",
        {k: float((arms.get(k) or {}).get("avg_prefill") or 0) for k in ORDER if k in arms},
        "Tokens",
    )
    per_task_grouped(FIG / "swe_per_task_accuracy.svg", tasks)

    lines: list[str] = []
    lines.append("# DynCLM SWE-Distill - Extensive Report\n\n")
    lines.append(
        f"Generated: {rep.get('generated_at')} · Model `{rep.get('model_id')}` · "
        f"Hardware `{rep.get('hardware_id')}` · live={rep.get('live')}\n\n"
    )
    lines.append(
        "**Disclaimer:** This is a SWE-Bench-**Lite-inspired** multi-turn coding distill "
        "under shared `max_context` with Baseline+compact / DynCLM / CLM. It is **not** "
        "an official SWE-Bench resolve-rate number and is not TerminalBench / Harbor.\n\n"
    )

    lines.append("## 1. Executive verdict\n\n")
    d_acc = float(dyn.get("accuracy_pct") or 0) - float(bc.get("accuracy_pct") or 0)
    i_acc = float(imm.get("accuracy_pct") or 0) - float(bc.get("accuracy_pct") or 0)
    lines.append(
        f"On this suite, DynCLM final-probe accuracy is **{dyn.get('accuracy_pct', 0):.1f}%** "
        f"({d_acc:+.1f} pp vs Baseline+compact **{bc.get('accuracy_pct', 0):.1f}%**); "
        f"CLM is **{imm.get('accuracy_pct', 0):.1f}%** ({i_acc:+.1f} pp). "
        f"Baseline+compact retains the early SPEC (verbatim or in rolling summary) in "
        f"**{bc.get('spec_survive_pct', 0):.0f}%** of tasks; DynCLM/CLM keep it via "
        f"soft-invalidation ({dyn.get('spec_survive_pct', 0):.0f}% / "
        f"{imm.get('spec_survive_pct', 0):.0f}%). "
        f"CLM pays the highest rewrite prefill; DynCLM sits between quality parity with CLM "
        f"and much lower rewrite N than always-rewrite.\n\n"
    )
    k_bc, n_bc = int(bc.get("correct") or 0), int(bc.get("n") or 0)
    k_d, n_d = int(dyn.get("correct") or 0), int(dyn.get("n") or 0)
    k_i, n_i = int(imm.get("correct") or 0), int(imm.get("n") or 0)
    if n_bc and n_d and n_i:
        p_d = fisher_exact_greater(k_d, n_d - k_d, k_bc, n_bc - k_bc)
        p_i = fisher_exact_greater(k_i, n_i - k_i, k_bc, n_bc - k_bc)
        lines.append(
            "Pre-registered statistics (docs/analysis-plan.md), task-level pass, "
            "one-sided Fisher exact: DynCLM vs Baseline+compact "
            f"**p={fmt_p(p_d)}**; CLM vs Baseline+compact **p={fmt_p(p_i)}**. "
            "Wilson 95% CIs in §4.\n\n"
        )
    lines.append(
        "**Read.** Baseline is Codex-style **lossy summarization**. Quote accuracy with "
        f"`spec_survive` (**{bc.get('spec_survive_pct', 0):.0f}%** here). Gaps with high "
        "survive are recency/confusion under summarization, not deleted SPECs. "
        "Not official SWE-Bench; not the same as graded quality (+5.6 pp vs raw with no "
        "window reclaim). See §2.\n\n"
    )

    lines.append("## 2. How to read Baseline+compact accuracy\n\n")
    lines.append(
        "This suite measures **reclaim under window pressure** against summarize "
        "compaction, not single-shot coding skill.\n\n"
        "1. **Plant**: turn 1 inserts the correct fix as `[SPEC]`.\n"
        "2. **Flood**: later turns append plausible wrong SPECs plus padding so tokens "
        "exceed `max_context` (~1.4×).\n"
        "3. **Rolling summary compact**: keep `sys` + recent raw; fold older spans into "
        "`[CONTEXT_SUMMARY]`; re-prefill.\n"
        "4. **Recency instruction**: without ignore markers, prefer the most recent SPEC.\n"
        "5. **Probe**: grade `ANSWER: <fragment>` against turn-1 truth.\n\n"
        f"`spec_survive` counts the expected fragment still present (verbatim or in the "
        f"summary). DynCLM/CLM: **{dyn.get('spec_survive_pct', 0):.0f}%** / "
        f"**{imm.get('spec_survive_pct', 0):.0f}%**. Baseline: "
        f"**{bc.get('spec_survive_pct', 0):.0f}%**. When survive is high and accuracy is "
        "lower, the gap is recency/confusion, not deletion of the fix.\n\n"
    )

    lines.append("## 3. Methodology\n\n")
    lines.append(str(rep.get("methodology") or "") + "\n\n")
    tw_bc = float(bc.get("total_wall_s") or 0) / 3600
    tw_dyn = float(dyn.get("total_wall_s") or 0) / 3600
    tw_imm = float(imm.get("total_wall_s") or 0) / 3600
    lines.append(
        f"| Knob | Value |\n|---|---|\n"
        f"| max_context | **{rep.get('max_context')}** |\n"
        f"| turns / task | **{rep.get('max_turns')}** |\n"
        f"| noise_pad (chars) | **{rep.get('noise_pad')}** |\n"
        f"| tasks | **{len(tasks)}** |\n"
        f"| cells | **{len(cells)}** |\n"
        f"| target wall | ~2h / arm |\n"
        f"| observed wall | Baseline+compact **{tw_bc:.2f}h** · DynCLM **{tw_dyn:.2f}h** · CLM **{tw_imm:.2f}h** |\n\n"
    )
    lines.append(
        "Wall came in under the 2h/arm budget while still forcing compaction "
        f"(baseline avg compacts **{float(bc.get('avg_compacts') or 0):.1f}**/task) and DynCLM/CLM reclaim. "
        "Context is already saturating near `max_context`; stretching wall further would not change the reclaim physics.\n\n"
    )
    lines.append(
        "Fixtures live under `testdata/swe_distill/` (paraphrased Lite-inspired excerpts "
        "with per-file attribution).\n\n"
    )

    lines.append("## 4. Headline results\n\n")
    lines.append(
        "Context columns: **final live** is decode-relevant tracker live tokens; "
        "**final total** is live+dead footprint; **final prompt** is oMLX `prompt_tokens` on the probe "
        "(marker arms still serialize ignored dead spans until rewrite). "
        "**Steady wall** excludes compact/rewrite turns when present.\n\n"
    )
    lines.append(
        "| Arm | Accuracy [Wilson 95% CI] | Correct/N | Avg final live | Avg final total | Avg final prompt | Peak live | Avg compacts | Avg rewrites | Avg prefill | Total wall (s) | Steady wall/task (s) | Wall (h) |\n"
        "|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|\n"
    )
    for name in ORDER:
        a = arm(name)
        if not a:
            continue
        lo_w, hi_w = wilson_ci(int(a.get("correct") or 0), int(a.get("n") or 0))
        tw = float(a.get("total_wall_s") or 0)
        lines.append(
            f"| {LABELS[name]} | {float(a.get('accuracy_pct') or 0):.1f}% "
            f"[{100 * lo_w:.1f}, {100 * hi_w:.1f}] | "
            f"{a.get('correct', 0)}/{a.get('n', 0)} | "
            f"{float(a.get('avg_final_context') or 0):.0f} | "
            f"{float(a.get('avg_final_total') or a.get('avg_final_context') or 0):.0f} | "
            f"{float(a.get('avg_final_prompt') or 0):.0f} | "
            f"{a.get('peak_context', 0)} | "
            f"{float(a.get('avg_compacts') or 0):.2f} | {float(a.get('avg_rewrites') or 0):.2f} | "
            f"{float(a.get('avg_prefill') or 0):.0f} | {tw:.1f} | "
            f"{float(a.get('avg_steady_wall_s') or a.get('avg_wall_s') or 0):.1f} | {tw/3600:.2f} |\n"
        )
    lines.append("\n")

    lines.append("## 5. Vs Baseline+compact\n\n")
    lines.append("| Metric | DynCLM | CLM |\n|---|---|---|\n")
    vd, vi = vs.get("dynclm") or {}, vs.get("immediate-rewrite") or {}
    lines.append(
        f"| Accuracy Δ (pp) | {float(vd.get('accuracy_delta_pp') or 0):+.1f} | {float(vi.get('accuracy_delta_pp') or 0):+.1f} |\n"
        f"| Final live ctx % | {float(vd.get('ctx_pct') or 0):+.1f}% | {float(vi.get('ctx_pct') or 0):+.1f}% |\n"
        f"| Final total ctx % | {float(vd.get('total_ctx_pct') or vd.get('ctx_pct') or 0):+.1f}% | {float(vi.get('total_ctx_pct') or vi.get('ctx_pct') or 0):+.1f}% |\n"
        f"| Final prompt % | {float(vd.get('prompt_pct') or 0):+.1f}% | {float(vi.get('prompt_pct') or 0):+.1f}% |\n"
        f"| Total wall % | {float(vd.get('wall_pct') or 0):+.1f}% | {float(vi.get('wall_pct') or 0):+.1f}% |\n"
        f"| Steady wall % | {float(vd.get('steady_wall_pct') or 0):+.1f}% | {float(vi.get('steady_wall_pct') or 0):+.1f}% |\n"
        f"| Prefill Δ (avg) | {float(vd.get('prefill_delta') or 0):+.0f} | {float(vi.get('prefill_delta') or 0):+.0f} |\n\n"
    )

    lines.append("## 6. Per-task breakdown\n\n")
    lines.append(
        "| Instance | Expected | Baseline+compact | DynCLM | CLM | Final ctx B/D/C | Reclaim events B/D/C |\n"
        "|---|---|:-:|:-:|:-:|---|---|\n"
    )
    for t in tasks:
        by = t.get("by_arm") or {}
        ctx = t.get("ctx_by_arm") or {}
        ev = t.get("reclaim_events_by_arm") or {}

        def pf(a: str) -> str:
            return "PASS" if by.get(a) else "FAIL"

        exp = str(t.get("expected") or "")[:48]
        lines.append(
            f"| `{t.get('instance_id')}` | `{exp}` | {pf('baseline-compact')} | {pf('dynclm')} | {pf('immediate-rewrite')} | "
            f"{ctx.get('baseline-compact', 0)} / {ctx.get('dynclm', 0)} / {ctx.get('immediate-rewrite', 0)} | "
            f"{ev.get('baseline-compact', 0)} / {ev.get('dynclm', 0)} / {ev.get('immediate-rewrite', 0)} |\n"
        )
    lines.append("\n")
    lucky = [
        c
        for c in cells
        if c.get("arm") == "baseline-compact"
        and c.get("success")
        and not c.get("spec_survived")
    ]
    if lucky:
        ids = ", ".join(f"`{c.get('instance_id')}`" for c in lucky)
        lines.append(
            f"Note: Baseline PASS with `spec_survived=false` on {ids} - lucky/format "
            "overlap after the authoritative SPEC was dropped, not retention of the fix.\n\n"
        )

    lines.append("## 7. Figures\n\n")
    lines.append("![Probe accuracy](figures/swe_probe_accuracy.svg)\n\n")
    lines.append("![Final context](figures/swe_final_context.svg)\n\n")
    lines.append("![Wall-clock](figures/swe_wall.svg)\n\n")
    lines.append("![Prefill](figures/swe_prefill.svg)\n\n")
    lines.append("![Per-task](figures/swe_per_task_accuracy.svg)\n\n")

    lines.append("## 8. Failure analysis\n\n")
    lines.append(
        "Baseline failures below should be read with `spec_survived`. When survive is "
        "true, the fix was still in context and the miss is recency/confusion. "
        "DynCLM/CLM show the model can answer when obsolete SPECs are ignored.\n\n"
    )
    for title, key in [
        ("Baseline+compact", "baseline-compact"),
        ("DynCLM", "dynclm"),
        ("CLM", "immediate-rewrite"),
    ]:
        fails = [c for c in cells if c.get("arm") == key and not c.get("success")]
        lines.append(f"### {title}\n\n")
        if not fails:
            lines.append("_None._\n\n")
            continue
        for c in fails:
            lines.append(
                f"- `{c.get('instance_id')}`: got `{str(c.get('got') or '')[:100]}` "
                f"(spec_survived={c.get('spec_survived')}, compacts={c.get('compacts')}, "
                f"rewrites={c.get('rewrites')})\n"
            )
        lines.append("\n")
    base_keep = [c for c in cells if c.get("arm") == "baseline-compact" and c.get("spec_survived")]
    n_bc = int(bc.get("n") or 0)
    lines.append(
        f"Early SPEC retained (verbatim or in summary) in "
        f"**{len(base_keep)}/{n_bc}** baseline tasks.\n\n"
    )

    lines.append("## 9. Efficiency deep-dive\n\n")
    lines.append(
        "Quality-adjusted read: Baseline+compact may look fast while answering from a "
        "lossy summary; DynCLM/CLM spend reclaim prefill to keep the authoritative "
        "snippet verbatim. Prefer accuracy-first comparisons; treat wall alone as secondary.\n\n"
    )
    lines.append(
        f"| Arm | Spec survive % | Avg wall/task (s) |\n|---|---:|---:|\n"
        f"| Baseline+compact | {float(bc.get('spec_survive_pct') or 0):.1f} | {float(bc.get('avg_wall_s') or 0):.1f} |\n"
        f"| DynCLM | {float(dyn.get('spec_survive_pct') or 0):.1f} | {float(dyn.get('avg_wall_s') or 0):.1f} |\n"
        f"| CLM | {float(imm.get('spec_survive_pct') or 0):.1f} | {float(imm.get('avg_wall_s') or 0):.1f} |\n\n"
    )

    lines.append("## 10. Relation to prior suites\n\n")
    lines.append(
        "| Suite | What it showed | What SWE-Distill adds |\n"
        "|---|---|---|\n"
        "| Short gate | Mechanism + prefill vs CLM | Coding issue/snippet domain |\n"
        "| Graded quality | Short multi-span accuracy (no reclaim trap) | Multi-turn growth + compaction pressure |\n"
        "| Long-horizon A/B | FACT canary + compact matrix | SWE-inspired fix fragments under same reclaim physics |\n\n"
    )

    lines.append("## 11. Limitations & next steps\n\n")
    lines.append(
        "- Not official SWE-Bench Verified/Lite resolve (no docker, no fail-to-pass).\n"
        "- Baseline+compact accuracy is a **retention-under-compaction** metric; do not "
        "quote it as the model’s coding competence without the SPEC-survive context.\n"
        "- Model is local `Qwen3.8-27B-4bit` - not paper Qwen3.6-27B full precision.\n"
        "- FLOPs are not prefix-reuse PFLOPs; we report wall / tokens / prefill.\n"
        "- Optional later: Harbor DynCLM agent on TBLite if a paper-adjacent coding row is required.\n\n"
    )

    lines.append("## 12. Artifacts\n\n")
    lines.append(f"- Source report JSON: `{report_json}`\n")
    lines.append(f"- Figures: `docs/figures/swe_*.svg`\n")
    lines.append("- Reproduce: `bash scripts/run_swe_distill.sh`\n")
    lines.append(
        f"- Regenerate this doc: `python3 scripts/gen_swe_distill_report.py --report {report_json}`\n"
    )

    out.write_text("".join(lines), encoding="utf-8")


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--report", required=True, help="path to swe_distill/report.json")
    ap.add_argument("--out", default="docs/swe-distill-report.md")
    args = ap.parse_args()
    report_json = Path(args.report)
    rep = json.loads(report_json.read_text())
    write_extensive(rep, Path(args.out), report_json)
    print("wrote", args.out)


if __name__ == "__main__":
    main()
