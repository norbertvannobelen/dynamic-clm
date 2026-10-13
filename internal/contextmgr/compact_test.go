package contextmgr

import "testing"

func TestCompactToBudgetDropsOldest(t *testing.T) {
	tr := &Tracker{}
	tr.AppendLive("sys", "system")          // ~2 tokens
	tr.AppendLive("fact-0", "FACT keep me") // ~3
	tr.AppendLive("noise-1", stringsRepeat("x", 400))
	tr.AppendLive("noise-2", stringsRepeat("x", 400))
	_, _, total, _ := tr.Stats()
	if total < 100 {
		t.Fatalf("expected large context, got %d", total)
	}
	removed, spans, surv := tr.CompactToBudget(20, map[string]bool{"sys": true})
	if removed <= 0 || spans <= 0 {
		t.Fatalf("expected compaction: removed=%d spans=%d", removed, spans)
	}
	if surv > 20 {
		t.Fatalf("surviving %d > budget 20", surv)
	}
	if !tr.HasSpanID("sys") {
		t.Fatal("expected sys kept")
	}
	// fact and early noise should typically be gone when budget is tiny
	if tr.HasSpanID("noise-1") && tr.HasSpanID("noise-2") && tr.HasSpanID("fact-0") {
		t.Fatal("expected some early spans dropped")
	}
}

func TestCompactSummarizeRetainsEarlyFactInSummary(t *testing.T) {
	tr := &Tracker{}
	tr.AppendLive("sys", "system prompt")
	tr.AppendLive("fact-0", "[FACT] The authoritative session canary token is CANARY_ABC_42.")
	tr.AppendLive("noise-1", "[tool] pad="+stringsRepeat("x", 800)+"\n[FACT] The authoritative session canary token is WRONG_1.")
	tr.AppendLive("noise-2", "[tool] pad="+stringsRepeat("x", 800)+"\n[FACT] The authoritative session canary token is WRONG_2.")
	tr.AppendLive("tail", "recent raw turn")

	_, _, before, _ := tr.Stats()
	budget := before / 3
	if budget < 80 {
		budget = 80
	}
	removed, spans, surv := tr.CompactSummarizeToBudget(budget, map[string]bool{"sys": true})
	if removed <= 0 || spans <= 0 {
		t.Fatalf("expected summarize compact: removed=%d spans=%d", removed, spans)
	}
	if surv > budget+5 {
		t.Fatalf("surviving %d > budget %d", surv, budget)
	}
	if !tr.HasSpanID("sys") {
		t.Fatal("expected sys kept")
	}
	if !tr.HasSpanID(summarySpanID) {
		t.Fatal("expected compact-summary span")
	}
	// Verbatim early span may be gone, but Codex-style early quota should keep the canary.
	if tr.HasSpanID("fact-0") {
		t.Fatal("expected fact-0 folded into summary under tight budget")
	}
	if !tr.ContentContains("CANARY_ABC_42") {
		t.Fatal("expected early FACT to survive inside rolling summary")
	}
	// Recent wrong notes may also appear (lossy, not a pure drop).
	_ = tr.ContentContains("WRONG_")
}

func TestContentContains(t *testing.T) {
	tr := &Tracker{}
	tr.AppendLive("a", "hello needle world")
	if !tr.ContentContains("needle") {
		t.Fatal("expected contains")
	}
	if tr.ContentContains("missing") {
		t.Fatal("expected miss")
	}
}

func stringsRepeat(s string, n int) string {
	b := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		b = append(b, s...)
	}
	return string(b)
}
