package main

import "testing"

func TestIsWedged(t *testing.T) {
	cases := []struct {
		out  string
		want bool
	}{
		{"No active page. Use \"browse goto <url>\" first.", true},
		{"no active page", true},
		{"[browse] Starting server...\nNavigated to https://example.com (200)", false},
		{"Navigated to https://publish.buffer.com/insights (200)", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isWedged(c.out); got != c.want {
			t.Errorf("isWedged(%q) = %v, want %v", c.out, got, c.want)
		}
	}
}

// The kill patterns must be scoped to the gstack profile — a pattern matching
// plain "Google Chrome" would risk killing the user's real browser.
func TestKillPatterns_GstackScopedOnly(t *testing.T) {
	for _, p := range killPatterns {
		if p == "Google Chrome" || p == "chrome" || p == "Chrome" {
			t.Errorf("kill pattern %q is too broad — would risk the user's real Chrome", p)
		}
	}
}
