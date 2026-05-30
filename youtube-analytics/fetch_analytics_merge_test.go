package main

import "testing"

// Helper: build a column index map mirroring the youtubeanalytics ColumnHeaders shape.
func cidx(cols ...string) map[string]int {
	out := map[string]int{}
	for i, c := range cols {
		out[c] = i
	}
	return out
}

func TestMergeDailyRows_PopulatesSeries(t *testing.T) {
	videos := []Video{{ID: "abc"}, {ID: "def"}}
	idx := map[string]int{"abc": 0, "def": 1}
	cols := cidx("video", "day", "views", "estimatedMinutesWatched", "averageViewPercentage", "subscribersGained", "subscribersLost", "estimatedRevenue")

	rows := [][]interface{}{
		{"abc", "2026-04-19", float64(100), float64(50.5), float64(33.3), float64(2), float64(0), float64(0.10)},
		{"abc", "2026-04-20", float64(50), float64(20.0), float64(28.1), float64(1), float64(0), float64(0.05)},
		{"def", "2026-04-19", float64(10), float64(5.0), float64(40.0), float64(0), float64(0), float64(0)},
	}
	applied := mergeDailyRows(rows, cols, idx, videos)
	if applied != 3 {
		t.Errorf("applied = %d, want 3", applied)
	}
	if len(videos[0].DailyMetrics) != 2 {
		t.Fatalf("video abc daily count = %d, want 2", len(videos[0].DailyMetrics))
	}
	if videos[0].DailyMetrics[0].Date != "2026-04-19" || videos[0].DailyMetrics[0].Views != 100 {
		t.Errorf("first daily row wrong: %+v", videos[0].DailyMetrics[0])
	}
	if videos[1].DailyMetrics[0].AvgRetention != 40.0 {
		t.Errorf("def retention not parsed: %+v", videos[1].DailyMetrics[0])
	}
}

func TestMergeDailyRows_ReplacesPriorSeries(t *testing.T) {
	// Pin the contract: re-running --daily must NOT append a duplicate
	// series on top of an existing one.
	videos := []Video{{ID: "abc", DailyMetrics: []DailyMetric{
		{Date: "2025-01-01", Views: 9999}, // stale
	}}}
	idx := map[string]int{"abc": 0}
	cols := cidx("video", "day", "views", "estimatedMinutesWatched")
	rows := [][]interface{}{
		{"abc", "2026-04-19", float64(100), float64(50.5)},
		{"abc", "2026-04-20", float64(50), float64(20.0)},
	}
	mergeDailyRows(rows, cols, idx, videos)
	if len(videos[0].DailyMetrics) != 2 {
		t.Errorf("expected stale series replaced, got %d rows", len(videos[0].DailyMetrics))
	}
	if videos[0].DailyMetrics[0].Date == "2025-01-01" {
		t.Errorf("stale entry leaked into fresh series")
	}
}

func TestMergeDailyRows_SkipsUnknownVideoIDs(t *testing.T) {
	videos := []Video{{ID: "abc"}}
	idx := map[string]int{"abc": 0}
	cols := cidx("video", "day", "views", "estimatedMinutesWatched")
	rows := [][]interface{}{
		{"orphan", "2026-04-19", float64(100), float64(50.0)},
		{"abc", "2026-04-19", float64(50), float64(20.0)},
	}
	mergeDailyRows(rows, cols, idx, videos)
	if len(videos[0].DailyMetrics) != 1 {
		t.Errorf("expected only the abc row applied, got %d", len(videos[0].DailyMetrics))
	}
}

func TestMergeCategoricalRows_TrafficSources(t *testing.T) {
	videos := []Video{{ID: "abc"}}
	idx := map[string]int{"abc": 0}
	cols := cidx("video", "insightTrafficSourceType", "views", "estimatedMinutesWatched")
	rows := [][]interface{}{
		{"abc", "YT_SEARCH", float64(80), float64(30.0)},
		{"abc", "RELATED_VIDEO", float64(20), float64(10.0)},
	}
	applied := mergeCategoricalRows(rows, cols, idx, videos, "insightTrafficSourceType", "traffic_sources")
	if applied != 2 {
		t.Errorf("applied = %d, want 2", applied)
	}
	if videos[0].TrafficSources["YT_SEARCH"].Views != 80 {
		t.Errorf("YT_SEARCH not stored: %+v", videos[0].TrafficSources)
	}
	if videos[0].TrafficSources["RELATED_VIDEO"].WatchMin != 10.0 {
		t.Errorf("RELATED_VIDEO watch_min not stored: %+v", videos[0].TrafficSources)
	}
}

func TestMergeCategoricalRows_SubStatus(t *testing.T) {
	videos := []Video{{ID: "abc"}}
	idx := map[string]int{"abc": 0}
	cols := cidx("video", "subscribedStatus", "views", "estimatedMinutesWatched")
	rows := [][]interface{}{
		{"abc", "SUBSCRIBED", float64(30), float64(20.0)},
		{"abc", "UNSUBSCRIBED", float64(70), float64(40.0)},
	}
	mergeCategoricalRows(rows, cols, idx, videos, "subscribedStatus", "sub_status_metrics")
	if videos[0].SubStatusMetrics["SUBSCRIBED"].Views != 30 {
		t.Errorf("SUBSCRIBED not stored: %+v", videos[0].SubStatusMetrics)
	}
	if videos[0].SubStatusMetrics["UNSUBSCRIBED"].Views != 70 {
		t.Errorf("UNSUBSCRIBED not stored: %+v", videos[0].SubStatusMetrics)
	}
}

func TestMergeCategoricalRows_ReplacesPriorMap(t *testing.T) {
	// A prior fetch added STALE_SOURCE; a fresh fetch with only YT_SEARCH
	// must wipe the map first, not leave the stale key behind.
	videos := []Video{{ID: "abc", TrafficSources: map[string]TrafficSourceMetric{
		"STALE_SOURCE": {Views: 999},
	}}}
	idx := map[string]int{"abc": 0}
	cols := cidx("video", "insightTrafficSourceType", "views", "estimatedMinutesWatched")
	rows := [][]interface{}{
		{"abc", "YT_SEARCH", float64(80), float64(30.0)},
	}
	mergeCategoricalRows(rows, cols, idx, videos, "insightTrafficSourceType", "traffic_sources")
	if _, ok := videos[0].TrafficSources["STALE_SOURCE"]; ok {
		t.Errorf("stale source key not cleared: %+v", videos[0].TrafficSources)
	}
}
