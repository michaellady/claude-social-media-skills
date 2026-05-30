package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestVideoFilter_Cohorts(t *testing.T) {
	videos := []Video{
		{ID: "a", Title: "A", VideoType: "long-form"},
		{ID: "b", Title: "B", VideoType: "long-form"},
		{ID: "c", Title: "C", VideoType: "live"},
	}
	assignments := map[string][]string{
		"a": {"gastown-series"},
		"b": {"evergreen"},
		"c": {"gastown-series", "live-deep-dives"},
	}

	tests := []struct {
		name    string
		filter  VideoFilter
		wantIDs []string
	}{
		{
			name:    "single cohort",
			filter:  VideoFilter{Cohorts: []string{"gastown-series"}, CohortAssignments: assignments},
			wantIDs: []string{"a", "c"},
		},
		{
			name:    "multiple cohorts is OR",
			filter:  VideoFilter{Cohorts: []string{"gastown-series", "evergreen"}, CohortAssignments: assignments},
			wantIDs: []string{"a", "b", "c"},
		},
		{
			name:    "no cohort filter passes all",
			filter:  VideoFilter{},
			wantIDs: []string{"a", "b", "c"},
		},
		{
			name:    "unknown cohort matches nothing",
			filter:  VideoFilter{Cohorts: []string{"made-up"}, CohortAssignments: assignments},
			wantIDs: []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.filter.Apply(videos)
			gotIDs := make([]string, 0, len(got))
			for _, v := range got {
				gotIDs = append(gotIDs, v.ID)
			}
			if !sliceEq(gotIDs, tc.wantIDs) {
				t.Errorf("got %v, want %v", gotIDs, tc.wantIDs)
			}
		})
	}
}

func TestVideoFilter_CohortsCombineWithType(t *testing.T) {
	videos := []Video{
		{ID: "a", VideoType: "long-form"},
		{ID: "b", VideoType: "live"},
	}
	assignments := map[string][]string{
		"a": {"gastown-series"},
		"b": {"gastown-series"},
	}
	f := VideoFilter{
		Cohorts:           []string{"gastown-series"},
		CohortAssignments: assignments,
		Types:             []string{"live"},
	}
	got := f.Apply(videos)
	if len(got) != 1 || got[0].ID != "b" {
		t.Errorf("expected [b], got %+v", got)
	}
}

func TestVideo_JSONRoundtrip_NewFieldsPopulated(t *testing.T) {
	v := Video{
		ID:    "abc",
		Title: "Test",
		DailyMetrics: []DailyMetric{
			{Date: "2026-04-19", Views: 100, EstimatedMinutes: 50.5, AvgRetention: 33.3, SubscribersGained: 2},
			{Date: "2026-04-20", Views: 50, EstimatedMinutes: 20.0},
		},
		TrafficSources: map[string]TrafficSourceMetric{
			"YT_SEARCH":     {Views: 80, WatchMin: 30.0},
			"RELATED_VIDEO": {Views: 20, WatchMin: 10.0},
		},
		SubStatusMetrics: map[string]TrafficSourceMetric{
			"SUBSCRIBED":   {Views: 30},
			"UNSUBSCRIBED": {Views: 70},
		},
	}
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var got Video
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.DailyMetrics) != 2 || got.DailyMetrics[0].Date != "2026-04-19" || got.DailyMetrics[0].Views != 100 {
		t.Errorf("daily metrics roundtrip lost data: %+v", got.DailyMetrics)
	}
	if got.TrafficSources["YT_SEARCH"].Views != 80 {
		t.Errorf("traffic sources roundtrip lost data: %+v", got.TrafficSources)
	}
	if got.SubStatusMetrics["UNSUBSCRIBED"].Views != 70 {
		t.Errorf("sub-status roundtrip lost data: %+v", got.SubStatusMetrics)
	}
}

func TestVideo_JSONRoundtrip_NewFieldsAbsent(t *testing.T) {
	v := Video{ID: "abc", Title: "Test", ViewCount: 10}
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	// omitempty must elide the new fields when nil/empty.
	for _, key := range []string{"daily_metrics", "traffic_sources", "sub_status_metrics"} {
		if strings.Contains(string(raw), key) {
			t.Errorf("expected %q to be omitted from JSON when absent, got %s", key, raw)
		}
	}
	var got Video
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != "abc" || got.ViewCount != 10 {
		t.Errorf("base fields lost: %+v", got)
	}
}

func sliceEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
