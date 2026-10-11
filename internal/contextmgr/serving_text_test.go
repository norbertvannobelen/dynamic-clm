package contextmgr

import (
	"strings"
	"testing"
)

func TestContextTextForServingOmitsDeadBodies(t *testing.T) {
	tr := &Tracker{}
	tr.AppendLive("sys", "system")
	tr.AppendLive("spec", "CORRECT_FRAGMENT_AAA")
	pad := strings.Repeat("x", 200)
	tr.AppendLive("tool-1", "WRONG_FRAGMENT_BBB pad="+pad)
	tr.ApplyMarkers(ParseMarkers(FormatIgnore("tool-1")), 1)

	legacy := tr.ContextText(true)
	if !strings.Contains(legacy, "WRONG_FRAGMENT_BBB") {
		t.Fatalf("legacy ContextText should still include dead body")
	}

	served := tr.ContextTextForServing(true)
	if strings.Contains(served, "WRONG_FRAGMENT_BBB") {
		t.Fatalf("serving text must not include dead body: %q", served)
	}
	if !strings.Contains(served, "CORRECT_FRAGMENT_AAA") {
		t.Fatalf("serving text must keep live SPEC")
	}
	stub := FormatIgnore("tool-1")
	if !strings.Contains(served, stub) {
		t.Fatalf("serving text should stub ignore marker %q, got %q", stub, served)
	}
	if len(served) >= len(legacy) {
		t.Fatalf("serving text should be shorter than legacy: serve=%d legacy=%d", len(served), len(legacy))
	}

	baseline := tr.ContextTextForServing(false)
	if strings.Contains(baseline, stub) || strings.Contains(baseline, "WRONG_FRAGMENT_BBB") {
		t.Fatalf("baseline serving should omit dead entirely: %q", baseline)
	}
}
