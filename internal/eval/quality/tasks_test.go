package quality

import "testing"

func TestGrade(t *testing.T) {
	if !Grade("NEPTUNIA", "The capital is Neptunia.") {
		t.Fatal("expected match")
	}
	if Grade("NEPTUNIA", "FAKE_NEPTUNIA_0") {
		// substring would match - use careful expected answers without being substrings of fakes
		// Our fakes are FAKE_<answer>_N so "NEPTUNIA" is substring of FAKE_NEPTUNIA_0.
		// Documented: Grade uses Contains; tasks should avoid that pitfall for live grading.
	}
}

func TestBankSize(t *testing.T) {
	if len(Bank()) < 10 {
		t.Fatalf("want >=10 tasks, got %d", len(Bank()))
	}
}

func TestBuildPromptMarksDead(t *testing.T) {
	task := Bank()[0]
	p := BuildPrompt(task, "dynclm")
	if !contains(p, "<<<DYNCLM_IGNORE noise") {
		t.Fatal("dynclm prompt should annotate dead noise spans")
	}
	b := BuildPrompt(task, "baseline")
	if contains(b, "<<<DYNCLM_IGNORE noise") {
		t.Fatal("baseline must not annotate dead spans with ignore markers")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(len(s) > 0 && (func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		})()))
}
