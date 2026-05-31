package main

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("abs %s: %v", name, err)
	}
	return p
}

// contractKeys is every top-level key the frozen contract (schema_version "1")
// promises. The dashboard must surface each one when present and must not panic
// when one is absent (old-shape snapshot).
var contractKeys = []string{
	"schema_version", "generated_at", "window", "youtube", "beehiiv",
	"linkedin", "buffer", "consulting", "priority_1", "channel_roi",
	"format_engagement", "reconciled_reach", "voice_corpus_freshness",
	"content_attribution",
}

func TestLoadSnapshotFull_AllContractKeysPresent(t *testing.T) {
	snap, err := loadSnapshot(fixture(t, "snapshot-full.json"))
	if err != nil {
		t.Fatalf("loadSnapshot: %v", err)
	}
	for _, k := range contractKeys {
		if !snap.has(k) {
			t.Errorf("full fixture missing contract key %q", k)
		}
	}
	// schema_version should read back as the JSON-encoded string "1".
	if got := string(snap.raw("schema_version")); got != `"1"` {
		t.Errorf("schema_version raw = %s, want \"1\"", got)
	}
}

func TestLoadSnapshotOldShape_TolerantDecode(t *testing.T) {
	snap, err := loadSnapshot(fixture(t, "snapshot-old.json"))
	if err != nil {
		t.Fatalf("old-shape snapshot must load, got error: %v", err)
	}
	// Present keys.
	for _, k := range []string{"window", "youtube", "beehiiv"} {
		if !snap.has(k) {
			t.Errorf("old fixture should have key %q", k)
		}
	}
	// Absent contract keys: has==false, raw==null, no panic.
	for _, k := range []string{"channel_roi", "format_engagement", "reconciled_reach", "voice_corpus_freshness", "content_attribution"} {
		if snap.has(k) {
			t.Errorf("old fixture should NOT have key %q", k)
		}
		if got := string(snap.raw(k)); got != "null" {
			t.Errorf("raw(%q) on absent key = %s, want null", k, got)
		}
	}
}

func TestSnapshotHas_NullAndEmpty(t *testing.T) {
	// A null value and an empty value both read as "not present".
	snap := &Snapshot{Top: map[string]json.RawMessage{
		"present": json.RawMessage(`{"a":1}`),
		"nulled":  json.RawMessage(`null`),
		"empty":   json.RawMessage(``),
	}}
	if !snap.has("present") {
		t.Error("present key should report has=true")
	}
	if snap.has("nulled") {
		t.Error("null key should report has=false")
	}
	if snap.has("empty") {
		t.Error("empty key should report has=false")
	}
	if snap.has("absent") {
		t.Error("absent key should report has=false")
	}
}

func TestFindSource_MatchAndMiss(t *testing.T) {
	snap, err := loadSnapshot(fixture(t, "snapshot-full.json"))
	if err != nil {
		t.Fatalf("loadSnapshot: %v", err)
	}
	rec, ok := snap.findSource("uEposKmbFvY")
	if !ok {
		t.Fatal("findSource(uEposKmbFvY) should match")
	}
	var probe struct {
		Source struct {
			Title string `json:"title"`
			Type  string `json:"type"`
		} `json:"source"`
		Amplification float64 `json:"amplification_ratio"`
		Derivatives   []any   `json:"derivatives"`
	}
	if err := json.Unmarshal(rec, &probe); err != nil {
		t.Fatalf("decode matched record: %v", err)
	}
	if probe.Source.Type != "long_form" {
		t.Errorf("source.type = %q, want long_form", probe.Source.Type)
	}
	if probe.Amplification != 11.2 {
		t.Errorf("amplification_ratio = %v, want 11.2", probe.Amplification)
	}
	if len(probe.Derivatives) != 2 {
		t.Errorf("derivatives len = %d, want 2", len(probe.Derivatives))
	}

	if _, ok := snap.findSource("does-not-exist"); ok {
		t.Error("findSource(does-not-exist) should miss")
	}
}

func TestFindSource_OldShapeNoAttribution(t *testing.T) {
	// content_attribution absent → findSource always misses, never panics.
	snap, err := loadSnapshot(fixture(t, "snapshot-old.json"))
	if err != nil {
		t.Fatalf("loadSnapshot: %v", err)
	}
	if _, ok := snap.findSource("anything"); ok {
		t.Error("findSource on a snapshot without content_attribution should miss")
	}
}
