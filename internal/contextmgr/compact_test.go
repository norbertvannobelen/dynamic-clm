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

func stringsRepeat(s string, n int) string {
	b := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		b = append(b, s...)
	}
	return string(b)
}
