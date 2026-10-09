# Dynamic CLM (DynCLM)

A context-management method where the model marks context as dead via append-only soft-invalidation markers, and a cost-model-driven scheduler fires a single batched physical rewrite when the deferred cost of carrying dead tokens exceeds the fixed cost of the rewrite event.

**Naming:** Spell out "Dynamic CLM" or use **DynCLM** in writeups. Do not abbreviate as DCLM (that name is taken by DataComp-LM).

## Phases

1. **Append** — normal operation; prefix cache preserved.
2. **Soft invalidation** — append `<<<DYNCLM_IGNORE ...>>>` markers; still append-only.
3. **Deferred rewrite** — one batched physical rewrite when the cost model fires.

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
```

Reports: `docs/quality-report.md` (accuracy deltas), `docs/interim-report.md`, `docs/report.md`.  
**Note:** the ~38.6% re-prefill figure is a **cost** metric; quality is reported in percentage points of graded accuracy.

## Layout

See `internal/` for scheduler, context manager, tracing, and eval harnesses. Results land in `results/`; reports in `docs/`.
