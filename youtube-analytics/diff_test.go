package main

import (
	"testing"
	"time"
)

func TestMergeCompareFilter_RangeOwnsDates(t *testing.T) {
	outer := VideoFilter{
		Since:             time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Until:             time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
		Cohorts:           []string{"gastown-series"},
		Types:             []string{"long-form"},
		CohortAssignments: map[string][]string{"abc": {"gastown-series"}},
	}
	rangeFilter := VideoFilter{
		Since: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		Until: time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC),
	}
	got := mergeCompareFilter(outer, rangeFilter)

	if !got.Since.Equal(rangeFilter.Since) || !got.Until.Equal(rangeFilter.Until) {
		t.Errorf("range should own dates, got Since=%v Until=%v", got.Since, got.Until)
	}
	if len(got.Cohorts) != 1 || got.Cohorts[0] != "gastown-series" {
		t.Errorf("outer Cohorts must be inherited, got %v", got.Cohorts)
	}
	if len(got.Types) != 1 || got.Types[0] != "long-form" {
		t.Errorf("outer Types must be inherited, got %v", got.Types)
	}
	if got.CohortAssignments["abc"] == nil {
		t.Errorf("outer CohortAssignments must be inherited")
	}
}

func TestMergeCompareFilter_AppliedToVideos(t *testing.T) {
	// Pin the wiring: a video outside the range OR in the wrong cohort gets dropped.
	videos := []Video{
		{ID: "in-range-in-cohort", VideoType: "long-form",
			PublishedAt: time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)},
		{ID: "out-of-range", VideoType: "long-form",
			PublishedAt: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)},
		{ID: "in-range-wrong-cohort", VideoType: "long-form",
			PublishedAt: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)},
	}
	outer := VideoFilter{
		Cohorts: []string{"gastown-series"},
		CohortAssignments: map[string][]string{
			"in-range-in-cohort":    {"gastown-series"},
			"out-of-range":          {"gastown-series"},
			"in-range-wrong-cohort": {"evc-numbered"},
		},
	}
	rangeFilter := VideoFilter{
		Since: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		Until: time.Date(2026, 4, 30, 23, 59, 59, 0, time.UTC),
	}
	merged := mergeCompareFilter(outer, rangeFilter)
	got := merged.Apply(videos)
	if len(got) != 1 || got[0].ID != "in-range-in-cohort" {
		t.Errorf("expected only [in-range-in-cohort], got %+v", got)
	}
}
