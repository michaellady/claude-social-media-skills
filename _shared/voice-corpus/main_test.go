package main

import (
	"testing"
	"time"
)

func TestIsStale(t *testing.T) {
	now := time.Now().UTC()

	// Zero time is always stale.
	if !isStale(time.Time{}, 7) {
		t.Error("isStale(zero, 7) = false, want true (zero time is always stale)")
	}

	// Just-now is fresh.
	if isStale(now, 7) {
		t.Error("isStale(now, 7) = true, want false (just fetched)")
	}

	// Clearly older than the window is stale.
	if !isStale(now.Add(-8*24*time.Hour), 7) {
		t.Error("isStale(8d ago, 7) = false, want true")
	}

	// Clearly inside the window is fresh.
	if isStale(now.Add(-6*24*time.Hour), 7) {
		t.Error("isStale(6d ago, 7) = true, want false")
	}

	// Boundary: staleness uses strict greater-than, so exactly-at-window is NOT
	// stale; a hair past it IS. Probe both sides of the 7-day edge.
	justInside := now.Add(-7*24*time.Hour + time.Minute) // ~6d23h59m old
	if isStale(justInside, 7) {
		t.Error("isStale(just inside 7d window) = true, want false")
	}
	justOutside := now.Add(-7*24*time.Hour - time.Minute) // ~7d00h01m old
	if !isStale(justOutside, 7) {
		t.Error("isStale(just outside 7d window) = false, want true")
	}
}

func TestEffectiveSource(t *testing.T) {
	cases := []struct {
		name string
		in   post
		want string
	}{
		{"empty defaults to newsletter", post{SourceType: ""}, "newsletter"},
		{"explicit newsletter", post{SourceType: "newsletter"}, "newsletter"},
		{"youtube_live preserved", post{SourceType: "youtube_live"}, "youtube_live"},
		{"unknown value passed through", post{SourceType: "podcast"}, "podcast"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := effectiveSource(c.in); got != c.want {
				t.Errorf("effectiveSource(%q) = %q, want %q", c.in.SourceType, got, c.want)
			}
		})
	}
}

func TestFilterBySource(t *testing.T) {
	posts := []post{
		{Title: "explicit newsletter", SourceType: "newsletter"},
		{Title: "legacy untagged", SourceType: ""}, // counts as newsletter
		{Title: "livestream", SourceType: "youtube_live"},
		{Title: "another live", SourceType: "youtube_live"},
	}

	news := filterBySource(posts, "newsletter")
	if len(news) != 2 {
		t.Fatalf("newsletter filter returned %d, want 2 (explicit + legacy untagged)", len(news))
	}
	for _, p := range news {
		if effectiveSource(p) != "newsletter" {
			t.Errorf("newsletter slice contains non-newsletter: %q (%q)", p.Title, p.SourceType)
		}
	}

	yt := filterBySource(posts, "youtube_live")
	if len(yt) != 2 {
		t.Fatalf("youtube_live filter returned %d, want 2", len(yt))
	}
	for _, p := range yt {
		if p.SourceType != "youtube_live" {
			t.Errorf("youtube_live slice contains wrong type: %q (%q)", p.Title, p.SourceType)
		}
	}

	// No matches yields an empty (non-nil) slice.
	none := filterBySource(posts, "podcast")
	if len(none) != 0 {
		t.Errorf("podcast filter returned %d, want 0", len(none))
	}

	// Empty input is handled.
	if got := filterBySource(nil, "newsletter"); len(got) != 0 {
		t.Errorf("filterBySource(nil) returned %d, want 0", len(got))
	}
}

func TestNormalizeDate(t *testing.T) {
	// RSS pubDate (RFC1123Z) -> YYYY-MM-DD.
	if got := normalizeDate("Fri, 30 May 2026 18:00:00 +0000"); got != "2026-05-30" {
		t.Errorf("normalizeDate(RFC1123Z) = %q, want 2026-05-30", got)
	}
	// Unparseable input passes through trimmed.
	if got := normalizeDate("  not a date  "); got != "not a date" {
		t.Errorf("normalizeDate(junk) = %q, want %q", got, "not a date")
	}
}
