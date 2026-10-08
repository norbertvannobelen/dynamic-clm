package contextmgr

import (
	"regexp"
	"strings"
)

// Markers are structurally distinct from live content (handoff §8.3).
const (
	IgnoreOpen  = "<<<DYNCLM_IGNORE"
	IgnoreClose = ">>>"
	UnmarkOpen  = "<<<DYNCLM_UNMARK"
	UnmarkClose = ">>>"
)

var (
	ignoreRe = regexp.MustCompile(`(?s)<<<DYNCLM_IGNORE\s+([^>]+?)>>>`)
	unmarkRe = regexp.MustCompile(`(?s)<<<DYNCLM_UNMARK\s+([^>]+?)>>>`)
)

// Marker is a parsed soft-invalidation or revocation directive.
type Marker struct {
	Kind    string // "ignore" or "unmark"
	Target  string
	Raw     string
	Start   int // byte offset in the scanned text
	End     int
	Revoked bool
}

// ParseMarkers extracts ignore/unmark directives from text appended by the model.
func ParseMarkers(text string) []Marker {
	var out []Marker
	for _, m := range ignoreRe.FindAllStringSubmatchIndex(text, -1) {
		raw := text[m[0]:m[1]]
		target := strings.TrimSpace(text[m[2]:m[3]])
		out = append(out, Marker{
			Kind:   "ignore",
			Target: target,
			Raw:    raw,
			Start:  m[0],
			End:    m[1],
		})
	}
	for _, m := range unmarkRe.FindAllStringSubmatchIndex(text, -1) {
		raw := text[m[0]:m[1]]
		target := strings.TrimSpace(text[m[2]:m[3]])
		out = append(out, Marker{
			Kind:   "unmark",
			Target: target,
			Raw:    raw,
			Start:  m[0],
			End:    m[1],
		})
	}
	return out
}

// FormatIgnore builds a short, distinct invalidation marker.
func FormatIgnore(target string) string {
	return IgnoreOpen + " " + strings.TrimSpace(target) + " " + IgnoreClose
}

// FormatUnmark builds a revocation marker.
func FormatUnmark(target string) string {
	return UnmarkOpen + " " + strings.TrimSpace(target) + " " + UnmarkClose
}

// EstimateTokens is a cheap whitespace/char heuristic (~4 chars/token).
func EstimateTokens(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	n := (len(s) + 3) / 4
	if n < 1 {
		return 1
	}
	return n
}
