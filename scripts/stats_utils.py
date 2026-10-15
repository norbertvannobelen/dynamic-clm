#!/usr/bin/env python3
"""Pure-stdlib statistics for DynCLM report generators.

Implements the pre-registered tests in docs/analysis-plan.md:
- Wilson score CI for proportions (probe/task success rates)
- One-sided Fisher exact test (session/task-level success counts)
- Welch two-sample t-test (continuous metrics; t CDF via incomplete beta)
- Percentile-bootstrap CI for means and mean differences
- Holm step-down adjustment for the primary-endpoint family

No third-party dependencies (reports must generate on a bare python3).
"""

from __future__ import annotations

import json
import math
import random
from pathlib import Path

Z95 = 1.959963984540054  # two-sided 95%


# ------------------------------------------------------- session-level counts

def session_late_pass(cell: dict, late_from_turn: int) -> bool | None:
    """Strict session pass: all late-window probes correct (pre-registered unit)."""
    probes = [
        t for t in cell.get("turns", [])
        if t.get("is_probe") and t.get("probe_correct") is not None
        and t.get("turn", 0) >= late_from_turn
    ]
    if not probes:
        return None
    return all(bool(t["probe_correct"]) for t in probes)


def lh_pass_counts(cells_dir: Path, late_from_turn: int) -> dict[str, tuple[int, int]]:
    """arm -> (pass, total) session-level counts from per-session cell files."""
    out: dict[str, tuple[int, int]] = {}
    if not cells_dir.exists():
        return out
    for arm_dir in sorted(cells_dir.glob("*")):
        if not arm_dir.is_dir():
            continue
        k = n = 0
        for f in sorted(arm_dir.glob("*.json")):
            ok = session_late_pass(json.loads(f.read_text()), late_from_turn)
            if ok is None:
                continue
            n += 1
            k += 1 if ok else 0
        if n:
            out[arm_dir.name] = (k, n)
    return out


# ---------------------------------------------------------------- proportions

def wilson_ci(k: int, n: int, z: float = Z95) -> tuple[float, float]:
    """Wilson score interval for a binomial proportion. Returns (lo, hi) in 0..1."""
    if n <= 0:
        return (0.0, 1.0)
    p = k / n
    denom = 1 + z * z / n
    center = (p + z * z / (2 * n)) / denom
    half = (z / denom) * math.sqrt(p * (1 - p) / n + z * z / (4 * n * n))
    return (max(0.0, center - half), min(1.0, center + half))


def fisher_exact_greater(a: int, b: int, c: int, d: int) -> float:
    """One-sided Fisher exact p for table [[a, b], [c, d]].

    Rows are groups (group1: a successes, b failures; group2: c, d).
    Returns P(X >= a) under the hypergeometric null with fixed margins,
    i.e. alternative "group1 success rate > group2 success rate".
    """
    n = a + b + c + d
    row1 = a + b
    col1 = a + c
    col2 = b + d

    def hypergeom_pmf(x: int) -> float:
        return math.comb(col1, x) * math.comb(col2, row1 - x) / math.comb(n, row1)

    lo = max(0, row1 - col2)
    hi = min(row1, col1)
    a = max(lo, min(a, hi))  # clamp into support
    total = 0.0
    for x in range(a, hi + 1):  # one-sided "greater": strict upper tail
        total += hypergeom_pmf(x)
    return min(1.0, total)


# ------------------------------------------------------------- t distribution

def _betacf(a: float, b: float, x: float) -> float:
    """Continued fraction for the incomplete beta function (Numerical Recipes)."""
    max_iter, eps, fpmin = 200, 3e-14, 1e-300
    qab, qap, qam = a + b, a + 1.0, a - 1.0
    c = 1.0
    d = 1.0 - qab * x / qap
    if abs(d) < fpmin:
        d = fpmin
    d = 1.0 / d
    h = d
    for m in range(1, max_iter + 1):
        m2 = 2 * m
        aa = m * (b - m) * x / ((qam + m2) * (a + m2))
        d = 1.0 + aa * d
        if abs(d) < fpmin:
            d = fpmin
        c = 1.0 + aa / c
        if abs(c) < fpmin:
            c = fpmin
        d = 1.0 / d
        h *= d * c
        aa = -(a + m) * (qab + m) * x / ((a + m2) * (qap + m2))
        d = 1.0 + aa * d
        if abs(d) < fpmin:
            d = fpmin
        c = 1.0 + aa / c
        if abs(c) < fpmin:
            c = fpmin
        d = 1.0 / d
        delta = d * c
        h *= delta
        if abs(delta - 1.0) < eps:
            break
    return h


