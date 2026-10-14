package contextmgr

// CacheBackend abstracts KV-cache control for SCR-style rewrites.
// oMLX does not currently expose span-level KV relocation; LogicalCacheBackend
// accounts re-prefill tokens without mutating server-side KV arrays.
type CacheBackend interface {
	// RelocateSpans relocates up to K surviving spans' KV states after an edit.
	// Returns tokens that still required prefill and wall-clock overhead ms.
	RelocateSpans(surviving []Span, newTokens int, k int) (prefillTokens int, overheadMS float64, err error)
	// SupportsSCR reports whether true span relocation is available.
	SupportsSCR() bool
}

// LogicalCacheBackend estimates SCR savings without server KV control.
// Naive full-suffix re-prefill would cost survivingTokens + newTokens.
// With K-cap SCR approximation, we charge only newTokens + spilled spans.
type LogicalCacheBackend struct {
	K int
}

// SupportsSCR is false - this is accounting-only.
func (l *LogicalCacheBackend) SupportsSCR() bool { return false }

// RelocateSpans estimates prefill under an SCR-like K-cap policy.
func (l *LogicalCacheBackend) RelocateSpans(surviving []Span, newTokens int, k int) (int, float64, error) {
	if k <= 0 {
		k = l.K
	}
	if k <= 0 {
		k = 6
	}
	// Sort by size descending; relocate up to K largest; spill the rest to prefill.
	sorted := make([]Span, len(surviving))
	copy(sorted, surviving)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].Tokens > sorted[i].Tokens {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	prefill := newTokens
	for i, sp := range sorted {
		if i < k {
			continue // relocated, no prefill
		}
		prefill += sp.Tokens
	}
	// Fixed overhead proxy for KV relocation / RoPE / bookkeeping (ms).
	overhead := 5.0 + float64(min(k, len(surviving)))*0.5
	return prefill, overhead, nil
}

// NaivePrefillTokens is the full-suffix re-prefill cost without SCR.
func NaivePrefillTokens(surviving []Span, newTokens int) int {
	n := newTokens
	for _, sp := range surviving {
		n += sp.Tokens
	}
	return n
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
