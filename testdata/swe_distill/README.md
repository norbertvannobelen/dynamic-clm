# SWE-Distill fixtures

Hand-curated, SWE-Bench-**Lite-inspired** coding excerpts for the DynCLM
multi-turn reclaim suite. These are short paraphrased issue/snippet pairs used
to stress context compaction vs soft-invalidation — **not** full repository
checkouts and **not** an official SWE-Bench resolve evaluation.

Each JSON file fields: `instance_id`, `repo`, `issue`, `correct_snippet`,
`wrong_snippets`, `expected_answer`, `probe_question`, `attribution`.
