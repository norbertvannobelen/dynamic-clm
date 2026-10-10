# SWE-Distill fixtures

Hand-curated, SWE-Bench-**Lite-inspired** coding excerpts for the DynCLM
multi-turn reclaim suite.

**Anti-parametric design:** each task’s graded `expected_answer` is a unique
`DYNCLM_CANARY_*` token that appears only in the turn-1 authoritative `[SPEC]`.
Obsolete tool dumps push competing `DYNCLM_TRAP_*` tokens. Baseline+compact
must fail once the early SPEC is dropped; DynCLM/CLM should keep the canary via
soft-invalidation. This prevents “100% baseline” from model prior knowledge.

Not full repository checkouts and not an official SWE-Bench resolve evaluation.
