package contextmgr

import (
	"strings"
)

const summarySpanID = "compact-summary"

// CompactSummarizeToBudget implements a Codex-style rolling summary compact:
// keep keepIDs and a recent raw tail, fold older spans into one lossy
// [CONTEXT_SUMMARY] note capped to the remaining budget.
//
// Summary fill is hybrid (paper-plausible, not blind FIFO drop):
//   - ~1/3 of the summary budget reserves earliest [FACT]/[SPEC]/goal lines
//     so authoritative early requirements often survive in abbreviated form;
//   - the rest prefers newest notes so recent wrong dumps also appear.
// Over many compacts, early detail can still fade when the early quota is
// overcrowded - mild information loss, not a guaranteed recency trap.
//
// Returns tokens removed from dropped spans (before summary insert), spans
// removed, and surviving token count (re-prefill estimate).
func (t *Tracker) CompactSummarizeToBudget(budget int, keepIDs map[string]bool) (removedTokens, removedSpans, surviving int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if budget < 0 {
		budget = 0
	}
	total := 0
	for _, sp := range t.spans {
		total += sp.Tokens
	}
	if total <= budget {
		return 0, 0, total
	}

	keep := map[string]bool{}
	for id := range keepIDs {
		keep[id] = true
	}

	// Recent raw target: leave room for a non-trivial summary (~20% of budget).
	rawBudget := (budget * 4) / 5
	if rawBudget < budget/2 {
		rawBudget = budget / 2
	}

	// Keep a contiguous recent suffix that fits rawBudget, plus keepIDs
	// (even if they sit earlier). Do not skip oversized middle spans to
	// "reach" older needles - that would not match Codex-style compact.
	n := len(t.spans)
	keepIdx := make([]bool, n)
	rawUsed := 0
	for i, sp := range t.spans {
		if keep[sp.ID] {
			keepIdx[i] = true
			rawUsed += sp.Tokens
		}
	}
	for i := n - 1; i >= 0; i-- {
		if keepIdx[i] {
			continue
		}
		sp := t.spans[i]
		if rawUsed+sp.Tokens > rawBudget {
			break
		}
		keepIdx[i] = true
		rawUsed += sp.Tokens
	}

	var dropped []Span
	for i, sp := range t.spans {
		if keepIdx[i] {
			continue
		}
		dropped = append(dropped, sp)
		removedTokens += sp.Tokens
		removedSpans++
	}
	if removedSpans == 0 {
		// Nothing droppable under raw policy - fall back to dropping oldest non-keep.
		for i, sp := range t.spans {
			if keep[sp.ID] {
				continue
			}
			keepIdx[i] = false
			dropped = append(dropped, sp)
			removedTokens += sp.Tokens
			removedSpans++
			break
		}
		if removedSpans == 0 {
			return 0, 0, total
		}
	}

	summaryBudget := budget - rawUsed
	if summaryBudget < 32 {
		summaryBudget = 32
	}
	summary := buildRollingSummary(dropped, summaryBudget)

	kept := make([]Span, 0, n-removedSpans+1)
	inserted := false
	for i, sp := range t.spans {
		if !keepIdx[i] {
			if !inserted {
				kept = append(kept, Span{
					ID:      summarySpanID,
					Content: summary,
					Tokens:  EstimateTokens(summary),
					Dead:    false,
				})
				inserted = true
			}
			continue
		}
		if sp.ID == summarySpanID {
			// Old summary was selected into the raw tail; keep it only if we
			// did not already insert a fresh one in its drop slot.
			kept = append(kept, sp)
			continue
		}
		kept = append(kept, sp)
	}
	if !inserted {
		// All dropped spans were after the last kept - append summary before tail.
		out := make([]Span, 0, len(kept)+1)
		// Place summary after system/keep prefix.
		prefix := 0
		for prefix < len(kept) && keep[kept[prefix].ID] {
			prefix++
		}
		out = append(out, kept[:prefix]...)
		out = append(out, Span{
			ID:      summarySpanID,
			Content: summary,
			Tokens:  EstimateTokens(summary),
			Dead:    false,
		})
		out = append(out, kept[prefix:]...)
		kept = out
	}

	// If still over budget (summary larger than expected), trim summary text.
	surviving = 0
	for _, sp := range kept {
		surviving += sp.Tokens
	}
	if surviving > budget {
		over := surviving - budget
		for i := range kept {
			if kept[i].ID != summarySpanID {
				continue
			}
			kept[i].Content = truncateToTokenBudget(kept[i].Content, kept[i].Tokens-over)
			kept[i].Tokens = EstimateTokens(kept[i].Content)
			break
		}
		surviving = 0
		for _, sp := range kept {
			surviving += sp.Tokens
		}
	}

	t.spans = kept
	return removedTokens, removedSpans, surviving
}

