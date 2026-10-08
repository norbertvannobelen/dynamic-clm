# §0 Verification — Context Language Models (CLM)

Verified 2026-10-08 against public sources (arXiv HTML, GitHub, WAI blog).

## Paper identity

| Field | Value |
|---|---|
| Title | Context Language Models |
| Authors | Rulin Shao, Shannon Zejiang Shen, Junjie Oscar Yin, Yuetai Li, Minheng Wang, Hamish Ivison, Radha Poovendran, Nathan Lambert, Teng Xiao, Mike Lewis, Wen-tau Yih, Luke Zettlemoyer, Pang Wei Koh |
| arXiv | [2609.37725](https://arxiv.org/abs/2609.37725) |
| Code | [facebookresearch/context-language-models](https://github.com/facebookresearch/context-language-models) |
| SCR code | `suffix_cache_reuse/` in that repo |

## Quantitative claims (treat as reproduction targets)

| Claim (handoff wording) | Verified paper wording | Status |
|---|---|---|
| ~35% server-side compute reduction from SCR | "further reducing server-side compute by **35%** relative to standard SGLang at matched performance" | Confirmed |
| +11.4% on BrowseComp-Plus | "**11.4%** higher accuracy with **21.5%** fewer FLOPs on BrowseComp-Plus" | Confirmed (accuracy delta + FLOP reduction) |
| +5% on EdgeBench | on 12-hour EdgeBench (10-task subset): "**5%** higher scores with **59%** fewer FLOPs" | Confirmed |
| 59% FLOP reduction on 12-hour repo-optimization task | Same EdgeBench claim: **59%** fewer prefix-reuse FLOPs | Confirmed (EdgeBench, not a separate unnamed task) |

SCR BrowseComp-Plus serving detail (WAI blog / paper §5.3): accuracy matched at 60.2%; compute 10.98 → 7.14 PFLOPs/question (~65% of standard SGLang).

## SCR implementation details

| Detail | Verified |
|---|---|
| K=6 span cap | Yes — "relocates up to K of them, largest first (K=6 in the main text)"; sensitivity over `{1,2,3,6,12,64}` saturates by K=6 |
| RoPE re-rotation | Yes — "re-rotates the keys' rotary position encodings to their new positions" |
| Recurrent-state snapshots for hybrid models | Yes — snapshot recurrent state before edit; continue from snapshot for linear-attention layers (e.g. Qwen3.6-27B hybrid) |

## Implication for DynCLM

Paper and code are accessible. Numbers above are **targets to reproduce**, not established facts for our method. Our gate and Phase B stand on their own against the local oMLX baseline even if CLM harness reproduction is incomplete.
