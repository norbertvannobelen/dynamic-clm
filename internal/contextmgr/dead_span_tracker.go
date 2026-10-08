package contextmgr

import (
	"strings"
	"sync"
)

// Span is a tracked region of context that may be live or dead.
type Span struct {
	ID        string
	Content   string
	Tokens    int
	Dead      bool
	MarkedAt  int // turn index when marked dead
	RevokedAt int // turn index when unmarked; 0 if never
}

// Tracker maintains live/dead token estimates over an append-only context.
type Tracker struct {
	mu       sync.Mutex
	spans    []Span
	nextID   int
	markers  int
	parsedOK int
	parseFail int
}

// AppendLive records a new live span (tool output, user/assistant text).
func (t *Tracker) AppendLive(id, content string) Span {
	t.mu.Lock()
	defer t.mu.Unlock()
	if id == "" {
		t.nextID++
		id = formatID(t.nextID)
	}
	sp := Span{ID: id, Content: content, Tokens: EstimateTokens(content), Dead: false}
	t.spans = append(t.spans, sp)
	return sp
}

// ApplyMarkers applies ignore/unmark directives against span IDs or content substrings.
func (t *Tracker) ApplyMarkers(markers []Marker, turn int) (applied, failed int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, m := range markers {
		t.markers++
		ok := false
		switch m.Kind {
		case "ignore":
			ok = t.markDeadLocked(m.Target, turn)
		case "unmark":
			ok = t.unmarkLocked(m.Target, turn)
		}
		if ok {
			applied++
			t.parsedOK++
		} else {
			failed++
			t.parseFail++
		}
	}
	return applied, failed
}

func (t *Tracker) markDeadLocked(target string, turn int) bool {
	target = strings.TrimSpace(target)
	for i := range t.spans {
		if t.spans[i].ID == target || strings.Contains(t.spans[i].Content, target) {
			if !t.spans[i].Dead {
				t.spans[i].Dead = true
				t.spans[i].MarkedAt = turn
			}
			return true
		}
	}
	return false
}

func (t *Tracker) unmarkLocked(target string, turn int) bool {
	target = strings.TrimSpace(target)
	for i := range t.spans {
		if t.spans[i].ID == target || strings.Contains(t.spans[i].Content, target) {
			if t.spans[i].Dead {
				t.spans[i].Dead = false
				t.spans[i].RevokedAt = turn
			}
			return true
		}
	}
	return false
}

// Stats returns aggregate token counts and parse rates.
func (t *Tracker) Stats() (live, dead, total int, parseRate float64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, sp := range t.spans {
		total += sp.Tokens
		if sp.Dead {
			dead += sp.Tokens
		} else {
			live += sp.Tokens
		}
	}
	attempts := t.parsedOK + t.parseFail
	if attempts == 0 {
		parseRate = 1.0
	} else {
		parseRate = float64(t.parsedOK) / float64(attempts)
	}
	return live, dead, total, parseRate
}

// DeadSpans returns a copy of currently dead spans.
func (t *Tracker) DeadSpans() []Span {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []Span
	for _, sp := range t.spans {
		if sp.Dead {
			out = append(out, sp)
		}
	}
	return out
}

// LiveSpans returns a copy of currently live spans.
func (t *Tracker) LiveSpans() []Span {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []Span
	for _, sp := range t.spans {
		if !sp.Dead {
			out = append(out, sp)
		}
	}
	return out
}

// RemoveDead drops dead spans (used after a physical rewrite).
func (t *Tracker) RemoveDead() (removedTokens int, removed int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	kept := t.spans[:0]
	for _, sp := range t.spans {
		if sp.Dead {
			removedTokens += sp.Tokens
			removed++
			continue
		}
		kept = append(kept, sp)
	}
	t.spans = kept
	return removedTokens, removed
}

// ContextText reconstructs the logical context (live + dead for soft-invalidation phase).
func (t *Tracker) ContextText(includeDead bool) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	var b strings.Builder
	for _, sp := range t.spans {
		if sp.Dead && !includeDead {
			continue
		}
		if sp.Dead {
			b.WriteString(FormatIgnore(sp.ID))
			b.WriteByte('\n')
		}
		b.WriteString(sp.Content)
		b.WriteByte('\n')
	}
	return b.String()
}

// CloneSpans returns a snapshot of all spans.
func (t *Tracker) CloneSpans() []Span {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Span, len(t.spans))
	copy(out, t.spans)
	return out
}

func formatID(n int) string {
	return "span-" + itoa(n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
