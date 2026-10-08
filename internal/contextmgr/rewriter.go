package contextmgr

import (
	"fmt"
	"time"
)

const DefaultSpanCapK = 6

// RewriteResult captures accounting for one batched physical rewrite.
type RewriteResult struct {
	RemovedTokens       int
	RemovedSpans        int
	SurvivingTokens     int
	PrefillTokens       int // actual (SCR-estimated) prefill
	NaivePrefillTokens  int // counterfactual full-suffix
	PrefillSavedTokens  int
	OverheadMS          float64
	WallClockMS         float64
	PostRewriteContext  int
	UsedSCRApproximation bool
}

// Rewriter performs batched physical rewrites: drop dead spans, account prefill.
type Rewriter struct {
	Backend CacheBackend
	K       int
}

// NewRewriter constructs a rewriter with the given backend (defaults to logical).
func NewRewriter(backend CacheBackend) *Rewriter {
	if backend == nil {
		backend = &LogicalCacheBackend{K: DefaultSpanCapK}
	}
	k := DefaultSpanCapK
	if lb, ok := backend.(*LogicalCacheBackend); ok && lb.K > 0 {
		k = lb.K
	}
	return &Rewriter{Backend: backend, K: k}
}

// Rewrite removes dead spans and estimates re-prefill under SCR-style relocation.
func (r *Rewriter) Rewrite(t *Tracker, newlyInsertedTokens int) (RewriteResult, error) {
	start := time.Now()
	live := t.LiveSpans()
	dead := t.DeadSpans()
	if len(dead) == 0 {
		return RewriteResult{}, fmt.Errorf("no dead spans to rewrite")
	}

	survivingTokens := 0
	for _, sp := range live {
		survivingTokens += sp.Tokens
	}
	naive := NaivePrefillTokens(live, newlyInsertedTokens)
	prefill, overhead, err := r.Backend.RelocateSpans(live, newlyInsertedTokens, r.K)
	if err != nil {
		return RewriteResult{}, err
	}
	removedTokens, removed := t.RemoveDead()
	_, _, post, _ := t.Stats()

	res := RewriteResult{
		RemovedTokens:        removedTokens,
		RemovedSpans:         removed,
		SurvivingTokens:      survivingTokens,
		PrefillTokens:        prefill,
		NaivePrefillTokens:   naive,
		PrefillSavedTokens:   naive - prefill,
		OverheadMS:           overhead,
		WallClockMS:          float64(time.Since(start).Milliseconds()) + overhead,
		PostRewriteContext:   post,
		UsedSCRApproximation: !r.Backend.SupportsSCR(),
	}
	return res, nil
}
