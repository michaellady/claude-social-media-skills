package main

import (
	"encoding/json"
	"os"
	"testing"
)

// labelToPlatform here is a copy of the canonical one in
// _shared/publish-constraints/constraints.go. Both are pinned to the SAME
// golden fixture so they can't silently diverge — if the OpusClip label→platform
// mapping changes, the golden file changes and both copies must follow.
func TestLabelToPlatform_MatchesPublishConstraintsGolden(t *testing.T) {
	b, err := os.ReadFile("../publish-constraints/testdata/label-platform.golden.json")
	if err != nil {
		t.Fatalf("read golden (is _shared/publish-constraints present?): %v", err)
	}
	var golden []struct {
		Label    string `json:"label"`
		Platform string `json:"platform"`
	}
	if err := json.Unmarshal(b, &golden); err != nil {
		t.Fatalf("parse golden: %v", err)
	}
	for _, p := range golden {
		if got := labelToPlatform(p.Label); got != p.Platform {
			t.Errorf("labelToPlatform(%q) = %q, want %q (diverged from publish-constraints)", p.Label, got, p.Platform)
		}
	}
}
