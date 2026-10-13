# Dynamic CLM (DynCLM)

A context-management method where the model marks context as dead via append-only soft-invalidation markers, and a cost-model-driven scheduler fires a single batched physical rewrite when the deferred cost of carrying dead tokens exceeds the fixed cost of the rewrite event.

**Naming:** Spell out "Dynamic CLM" or use **DynCLM** in writeups. Do not abbreviate as DCLM (that name is taken by DataComp-LM).

## DynCLM vs Codex-style compaction

DynCLM is still compaction in the broad sense (shrink context under a budget). It is **not** the same as Codex-style summarization: summarizers fold older turns into a **lossy** note; DynCLM/CLM **soft-invalidate** known-stale spans and later **physically drop** them while keeping survivors **verbatim**. DynCLM additionally **defers** that rewrite via a cost model (`β` / `γ` / `Δ`); immediate-rewrite CLM reclaims every edit.

Arm definitions and how to read per-suite gaps: **[docs/dynclm-vs-compaction.md](docs/dynclm-vs-compaction.md)**.

## Phases

1. **Append**: normal operation; prefix cache preserved.
2. **Soft invalidation**: append `<<<DYNCLM_IGNORE ...>>>` markers; still append-only.
3. **Deferred rewrite**: one batched physical rewrite when the cost model fires.

## Local stack

- Hardware: Apple M5 Max
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

Reports: `docs/dynclm-vs-compaction.md` (mechanism vs Codex-style compact), `docs/longhorizon-report.md` (primary for paper claim), `docs/quality-report.md`, `docs/interim-report.md`, `docs/report.md`.

## Layout

See `internal/` for scheduler, context manager, tracing, and eval harnesses. Results land in `results/`; reports in `docs/`.
