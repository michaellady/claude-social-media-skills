package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestLoadVoiceFreshness_CountsNewsletterVsYouTube(t *testing.T) {
	t.Setenv("DASH_VOICE_CACHE", fixture(t, "voice-cache.json"))
	vf := loadVoiceFreshness()
	if !vf.Available {
		t.Fatal("voice cache present → Available should be true")
	}
	// Fixture: 2 newsletter + 1 untyped ("") → 3 newsletter; 1 youtube_live + 1
	// youtube_transcript → 2 youtube.
	if vf.NumNewsletter != 3 {
		t.Errorf("NumNewsletter = %d, want 3", vf.NumNewsletter)
	}
	if vf.NumYouTube != 2 {
		t.Errorf("NumYouTube = %d, want 2", vf.NumYouTube)
	}
	if vf.NewsletterFetchedAt != "2026-05-29T08:00:00Z" {
		t.Errorf("NewsletterFetchedAt = %q", vf.NewsletterFetchedAt)
	}
	if vf.YouTubeTranscriptFetchedAt != "2026-05-29T08:01:12Z" {
		t.Errorf("YouTubeTranscriptFetchedAt = %q", vf.YouTubeTranscriptFetchedAt)
	}
}

func TestLoadVoiceFreshness_MissingCache(t *testing.T) {
	t.Setenv("DASH_VOICE_CACHE", fixture(t, "no-such-voice-cache.json"))
	vf := loadVoiceFreshness()
	if vf.Available {
		t.Error("missing cache → Available should be false")
	}
	if vf.NumNewsletter != 0 || vf.NumYouTube != 0 {
		t.Errorf("missing cache → counts should be 0, got %d/%d", vf.NumNewsletter, vf.NumYouTube)
	}
}

func TestNewestMatch_LexicalNewest(t *testing.T) {
	// ledger-yt has 2026-05-18.md and 2026-05-25.md; newest lexical = 05-25.
	got := newestMatch(fixture(t, "ledger-yt"), ".md")
	want := filepath.Join(fixture(t, "ledger-yt"), "2026-05-25.md")
	if got != want {
		t.Errorf("newestMatch = %q, want %q", got, want)
	}
}

func TestNewestMatch_NoMatchAndMissingDir(t *testing.T) {
	// Right dir, wrong suffix → empty.
	if got := newestMatch(fixture(t, "ledger-yt"), ".json"); got != "" {
		t.Errorf("no .json in ledger-yt should be empty, got %q", got)
	}
	// Missing dir → empty.
	if got := newestMatch(fixture(t, "no-such-dir"), ".json"); got != "" {
		t.Errorf("missing dir should be empty, got %q", got)
	}
}

func TestSnapshotAgeDays(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		date string
		want int
	}{
		{"2026-05-30", 0},
		{"2026-05-23", 7},
		{"2026-05-16", 14},
		{"not-a-date", -1},
		{"snapshot-full", -1}, // fixture filename, not a date
	}
	for _, c := range cases {
		if got := snapshotAgeDays(c.date, now); got != c.want {
			t.Errorf("snapshotAgeDays(%q) = %d, want %d", c.date, got, c.want)
		}
	}
}