// ContentContains reports whether any span body contains substr.
func (t *Tracker) ContentContains(substr string) bool {
	if substr == "" {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, sp := range t.spans {
		if strings.Contains(sp.Content, substr) {
			return true
		}
	}
	return false
}

func buildRollingSummary(dropped []Span, tokenBudget int) string {
	if tokenBudget < 16 {
		tokenBudget = 16
	}
	// Half the summary budget reserves early requirements so paper-style
	// summarization keeps goals/SPECs more often than blind FIFO drop.
	earlyBudget := tokenBudget / 2
	if earlyBudget < 48 {
		earlyBudget = 48
	}
	if earlyBudget > (tokenBudget*2)/3 {
		earlyBudget = (tokenBudget * 2) / 3
	}
	recentBudget := tokenBudget - earlyBudget
	if recentBudget < 16 {
		recentBudget = 16
		earlyBudget = tokenBudget - recentBudget
	}

	earlyNotes := extractKeyNotes(dropped, true, earlyBudget)
	recentNotes := extractKeyNotes(dropped, false, recentBudget)

	var b strings.Builder
	b.WriteString("[CONTEXT_SUMMARY] Codex-style rolling compact of earlier turns.\n")
	b.WriteString("Early requirements / facts (abbreviated, may be incomplete):\n")
	if earlyNotes == "" {
		b.WriteString("- (none extracted)\n")
	} else {
		b.WriteString(earlyNotes)
	}
	b.WriteString("Recent notes (lossy, newest preferred):\n")
	if recentNotes == "" {
		b.WriteString("- (none extracted)\n")
	} else {
		b.WriteString(recentNotes)
	}
	out := b.String()
	if EstimateTokens(out) <= tokenBudget {
		return out
	}
	return truncateToTokenBudget(out, tokenBudget)
}

// extractKeyNotes pulls [FACT]/[SPEC]/and short line previews from dropped spans.
// oldestFirst=true fills from the start (early needles); false fills newest-first.
func extractKeyNotes(dropped []Span, oldestFirst bool, tokenBudget int) string {
	if len(dropped) == 0 || tokenBudget <= 0 {
		return ""
	}
	var notes []string
	if oldestFirst {
		for _, sp := range dropped {
			notes = append(notes, keyLinesFromSpan(sp)...)
		}
	} else {
		for i := len(dropped) - 1; i >= 0; i-- {
			notes = append(notes, keyLinesFromSpan(dropped[i])...)
		}
	}
	var b strings.Builder
	used := 0
	for _, n := range notes {
		line := "- " + n + "\n"
		cost := EstimateTokens(line)
		if used+cost > tokenBudget {
			break
		}
		b.WriteString(line)
		used += cost
	}
	return b.String()
}

func keyLinesFromSpan(sp Span) []string {
	// Prefer answer fragments / facts first so a tight early quota does not
	// burn on long [SPEC] issue prose before the needle.
	var high, mid []string
	for _, line := range strings.Split(sp.Content, "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" {
			continue
		}
		if len(trim) > 240 {
			trim = trim[:240] + "…"
		}
		upper := strings.ToUpper(trim)
		switch {
		case strings.Contains(upper, "KEY ANSWER"),
			strings.Contains(upper, "[FACT]"),
			strings.Contains(upper, "AUTHORITATIVE"),
			strings.Contains(upper, "CANARY_"):
			high = append(high, trim)
		case strings.Contains(upper, "[SPEC]"):
			mid = append(mid, trim)
		}
	}
	out := append(high, mid...)
	if len(out) > 0 {
		return out
	}
	preview := strings.Join(strings.Fields(sp.Content), " ")
	if preview == "" {
		return nil
	}
	if len(preview) > 120 {
		preview = preview[:120] + "…"
	}
	return []string{preview}
}

func truncateToTokenBudget(s string, tokenBudget int) string {
	if tokenBudget <= 0 {
		return ""
	}
	// EstimateTokens ≈ chars/4; cut chars with a small margin.
	maxChars := tokenBudget * 4
	if maxChars < 32 {
		maxChars = 32
	}
	if len(s) <= maxChars {
		return s
	}
	return s[:maxChars] + "…"
}
