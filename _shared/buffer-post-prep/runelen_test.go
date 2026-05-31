package main

import "testing"

// Char limits are in characters, not bytes. The EVC voice uses em-dashes and
// emoji heavily, so byte-counting (the old len()) over-counts and would falsely
// reject posts that are actually within a platform's character limit.
func TestRuneLen_CountsCharsNotBytes(t *testing.T) {
	cases := []struct {
		name string
		text string
		want int
	}{
		{"ascii", "hello", 5},
		{"em-dash is 1 char (3 bytes)", "a—b", 3},
		{"emoji is 1 char (4 bytes)", "scale 📈", 7},
		{"newsletter CTA emoji", `Comment "newsletter" 📬`, 22},
	}
	for _, c := range cases {
		if got := runeLen(c.text); got != c.want {
			t.Errorf("runeLen(%q) = %d, want %d (len()=%d bytes)", c.text, got, c.want, len(c.text))
		}
	}
}

// Regression: a Twitter post built entirely from em-dashes that is exactly at
// the 280-char limit must PASS — byte-counting would see 840 bytes and reject.
func TestRuneLen_TwitterEmDashAtLimit(t *testing.T) {
	text := ""
	for i := 0; i < 280; i++ {
		text += "—" // 3 bytes each → 840 bytes, 280 runes
	}
	if got := runeLen(text); got != 280 {
		t.Fatalf("runeLen = %d, want 280", got)
	}
	if len(text) <= 280 {
		t.Fatalf("test setup wrong: byte len %d should exceed 280 to prove the bug", len(text))
	}
	// With the fix, 280 runes == limit 280 → not over-limit (would have been
	// rejected as 840 > 280 before).
	if runeLen(text) > 280 {
		t.Error("280 em-dashes should be within the 280-char Twitter limit")
	}
}
