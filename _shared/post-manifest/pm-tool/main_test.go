package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const fixtureManifest = `{
  "project_id": "P_TEST",
  "clips": [
    { "clip_id": "good", "scheduled_posts": [
      { "label": "YOUTUBE Acct", "scheduled_at_utc": "2026-07-01T16:00:00Z",
        "api_response": { "data": { "scheduleId": "sid-1-YOUTUBE" } } }
    ]},
    { "clip_id": "pending", "scheduled_posts": [
      { "label": "YOUTUBE Acct", "scheduled_at_utc": "2026-07-01T19:00:00Z" }
    ]},
    { "clip_id": "fired", "scheduled_posts": [
      { "label": "FACEBOOK_PAGE Acct", "scheduled_at_utc": "2026-06-01T19:00:00Z" }
    ]}
  ]
}`

// A helper file with no clips[] must be ignored (transcripts, platform-results).
const fixtureNonManifest = `{ "measured_at": "x", "rows": [] }`

func TestCollectGaps_SplitsRecoverableFromStale(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "m.json"), []byte(fixtureManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "results.json"), []byte(fixtureNonManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)

	rec, stale, nManifests := collectGaps(dir, now)

	if nManifests != 1 {
		t.Errorf("nManifests = %d, want 1 (non-manifest helper must be skipped)", nManifests)
	}
	if len(rec) != 1 || rec[0].clip != "pending" {
		t.Errorf("recoverable = %+v, want exactly the future 'pending' clip", rec)
	}
	if len(stale) != 1 || stale[0].clip != "fired" {
		t.Errorf("stale = %+v, want exactly the past 'fired' clip", stale)
	}
}

func TestCollectGaps_FullCoverageHasNoGaps(t *testing.T) {
	dir := t.TempDir()
	full := `{"project_id":"P","clips":[{"clip_id":"a","scheduled_posts":[
	  {"label":"YOUTUBE x","scheduled_at_utc":"2026-07-01T00:00:00Z","api_response":{"data":{"scheduleId":"s-YOUTUBE"}}}]}]}`
	os.WriteFile(filepath.Join(dir, "m.json"), []byte(full), 0o644)
	rec, stale, _ := collectGaps(dir, time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC))
	if len(rec) != 0 || len(stale) != 0 {
		t.Errorf("expected zero gaps, got rec=%+v stale=%+v", rec, stale)
	}
}

func TestScheduleID_ReadsNestedPath(t *testing.T) {
	sp := map[string]any{"api_response": map[string]any{"data": map[string]any{"scheduleId": "abc"}}}
	if got := scheduleID(sp); got != "abc" {
		t.Errorf("scheduleID = %q, want abc", got)
	}
	if got := scheduleID(map[string]any{"label": "x"}); got != "" {
		t.Errorf("bare entry scheduleID = %q, want empty", got)
	}
}
