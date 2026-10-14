# Dynamic CLM (DynCLM)

**Premise.** CLM soft-marks stale context and physically rewrites every turn that has dead text - correct reclaim, but rewrite/re-prefill every time. **DynCLM** uses the **same** mark → drop → verbatim-survivors methodology, but marks internally and **defers** the rewrite until a cost model says we are tight on budget/memory. Where per-turn rewrite is expensive, that should cut rewrite load while keeping CLM-like quality on selective-ignore canaries.

**Mac / bandwidth.** One competing view is that CLM is a poor fit for Mac due to memory-bandwidth limits on always-rewrite/re-prefill. This repo compares CLM (and DynCLM) on Apple Silicon — so far **M5 Max** — to show either that CLM works there, or that DynCLM’s **hardware profile** (calibrated `ρ`) can adapt reclaim timing where narrower SKUs (**M5 Pro**, base **M5**) may still hit that bandwidth ceiling.

Method + math: **[docs/method.md](docs/method.md)**. Live proof attempt: **[docs/report.md](docs/report.md)**.

**Naming:** Spell out "Dynamic CLM" or use **DynCLM** in writeups. Do not abbreviate as DCLM (that name is taken by DataComp-LM).

## Protocols at a glance

| | Summarize compact | CLM | DynCLM |
|---|---|---|---|
| Reclaim | Lossy `[CONTEXT_SUMMARY]` when over window | Soft-ignore + rewrite every edit | Soft-ignore + **batched** rewrite |
| Survivors | Paraphrase + recent raw | Verbatim live spans | Verbatim live spans |

Full comparison and how markers/rewrites work: [docs/method.md](docs/method.md).

## Phases (DynCLM)

1. **Append**: normal operation; prefix cache preserved.
2. **Soft invalidation**: append `<<<DYNCLM_IGNORE ...>>>` markers; still append-only.
3. **Deferred rewrite**: one batched physical rewrite when the cost model fires (same drop as CLM).

## Local stack

- Hardware: Apple M5 Max (ship evidence today; M5 Pro / base M5 hardware profiles still open)
- Server: [oMLX](http://127.0.0.1:8000) (OpenAI-compatible)
- Gate baseline model: `Qwen3.8-27B-4bit`

Set `OMLX_API_KEY` (or rely on `~/.omlx/settings.json`). Never commit keys.

## Commands

```bash
make build
make test
make calibrate   # measure r, k_d, C_fixed → ρ
make gate        # Phase A hypothesis gate + interim report
make learning    # Phase B (only after gate pass)
./bin/dynclm-quality -model Qwen3.8-27B-4bit   # live graded quality suite
./bin/dynclm-longhorizon -model Qwen3.8-27B-4bit  # long-horizon quality + decode speed
```

Docs: `docs/method.md` (premise + method), `docs/report.md` (results), `docs/longhorizon-report.md`, `docs/quality-report.md`, `docs/interim-report.md`.

## Layout

See `internal/` for scheduler, context manager, tracing, and eval harnesses. Results land in `results/`; reports in `docs/`.
