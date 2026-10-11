# SWE-Distill fixtures

SWE-Bench-**Lite-inspired** multi-turn coding tasks for DynCLM reclaim evaluation.

## Realistic protocol

1. Turn 1 plants an authoritative `[SPEC]` with the correct fix fragment.
2. Later turns append **plausible but wrong** reviewer/tool SPECs (recency trap).
3. Shared `max_context` forces Baseline+compact to drop early spans; DynCLM/CLM soft-ignore obsolete SPECs.
4. Final probe asks for the fix fragment; grade `ANSWER: <fragment>`.

Wrong answers are real alternative implementations (not synthetic canaries). Baseline is expected to be **weaker** (follows recent wrong SPEC after compaction), not forced to 0%. Live models may still score some baseline passes from partial context or priors — that is acceptable realism.

**Serving:** DynCLM/CLM prompts use `ContextTextForServing` — dead spans become short ignore stubs (bodies omitted) so Chat latency tracks live context. Rewrite still batches physical reclaim of the tracker.

Not an official SWE-Bench resolve-rate evaluation.