def _betai(a: float, b: float, x: float) -> float:
    """Regularized incomplete beta function I_x(a, b)."""
    if x <= 0.0:
        return 0.0
    if x >= 1.0:
        return 1.0
    bt = math.exp(
        math.lgamma(a + b) - math.lgamma(a) - math.lgamma(b)
        + a * math.log(x) + b * math.log(1.0 - x)
    )
    if x < (a + 1.0) / (a + b + 2.0):
        return bt * _betacf(a, b, x) / a
    return 1.0 - bt * _betacf(b, a, 1.0 - x) / b


def student_t_sf(t: float, df: float) -> float:
    """Survival function P(T > t) for Student's t with df degrees of freedom."""
    if df <= 0:
        return 0.5
    x = df / (df + t * t)
    ib = _betai(df / 2.0, 0.5, x)
    return ib / 2.0 if t >= 0 else 1.0 - ib / 2.0


def welch_t(x: list[float], y: list[float]) -> tuple[float, float, float]:
    """Welch two-sample t-test. Returns (t_stat, df, two-sided p).

    Direction: positive t means mean(x) > mean(y).
    """
    n1, n2 = len(x), len(y)
    if n1 < 2 or n2 < 2:
        return (0.0, 1.0, 1.0)
    m1, m2 = sum(x) / n1, sum(y) / n2
    v1 = sum((xi - m1) ** 2 for xi in x) / (n1 - 1)
    v2 = sum((yi - m2) ** 2 for yi in y) / (n2 - 1)
    vn1, vn2 = v1 / n1, v2 / n2
    denom = math.sqrt(vn1 + vn2)
    if denom == 0:
        # zero variance in both arms: degenerate; significant iff means differ
        return (0.0, 1.0, 1.0 if m1 == m2 else 1e-12)
    t = (m1 - m2) / denom
    num = (vn1 + vn2) ** 2
    den = (vn1**2) / (n1 - 1) + (vn2**2) / (n2 - 1)
    df = num / den if den > 0 else 1.0
    p_two = 2.0 * student_t_sf(abs(t), df)
    return (t, df, min(1.0, p_two))


# ------------------------------------------------------------------ bootstrap

def bootstrap_ci_mean(xs: list[float], B: int = 10000, alpha: float = 0.05,
                      seed: int = 20261015) -> tuple[float, float]:
    """Percentile bootstrap CI for the mean of a single sample."""
    if not xs:
        return (0.0, 0.0)
    if len(xs) == 1:
        return (xs[0], xs[0])
    rng = random.Random(seed)
    n = len(xs)
    stats = []
    for _ in range(B):
        stats.append(sum(rng.choices(xs, k=n)) / n)
    stats.sort()
    lo = stats[int((alpha / 2) * B)]
    hi = stats[min(B - 1, int((1 - alpha / 2) * B))]
    return (lo, hi)


def bootstrap_ci_diff(xs: list[float], ys: list[float], B: int = 10000,
                      alpha: float = 0.05, seed: int = 20261015) -> tuple[float, float]:
    """Percentile bootstrap CI for mean(xs) - mean(ys)."""
    if not xs or not ys:
        return (0.0, 0.0)
    rng = random.Random(seed)
    n1, n2 = len(xs), len(ys)
    stats = []
    for _ in range(B):
        mx = sum(rng.choices(xs, k=n1)) / n1
        my = sum(rng.choices(ys, k=n2)) / n2
        stats.append(mx - my)
    stats.sort()
    lo = stats[int((alpha / 2) * B)]
    hi = stats[min(B - 1, int((1 - alpha / 2) * B))]
    return (lo, hi)


# ---------------------------------------------------------------- multiplicity

def holm_adjust(pvals: list[float]) -> list[float]:
    """Holm step-down adjusted p-values, returned in the original order."""
    m = len(pvals)
    order = sorted(range(m), key=lambda i: pvals[i])
    adj = [0.0] * m
    running = 0.0
    for rank, i in enumerate(order):
        running = max(running, min(1.0, (m - rank) * pvals[i]))
        adj[i] = running
    return adj


# ------------------------------------------------------------------ formatting

def fmt_prop_ci(k: int, n: int, nd: int = 1, show_n: bool = True) -> str:
    """e.g. '100.0% [69.2, 100.0] (n=10)'."""
    if n <= 0:
        return "-"
    lo, hi = wilson_ci(k, n)
    s = f"{100 * k / n:.{nd}f}% [{100 * lo:.{nd}f}, {100 * hi:.{nd}f}]"
    return s + (f" (n={n})" if show_n else "")


def fmt_mean_ci(xs: list[float], nd: int = 1) -> str:
    """e.g. '365.9 [351.2, 381.0]'."""
    if not xs:
        return "-"
    mean = sum(xs) / len(xs)
    lo, hi = bootstrap_ci_mean(xs)
    return f"{mean:.{nd}f} [{lo:.{nd}f}, {hi:.{nd}f}]"


def fmt_p(p: float) -> str:
    if p < 1e-4:
        return f"{p:.1e}"
    return f"{p:.4f}".rstrip("0").rstrip(".") if p < 0.01 else f"{p:.3f}"
