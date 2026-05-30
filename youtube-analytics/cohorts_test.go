package main

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"
)

func TestLoadCohorts_YAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cohorts.yaml")
	yaml := `- id: gastown-series
  name: Gastown content
  description: Reviews of Gas Town
  rules:
    - title_regex: '(?i)gas\s*town|gastown'
- id: live-deep-dives
  name: Long live streams
  rules:
    - video_type: live
      duration_min: 1800
`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	cohorts, err := loadCohorts(path)
	if err != nil {
		t.Fatalf("loadCohorts: %v", err)
	}
	if len(cohorts) != 2 {
		t.Fatalf("want 2 cohorts, got %d", len(cohorts))
	}
	if cohorts[0].ID != "gastown-series" || cohorts[1].ID != "live-deep-dives" {
		t.Errorf("unexpected ids: %+v", cohorts)
	}
	if cohorts[0].Rules[0].TitleRegex == "" {
		t.Errorf("missing title_regex on gastown rule")
	}
	if cohorts[1].Rules[0].VideoType != "live" || cohorts[1].Rules[0].DurationMin != 1800 {
		t.Errorf("bad live rule: %+v", cohorts[1].Rules[0])
	}
}

func TestLoadCohorts_RejectsBadRegex(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cohorts.yaml")
	yaml := `- id: broken
  name: Has bad regex
  rules:
    - title_regex: '(unclosed'
`
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadCohorts(path); err == nil {
		t.Errorf("expected load to fail on invalid regex; succeeded silently")
	}
}

func TestLoadCohorts_Missing(t *testing.T) {
	cohorts, err := loadCohorts(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatalf("missing file should not error, got %v", err)
	}
	if len(cohorts) != 0 {
		t.Errorf("expected empty, got %d", len(cohorts))
	}
}

func TestAssignmentsRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cohort_assignments.json")
	in := map[string][]string{
		"abc": {"gastown-series", "live-deep-dives"},
		"def": {"gastown-series"},
	}
	if err := saveAssignments(path, in); err != nil {
		t.Fatalf("save: %v", err)
	}
	out, err := loadAssignments(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("roundtrip mismatch:\n in: %v\nout: %v", in, out)
	}
}

func TestLoadAssignments_Missing(t *testing.T) {
	out, err := loadAssignments(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("missing file should not error, got %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty, got %v", out)
	}
}

func TestAutoAssign_TitleRegex(t *testing.T) {
	cohorts := []Cohort{{
		ID: "gastown-series",
		Rules: []CohortRule{{TitleRegex: `(?i)gas\s*town`}},
	}}
	videos := []Video{
		{ID: "a", Title: "First impressions of Gas Town"},
		{ID: "b", Title: "Gastown deep dive"},
		{ID: "c", Title: "Unrelated video about cats"},
	}
	got := autoAssignAll(cohorts, videos, nil)
	want := map[string][]string{
		"a": {"gastown-series"},
		"b": {"gastown-series"},
	}
	if !equalAssignments(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestAutoAssign_TypeAndDuration(t *testing.T) {
	cohorts := []Cohort{{
		ID: "live-deep-dives",
		Rules: []CohortRule{{VideoType: "live", DurationMin: 1800}},
	}}
	videos := []Video{
		{ID: "a", VideoType: "live", DurationSeconds: 3600}, // match
		{ID: "b", VideoType: "live", DurationSeconds: 600},  // too short
		{ID: "c", VideoType: "long-form", DurationSeconds: 3600}, // wrong type
	}
	got := autoAssignAll(cohorts, videos, nil)
	want := map[string][]string{"a": {"live-deep-dives"}}
	if !equalAssignments(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestAutoAssign_PreservesManual(t *testing.T) {
	// User manually assigned "abc" to "evergreen" — a cohort with no rules
	// (so the rule engine can't reproduce it). The next autoAssign run must
	// keep that manual assignment intact while still applying rule-driven
	// adds.
	cohorts := []Cohort{
		{ID: "gastown-series", Rules: []CohortRule{{TitleRegex: `(?i)gastown`}}},
		{ID: "evergreen", Rules: nil}, // no rules — purely manual
	}
	videos := []Video{
		{ID: "abc", Title: "Gastown deep dive"},
		{ID: "xyz", Title: "Random short"},
	}
	existing := map[string][]string{
		"abc": {"evergreen"}, // manual: not derivable from any rule
		"xyz": {"evergreen"},
	}
	got := autoAssignAll(cohorts, videos, existing)
	want := map[string][]string{
		"abc": {"evergreen", "gastown-series"},
		"xyz": {"evergreen"},
	}
	if !equalAssignments(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestAutoAssign_RuleRemovesStaleAssignment(t *testing.T) {
	// "abc" was previously rule-assigned to gastown-series. The video's
	// title changed and no longer matches. The stale rule-derived
	// assignment should be removed.
	cohorts := []Cohort{
		{ID: "gastown-series", Rules: []CohortRule{{TitleRegex: `(?i)gastown`}}},
	}
	videos := []Video{
		{ID: "abc", Title: "Renamed: now about Claude Code"},
	}
	existing := map[string][]string{
		"abc": {"gastown-series"}, // was rule-assigned, no longer matches
	}
	got := autoAssignAll(cohorts, videos, existing)
	if len(got["abc"]) != 0 {
		t.Errorf("stale rule-driven assignment should be removed, got %v", got)
	}
}

func TestAutoAssign_PublishedDateRule(t *testing.T) {
	cutoff := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	cohorts := []Cohort{{
		ID:    "april-onward",
		Rules: []CohortRule{{PublishedAfter: cutoff}},
	}}
	videos := []Video{
		{ID: "old", PublishedAt: time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)},
		{ID: "new", PublishedAt: time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)},
	}
	got := autoAssignAll(cohorts, videos, nil)
	want := map[string][]string{"new": {"april-onward"}}
	if !equalAssignments(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// equalAssignments compares maps treating slice order as insignificant.
func equalAssignments(a, b map[string][]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, va := range a {
		vb, ok := b[k]
		if !ok {
			return false
		}
		if len(va) != len(vb) {
			return false
		}
		ac := append([]string(nil), va...)
		bc := append([]string(nil), vb...)
		sort.Strings(ac)
		sort.Strings(bc)
		if !reflect.DeepEqual(ac, bc) {
			return false
		}
	}
	return true
}
