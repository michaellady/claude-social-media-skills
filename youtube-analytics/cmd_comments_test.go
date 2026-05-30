package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestLoadComments_Missing(t *testing.T) {
	out, err := loadComments(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("missing file should not error, got %v", err)
	}
	if out.Videos == nil {
		t.Errorf("expected initialized empty map, got nil")
	}
	if len(out.Videos) != 0 {
		t.Errorf("expected empty videos map, got %d", len(out.Videos))
	}
}

func TestCommentsRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "comments.json")
	in := &CommentsFile{
		FetchedAt: time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC),
		Videos: map[string]VideoComments{
			"abc": {
				VideoID:   "abc",
				FetchedAt: time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC),
				Comments: []Comment{
					{ID: "c1", AuthorName: "Alice", Text: "great video", LikeCount: 5, ReplyCount: 1,
						PublishedAt: time.Date(2026, 4, 23, 0, 0, 0, 0, time.UTC)},
					{ID: "c2", AuthorName: "Bob", Text: "couldn't follow", LikeCount: 0,
						PublishedAt: time.Date(2026, 4, 24, 0, 0, 0, 0, time.UTC)},
				},
			},
		},
	}
	if err := saveComments(path, in); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := loadComments(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got.Videos["abc"].Comments) != 2 {
		t.Fatalf("comments roundtrip lost data: %+v", got.Videos["abc"])
	}
	if got.Videos["abc"].Comments[0].LikeCount != 5 {
		t.Errorf("first comment like_count wrong: %+v", got.Videos["abc"].Comments[0])
	}
}

func TestSaveComments_PreservesExistingVideos(t *testing.T) {
	// Re-running fetch-comments on a subset should not erase entries for
	// videos not in the current run. Verifies the merge contract used by
	// cmdFetchComments (which loads, mutates only the keys it fetches, saves).
	dir := t.TempDir()
	path := filepath.Join(dir, "comments.json")

	first := &CommentsFile{
		Videos: map[string]VideoComments{
			"a": {VideoID: "a", Comments: []Comment{{ID: "x", Text: "first"}}},
			"b": {VideoID: "b", Comments: []Comment{{ID: "y", Text: "first"}}},
		},
	}
	if err := saveComments(path, first); err != nil {
		t.Fatal(err)
	}

	loaded, err := loadComments(path)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate: a partial re-run only refreshed video "a".
	loaded.Videos["a"] = VideoComments{VideoID: "a", Comments: []Comment{{ID: "x2", Text: "second"}}}
	if err := saveComments(path, loaded); err != nil {
		t.Fatal(err)
	}

	final, err := loadComments(path)
	if err != nil {
		t.Fatal(err)
	}
	if final.Videos["a"].Comments[0].ID != "x2" {
		t.Errorf("video a not refreshed: %+v", final.Videos["a"])
	}
	if final.Videos["b"].Comments[0].ID != "y" {
		t.Errorf("video b dropped during partial re-run: %+v", final.Videos["b"])
	}
}
