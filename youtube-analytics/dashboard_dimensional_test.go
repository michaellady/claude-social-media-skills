package main

import (
	"strings"
	"testing"
)

func TestBuildDashTrafficMix_EmptyIsFalse(t *testing.T) {
	has, _ := buildDashTrafficMix([]Video{{ID: "a"}})
	if has {
		t.Errorf("expected has=false when no traffic_sources present")
	}
}

func TestBuildDashTrafficMix_AggregatesAndRanks(t *testing.T) {
	videos := []Video{
		{ID: "a", TrafficSources: map[string]TrafficSourceMetric{
			"YT_SEARCH":  {Views: 80, WatchMin: 30},
			"SUBSCRIBER": {Views: 20, WatchMin: 10},
		}},
		{ID: "b", TrafficSources: map[string]TrafficSourceMetric{
			"YT_SEARCH":     {Views: 20, WatchMin: 5},
			"RELATED_VIDEO": {Views: 200, WatchMin: 80},
		}},
	}
	has, jsonOut := buildDashTrafficMix(videos)
	if !has {
		t.Fatalf("expected has=true with traffic data present")
	}
	s := string(jsonOut)
	// Top by views: RELATED_VIDEO=200, then YT_SEARCH=100, then SUBSCRIBER=20.
	posTop := strings.Index(s, "RELATED_VIDEO")
	posMid := strings.Index(s, "YT_SEARCH")
	posLow := strings.Index(s, "SUBSCRIBER")
	if posTop < 0 || posMid < 0 || posLow < 0 || posTop > posMid || posMid > posLow {
		t.Errorf("expected sorted desc by views (RELATED_VIDEO, YT_SEARCH, SUBSCRIBER):\n%s", s)
	}
	if !strings.Contains(s, `"views":200`) {
		t.Errorf("expected RELATED_VIDEO views=200, got %s", s)
	}
}

func TestBuildDashTopUnsubConversion_BelowThresholdExcluded(t *testing.T) {
	videos := []Video{
		{ID: "ok", Title: "Above threshold", SubscribersGained: 10,
			SubStatusMetrics: map[string]TrafficSourceMetric{"UNSUBSCRIBED": {Views: 500}}},
		{ID: "skip", Title: "Below threshold", SubscribersGained: 5,
			SubStatusMetrics: map[string]TrafficSourceMetric{"UNSUBSCRIBED": {Views: 50}}},
	}
	has, rows := buildDashTopUnsubConversion(videos)
	if !has {
		t.Fatalf("expected has=true with at least one qualifying video")
	}
	if len(rows) != 1 {
		t.Errorf("expected 1 qualifying row (50-view video skipped), got %d", len(rows))
	}
	if !strings.Contains(rows[0].Title, "Above threshold") {
		t.Errorf("wrong row kept: %+v", rows[0])
	}
}

func TestBuildDashTopUnsubConversion_NoSubStatusIsFalse(t *testing.T) {
	has, rows := buildDashTopUnsubConversion([]Video{{ID: "x"}})
	if has || rows != nil {
		t.Errorf("expected (false, nil) without SubStatusMetrics, got (%v, %v)", has, rows)
	}
}
