# SWE-Distill fixtures

SWE-Bench-**Lite-inspired** multi-turn coding tasks for DynCLM reclaim evaluation.

## Realistic protocol

1. Turn 1 plants an authoritative `[SPEC]` with the correct fix fragment.
2. Later turns append **plausible but wrong** reviewer/tool SPECs.
3. Shared `max_context` forces Baseline+compact to apply a **Codex-style rolling summary** (keep recent raw; fold older spans into `[CONTEXT_SUMMARY]`); DynCLM/CLM soft-ignore obsolete SPECs.
4. Final probe asks for the fix fragment; grade `ANSWER: <fragment>`.

Wrong answers are real alternative implementations (not synthetic canaries). Baseline uses Codex-style rolling summary; quote accuracy together with `spec_survive` (current live run: 50% accuracy, 100% survive).

**Serving:** DynCLM/CLM prompts use `ContextTextForServing`: dead spans become short ignore stubs (bodies omitted) so Chat latency tracks live context. Rewrite still batches physical reclaim of the tracker.

Not an official SWE-Bench resolve-rate evaluation.
