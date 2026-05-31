package main

import (
	"os"
	"path/filepath"
	"testing"
)

// ids extracts the IDs of a slice of videos, in order, for compact assertions.
func ids(videos []ytVideo) []string {
	out := make([]string, len(videos))
	for i, v := range videos {
		out[i] = v.ID
	}
	return out
}

func TestLoadLiveVideos_WrapperFiltersToLiveAndDropsEmptyIDs(t *testing.T) {
	got, err := loadLiveVideos(filepath.Join("testdata", "videos_wrapper.json"))
	if err != nil {
		t.Fatalf("loadLiveVideos: %v", err)
	}
	// Only video_type=="live" with non-empty IDs survive: live001/002/003.
	// short001 and long001 are filtered out; the empty-ID live entry is dropped.
	want := map[string]bool{"live001": true, "live002": true, "live003": true}
	if len(got) != len(want) {
		t.Fatalf("expected %d live videos, got %d: %v", len(want), len(got), ids(got))
	}
	for _, v := range got {
		if !want[v.ID] {
			t.Errorf("unexpected video in result: %q (type=%q)", v.ID, v.VideoType)
		}
		if v.VideoType != "live" {
			t.Errorf("video %q has type %q, want live", v.ID, v.VideoType)
		}
		if v.ID == "" {
			t.Errorf("empty-ID video leaked into result")
		}
	}
}

func TestLoadLiveVideos_BareArray(t *testing.T) {
	got, err := loadLiveVideos(filepath.Join("testdata", "videos_bare.json"))
	if err != nil {
		t.Fatalf("loadLiveVideos (bare array): %v", err)
	}
	// bareshort is filtered; barelive1/2 survive.
	want := map[string]bool{"barelive1": true, "barelive2": true}
	if len(got) != len(want) {
		t.Fatalf("expected %d live videos from bare array, got %d: %v", len(want), len(got), ids(got))
	}
	for _, v := range got {
		if !want[v.ID] {
			t.Errorf("unexpected video from bare array: %q", v.ID)
		}
	}
}

func TestLoadLiveVideos_EmptyWrapperFallsBackCleanly(t *testing.T) {
	// {"videos":[]} parses as a valid wrapper with zero videos. The bare-array
	// fallback also yields nothing, so the result is simply empty — not an error.
	got, err := loadLiveVideos(filepath.Join("testdata", "videos_empty_wrapper.json"))
	if err != nil {
		t.Fatalf("loadLiveVideos (empty wrapper): %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 live videos from empty wrapper, got %d: %v", len(got), ids(got))
	}
}

func TestLoadLiveVideos_MissingFileErrors(t *testing.T) {
	_, err := loadLiveVideos(filepath.Join("testdata", "does_not_exist.json"))
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoadLiveVideos_InvalidJSONErrors(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "garbage.json")
	if err := os.WriteFile(tmp, []byte("this is not json"), 0644); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	_, err := loadLiveVideos(tmp)
	if err == nil {
		t.Fatal("expected parse error for invalid JSON, got nil")
	}
}

func TestSortAndLimitVideos_SortsDescByPublished(t *testing.T) {
	in := []ytVideo{
		{ID: "mid", PublishedAt: "2026-05-15T18:00:00Z"},
		{ID: "new", PublishedAt: "2026-05-30T18:00:00Z"},
		{ID: "old", PublishedAt: "2026-05-01T18:00:00Z"},
	}
	got := sortAndLimitVideos(in, 0) // 0 = keep all
	want := []string{"new", "mid", "old"}
	gotIDs := ids(got)
	for i := range want {
		if gotIDs[i] != want[i] {
			t.Fatalf("sort order = %v, want %v", gotIDs, want)
		}
	}
}

func TestSortAndLimitVideos_LimitKeepsMostRecentN(t *testing.T) {
	in := []ytVideo{
		{ID: "mid", PublishedAt: "2026-05-15T18:00:00Z"},
		{ID: "new", PublishedAt: "2026-05-30T18:00:00Z"},
		{ID: "old", PublishedAt: "2026-05-01T18:00:00Z"},
	}
	got := sortAndLimitVideos(in, 2)
	if len(got) != 2 {
		t.Fatalf("expected 2 videos after limit, got %d: %v", len(got), ids(got))
	}
	// Most recent two, in order.
	if got[0].ID != "new" || got[1].ID != "mid" {
		t.Fatalf("limited order = %v, want [new mid]", ids(got))
	}
}

func TestSortAndLimitVideos_LimitLargerThanInputKeepsAll(t *testing.T) {
	in := []ytVideo{
		{ID: "a", PublishedAt: "2026-05-02T00:00:00Z"},
		{ID: "b", PublishedAt: "2026-05-01T00:00:00Z"},
	}
	got := sortAndLimitVideos(in, 100)
	if len(got) != 2 {
		t.Fatalf("expected all 2 videos kept, got %d", len(got))
	}
}

// End-to-end of the loader + sort/limit, mirroring how fetchYouTubeTranscripts
// composes them: load live-only from the wrapper, then take the most-recent 2.
func TestLoadThenSortAndLimit_Composed(t *testing.T) {
	live, err := loadLiveVideos(filepath.Join("testdata", "videos_wrapper.json"))
	if err != nil {
		t.Fatalf("loadLiveVideos: %v", err)
	}
	got := sortAndLimitVideos(live, 2)
	if len(got) != 2 {
		t.Fatalf("expected 2 after limit, got %d: %v", len(got), ids(got))
	}
	// live002 (2026-05-30) newest, live003 (2026-05-15) next.
	if got[0].ID != "live002" || got[1].ID != "live003" {
		t.Fatalf("composed order = %v, want [live002 live003]", ids(got))
	}
}

func TestResolveRelative(t *testing.T) {
	abs := "/already/absolute/path.sh"
	if got := resolveRelative("/base/dir", abs); got != abs {
		t.Errorf("resolveRelative(absolute) = %q, want %q (passthrough)", got, abs)
	}
	got := resolveRelative("/base/dir", "scripts/gen.sh")
	want := filepath.Join("/base/dir", "scripts/gen.sh")
	if got != want {
		t.Errorf("resolveRelative(relative) = %q, want %q", got, want)
	}
}

func TestShortDate(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"rfc3339", "2026-05-30T18:00:00Z", "2026-05-30"},
		{"rfc3339-offset", "2026-05-30T11:00:00-07:00", "2026-05-30"},
		{"already-short", "2026-05-30", "2026-05-30"},
		{"junk-long-enough", "not-a-date-but-long", "not-a-date"}, // first 10 chars
		{"junk-too-short", "short", "short"},                      // passthrough
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := shortDate(c.in); got != c.want {
				t.Errorf("shortDate(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestFirstLine(t *testing.T) {
	if got := firstLine("hello\nworld"); got != "hello" {
		t.Errorf("firstLine multi = %q, want %q", got, "hello")
	}
	if got := firstLine("no newline"); got != "no newline" {
		t.Errorf("firstLine single = %q, want %q", got, "no newline")
	}
	if got := firstLine(""); got != "" {
		t.Errorf("firstLine empty = %q, want empty", got)
	}
}
