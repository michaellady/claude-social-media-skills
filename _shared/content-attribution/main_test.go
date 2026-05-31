package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeSnap drops a snapshot-<date>.json into dir for the snapshotPostsMatch tests.
func writeSnap(t *testing.T, dir string, doc map[string]any) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "snapshot-2026-05-30.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TikTok: match by source_tag, whole engagement object + join_method/post_id come through.
func TestTiktokMatch_BySourceTag(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CA_TIKTOK_CACHE_DIR", dir)
	writeSnap(t, dir, map[string]any{
		"platform": "tiktok",
		"recent_posts": []any{
			map[string]any{
				"caption":    "great clip [opus:ABC123]",
				"source_tag": map[string]any{"scheme": "opus", "id": "ABC123"},
				"post_id":    "777",
				"engagement": map[string]any{"views": 539, "likes": 2, "comments": 0, "shares": nil},
			},
		},
	})
	rec := tiktokMatch("ABC123")
	if rec.Pending || rec.Engagement == nil {
		t.Fatalf("expected match, got %+v", rec)
	}
	if rec.Engagement["join_method"] != "tag" {
		t.Errorf("join_method = %v, want tag", rec.Engagement["join_method"])
	}
	if rec.Engagement["post_id"] != "777" {
		t.Errorf("post_id = %v, want 777", rec.Engagement["post_id"])
	}
	if v, _ := rec.Engagement["views"].(float64); v != 539 {
		t.Errorf("views = %v, want 539", rec.Engagement["views"])
	}
}

// Threads: no source_tag → must match on the raw [opus:] substring in caption.
func TestThreadsMatch_CaptionFallback(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CA_THREADS_CACHE_DIR", dir)
	writeSnap(t, dir, map[string]any{
		"recent_posts": []any{
			map[string]any{
				"caption":    "spent the week dissecting … [opus:XYZ]",
				"engagement": map[string]any{"views": 359, "likes": 3, "replies": 0},
			},
		},
	})
	rec := threadsMatch("XYZ")
	if rec.Pending || rec.Engagement == nil {
		t.Fatalf("expected caption-fallback match, got %+v", rec)
	}
	if v, _ := rec.Engagement["replies"].(float64); v != 0 {
		t.Errorf("replies = %v, want 0", rec.Engagement["replies"])
	}
}

// No snapshot on disk → pending with the platform's task id (symmetric w/ liPersonalMatch).
func TestSnapshotPostsMatch_PendingWhenAbsent(t *testing.T) {
	t.Setenv("CA_TIKTOK_CACHE_DIR", t.TempDir()) // empty dir, no snapshot
	rec := tiktokMatch("ABC123")
	if !rec.Pending || rec.PendingTask != pendingTiktok {
		t.Fatalf("expected pending %s, got %+v", pendingTiktok, rec)
	}
}

// Buffer (#371): per-service recent_posts[] resolves an IG opus derivative by
// channel + clipID, and the whole engagement object + join_method come through.
func TestBufferMatch_ByChannelAndTag(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CA_BUFFER_CACHE_DIR", dir)
	writeSnap(t, dir, map[string]any{
		"recent_posts": []any{
			map[string]any{
				"channel":    "instagram/business",
				"caption":    "Claude, Codex, Gemini … [opus:WSJqHQbFXl]",
				"source_tag": map[string]any{"scheme": "opus", "id": "WSJqHQbFXl"},
				"engagement": map[string]any{"impressions": 214, "reach": 162, "likes": 3, "comments": 0},
			},
		},
	})
	rec := bufferMatch("instagram_business", "WSJqHQbFXl", "")
	if rec.Pending || rec.Engagement == nil {
		t.Fatalf("expected IG match, got %+v", rec)
	}
	if rec.Engagement["join_method"] != "tag" {
		t.Errorf("join_method = %v, want tag", rec.Engagement["join_method"])
	}
	if v, _ := rec.Engagement["impressions"].(float64); v != 214 {
		t.Errorf("impressions = %v, want 214", rec.Engagement["impressions"])
	}
}

// Buffer channel filter prevents cross-service double-counting: the same clip
// posted to IG must NOT resolve when the JOIN asks for facebook_page.
func TestBufferMatch_ChannelFilterNoCrossService(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CA_BUFFER_CACHE_DIR", dir)
	writeSnap(t, dir, map[string]any{
		"recent_posts": []any{
			map[string]any{
				"channel":    "instagram/business",
				"source_tag": map[string]any{"scheme": "opus", "id": "WSJqHQbFXl"},
				"engagement": map[string]any{"impressions": 214},
			},
		},
	})
	rec := bufferMatch("facebook_page", "WSJqHQbFXl", "")
	if rec.Pending {
		t.Fatalf("expected no_match (wrong channel), got pending: %+v", rec)
	}
	if rec.Reason != "no_match" {
		t.Errorf("reason = %q, want no_match (IG post must not satisfy a facebook_page query)", rec.Reason)
	}
}

// Buffer legacy fallback: a snapshot with neither recent_posts[] nor scheduleId
// stays pending #371 (unchanged behavior for old snapshots).
func TestBufferMatch_PendingWhenNoRecentPosts(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CA_BUFFER_CACHE_DIR", dir)
	writeSnap(t, dir, map[string]any{
		"channel_roi": []any{map[string]any{"channel": "instagram/business"}},
	})
	rec := bufferMatch("instagram_business", "WSJqHQbFXl", "")
	if !rec.Pending || rec.PendingTask != pendingBufferFormat {
		t.Fatalf("expected pending %s, got %+v", pendingBufferFormat, rec)
	}
}

// LinkedIn personal: opus clips on LI-personal often get 0 reactions but real
// owner-view impressions — those MUST flow into the engagement object (else the
// derived-reach sum, which is views+impressions, drops them). Regression for the
// liPersonalMatch impressions field.
func TestLiPersonalMatch_SurfacesImpressions(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CA_LINKEDIN_CACHE_DIR", dir)
	writeSnap(t, dir, map[string]any{
		"profile": map[string]any{
			"recent_posts": []any{
				map[string]any{
					"text":       "Tokens currently burn in debugging [opus:10EJC3NIge]",
					"source_tag": map[string]any{"scheme": "opus", "id": "10EJC3NIge"},
					"impressions": 73, "reactions": 0, "comments": 0,
				},
			},
		},
	})
	rec := liPersonalMatch("10EJC3NIge")
	if rec.Pending || rec.Engagement == nil {
		t.Fatalf("expected match, got %+v", rec)
	}
	if v, _ := rec.Engagement["impressions"].(float64); v != 73 {
		t.Errorf("impressions = %v, want 73 (a 0-reaction LI clip still has reach)", rec.Engagement["impressions"])
	}
}

// Snapshot present but the clip's tag isn't in it → no_match (NOT pending).
func TestSnapshotPostsMatch_NoMatchWhenTagAbsent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CA_THREADS_CACHE_DIR", dir)
	writeSnap(t, dir, map[string]any{
		"recent_posts": []any{
			map[string]any{"caption": "organic post, no tag", "engagement": map[string]any{"views": 10}},
		},
	})
	rec := threadsMatch("NOPE")
	if rec.Pending {
		t.Fatalf("expected no_match, got pending: %+v", rec)
	}
	if rec.Reason != "no_match" {
		t.Errorf("reason = %q, want no_match", rec.Reason)
	}
}
