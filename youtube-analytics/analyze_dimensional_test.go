package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

// captureStdout swaps os.Stdout for a pipe, runs fn, restores, and returns
// the captured output. Used to assert against the analyze rollup printers
// without restructuring them to take an io.Writer.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = orig }()
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()
	fn()
	w.Close()
	return <-done
}

func TestPrintTrafficSourceMix_EmptyIsSilent(t *testing.T) {
	out := captureStdout(t, func() { printTrafficSourceMix([]Video{{ID: "a"}}) })
	if out != "" {
		t.Errorf("expected silent output for videos without TrafficSources, got %q", out)
	}
}

func TestPrintTrafficSourceMix_AggregatesAndRanks(t *testing.T) {
	videos := []Video{
		{ID: "a", TrafficSources: map[string]TrafficSourceMetric{
			"YT_SEARCH":  {Views: 80, WatchMin: 30},
			"SUBSCRIBER": {Views: 20, WatchMin: 10},
		}},
		{ID: "b", TrafficSources: map[string]TrafficSourceMetric{
			"YT_SEARCH":     {Views: 20, WatchMin: 5},
			"RELATED_VIDEO": {Views: 100, WatchMin: 50},
		}},
	}
	out := captureStdout(t, func() { printTrafficSourceMix(videos) })

	if !strings.Contains(out, "Traffic Source Mix") {
		t.Errorf("missing section header:\n%s", out)
	}
	// Expected: YT_SEARCH=100 (largest), RELATED_VIDEO=100 (tie, second by sort), SUBSCRIBER=20.
	// Total = 220. YT_SEARCH should be 45.5%, RELATED_VIDEO 45.5%, SUBSCRIBER 9.1%.
	if !strings.Contains(out, "YT_SEARCH") || !strings.Contains(out, "RELATED_VIDEO") || !strings.Contains(out, "SUBSCRIBER") {
		t.Errorf("missing source label in output:\n%s", out)
	}
	if !strings.Contains(out, "45.5%") {
		t.Errorf("expected dominant-source share around 45.5%%, got:\n%s", out)
	}
	// Top entry must come before SUBSCRIBER (lower-share line).
	posTop := strings.Index(out, "YT_SEARCH")
	posLow := strings.Index(out, "SUBSCRIBER")
	if posTop < 0 || posLow < 0 || posTop > posLow {
		t.Errorf("expected sources ranked desc by views (YT_SEARCH before SUBSCRIBER):\n%s", out)
	}
}

func TestPrintTopByUnsubscribedConversion_EmptyIsSilent(t *testing.T) {
	out := captureStdout(t, func() { printTopByUnsubscribedConversion([]Video{{ID: "a"}}) })
	if out != "" {
		t.Errorf("expected silent output without sub_status_metrics, got %q", out)
	}
}

func TestPrintTopByUnsubscribedConversion_RanksByRate(t *testing.T) {
	videos := []Video{
		{ID: "low", Title: "Low conversion", VideoType: "long-form",
			SubscribersGained: 1,
			SubStatusMetrics:  map[string]TrafficSourceMetric{"UNSUBSCRIBED": {Views: 1000}}},
		{ID: "high", Title: "High conversion", VideoType: "live",
			SubscribersGained: 50,
			SubStatusMetrics:  map[string]TrafficSourceMetric{"UNSUBSCRIBED": {Views: 1000}}},
		{ID: "tinyN", Title: "Below min unsubbed views",
			SubscribersGained: 5,
			SubStatusMetrics:  map[string]TrafficSourceMetric{"UNSUBSCRIBED": {Views: 50}}}, // below 100 threshold
	}
	out := captureStdout(t, func() { printTopByUnsubscribedConversion(videos) })

	if !strings.Contains(out, "UNSUBSCRIBED Conversion") {
		t.Errorf("missing section header:\n%s", out)
	}
	posHigh := strings.Index(out, "High conversion")
	posLow := strings.Index(out, "Low conversion")
	if posHigh < 0 || posLow < 0 || posHigh > posLow {
		t.Errorf("expected high-rate video ranked first:\n%s", out)
	}
	if strings.Contains(out, "Below min unsubbed views") {
		t.Errorf("video with <100 unsubbed views should be excluded; got:\n%s", out)
	}
	// 50 subs / 1000 unsub views * 1000 = 50.00 subs/1K
	if !strings.Contains(out, "50.00") {
		t.Errorf("expected high-rate value 50.00, got:\n%s", out)
	}
}
