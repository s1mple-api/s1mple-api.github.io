package collector

import "testing"

func TestStripMarkdownLink(t *testing.T) {
	if got := stripMarkdownLink("[Falcons](https://example.com/team)"); got != "Falcons" {
		t.Fatalf("stripMarkdownLink returned %q", got)
	}
}

func TestEventDateRange(t *testing.T) {
	for _, input := range []string{"Jun 3 - Jul 7, 2026", "June 3 - July 7, 2026"} {
		if !eventDateRange.MatchString(input) {
			t.Errorf("eventDateRange did not match %q", input)
		}
	}
}
