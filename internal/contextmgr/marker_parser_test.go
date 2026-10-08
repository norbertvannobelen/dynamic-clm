package contextmgr

import "testing"

func TestParseIgnoreAndUnmark(t *testing.T) {
	text := FormatIgnore("span-3") + "\nfoo\n" + FormatUnmark("span-3")
	ms := ParseMarkers(text)
	if len(ms) != 2 {
		t.Fatalf("got %d markers", len(ms))
	}
	if ms[0].Kind != "ignore" || ms[0].Target != "span-3" {
		t.Fatalf("ignore: %+v", ms[0])
	}
	if ms[1].Kind != "unmark" {
		t.Fatalf("unmark: %+v", ms[1])
	}
}

func TestTrackerDeadAndRewrite(t *testing.T) {
	tr := &Tracker{}
	tr.AppendLive("span-1", "hello world this is content")
	tr.AppendLive("span-2", "more live content here")
	tr.ApplyMarkers(ParseMarkers(FormatIgnore("span-1")), 1)
	live, dead, _, rate := tr.Stats()
	if dead == 0 || live == 0 {
		t.Fatalf("live=%d dead=%d", live, dead)
	}
	if rate != 1 {
		t.Fatalf("parse rate %v", rate)
	}
	rw := NewRewriter(nil)
	res, err := rw.Rewrite(tr, 5)
	if err != nil {
		t.Fatal(err)
	}
	if res.RemovedTokens == 0 {
		t.Fatal("expected removed tokens")
	}
	if res.PrefillTokens > res.NaivePrefillTokens {
		t.Fatalf("SCR estimate should not exceed naive: %d > %d", res.PrefillTokens, res.NaivePrefillTokens)
	}
}
