package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestLoadPeerChannels_YAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "peer-channels.yaml")
	yaml := `- handle: "@fireship"
  angle: "Hyper-tight tech news"
- handle: "@t3dotgg"
  angle: "Opinion-led with strong POV"
`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := loadPeerChannels(path)
	if err != nil {
		t.Fatalf("loadPeerChannels: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 channels, got %d", len(got))
	}
	if got[0].Handle != "@fireship" || got[0].Angle == "" {
		t.Errorf("first channel parsed wrong: %+v", got[0])
	}
}

func TestLoadPeerChannels_Missing(t *testing.T) {
	got, err := loadPeerChannels(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatalf("missing file should not error, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty, got %d", len(got))
	}
}

func TestPeerVideosRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "peer-videos.json")
	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	in := &PeerVideosFile{
		FetchedAt: now,
		Channels: map[string]PeerChannelVideos{
			"@fireship": {
				ChannelID:    "UCsBjURrPoezykLs9EqgamOA",
				ChannelTitle: "Fireship",
				Videos: []PeerVideo{
					{ID: "abc", Title: "Foo", PublishedAt: now.Add(-7 * 24 * time.Hour),
						ViewCount: 100000, DurationSeconds: 95, VideoType: "long-form"},
				},
			},
		},
	}
	if err := savePeerVideos(path, in); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := loadPeerVideos(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !reflect.DeepEqual(got.Channels["@fireship"].Videos[0].ID, "abc") {
		t.Errorf("roundtrip lost data: %+v", got)
	}
	if got.Channels["@fireship"].Videos[0].ViewCount != 100000 {
		t.Errorf("view_count roundtrip wrong: %+v", got.Channels["@fireship"].Videos[0])
	}
}

func TestLoadPeerVideos_Missing(t *testing.T) {
	got, err := loadPeerVideos(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("missing file should not error, got %v", err)
	}
	if got.Channels == nil {
		t.Errorf("expected initialized empty map, got nil")
	}
}

func TestSelectTopRecent_FiltersByWindow(t *testing.T) {
	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	videos := []PeerVideo{
		{ID: "old", PublishedAt: now.Add(-200 * 24 * time.Hour), ViewCount: 1000000}, // outside 90d
		{ID: "new1", PublishedAt: now.Add(-10 * 24 * time.Hour), ViewCount: 500},
		{ID: "new2", PublishedAt: now.Add(-30 * 24 * time.Hour), ViewCount: 50000},
	}
	got := selectTopRecent(videos, now, 90*24*time.Hour, 10)
	if len(got) != 2 {
		t.Fatalf("expected 2 within window, got %d", len(got))
	}
	for _, v := range got {
		if v.ID == "old" {
			t.Errorf("old video leaked through window filter: %+v", v)
		}
	}
}

func TestSelectTopRecent_RanksByViewsDesc(t *testing.T) {
	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	videos := []PeerVideo{
		{ID: "a", PublishedAt: now.Add(-1 * 24 * time.Hour), ViewCount: 100},
		{ID: "b", PublishedAt: now.Add(-1 * 24 * time.Hour), ViewCount: 5000},
		{ID: "c", PublishedAt: now.Add(-1 * 24 * time.Hour), ViewCount: 1000},
	}
	got := selectTopRecent(videos, now, 90*24*time.Hour, 10)
	if len(got) != 3 {
		t.Fatalf("expected all 3, got %d", len(got))
	}
	if got[0].ID != "b" || got[1].ID != "c" || got[2].ID != "a" {
		t.Errorf("not sorted desc by views: %+v", got)
	}
}

func TestSelectTopRecent_CapsAtN(t *testing.T) {
	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	videos := make([]PeerVideo, 20)
	for i := range videos {
		videos[i] = PeerVideo{ID: string(rune('a' + i)),
			PublishedAt: now.Add(-1 * 24 * time.Hour),
			ViewCount:   int64(i * 100)}
	}
	got := selectTopRecent(videos, now, 90*24*time.Hour, 5)
	if len(got) != 5 {
		t.Errorf("expected top-5 cap, got %d", len(got))
	}
}

func TestSelectTopRecent_EmptyInput(t *testing.T) {
	got := selectTopRecent(nil, time.Now(), 90*24*time.Hour, 10)
	if len(got) != 0 {
		t.Errorf("expected empty, got %d", len(got))
	}
}

func TestNormalizeHandle(t *testing.T) {
	cases := map[string]string{
		"@fireship":                       "@fireship",
		"fireship":                        "@fireship",
		"https://www.youtube.com/@fireship": "@fireship",
		"youtube.com/@t3dotgg":            "@t3dotgg",
		"":                                "",
	}
	for in, want := range cases {
		got := normalizeHandle(in)
		if got != want {
			t.Errorf("normalizeHandle(%q) = %q, want %q", in, got, want)
		}
	}
}
