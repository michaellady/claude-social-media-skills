package main

import (
	"encoding/json"
	"os"
	"testing"
)

// buffer-post-prep's platformLimits are a Buffer POLICY (sometimes deliberately
// tighter than the platform allows — e.g. facebook=500 even though Facebook
// permits far more). The PLATFORM-NATIVE ceilings live in one place:
// _shared/publish-constraints/constraints.json. This test makes that file the
// single source of truth for the ceiling and guards that no Buffer policy cap
// ever EXCEEDS it — without changing the intentional tightening (a literal
// "read the native cap" would loosen facebook 500→2200, a behavior change).
//
// Buffer services with no OpusClip-native counterpart (twitter, bluesky,
// pinterest, mastodon) are Buffer-only surfaces and are skipped — there is no
// shared ceiling to check them against.
var serviceToNativePlatform = map[string]string{
	"threads":   "threads",
	"facebook":  "facebook_page",
	"instagram": "instagram_business",
	"tiktok":    "tiktok_business",
	"linkedin":  "linkedin_page", // linkedin_personal shares the same 3000 cap
}

func TestBufferPolicyCapsNeverExceedNativeCeiling(t *testing.T) {
	b, err := os.ReadFile("../publish-constraints/constraints.json")
	if err != nil {
		t.Fatalf("read shared native ceilings (is _shared/publish-constraints present?): %v", err)
	}
	var native map[string]struct {
		PostTextField string `json:"post_text_field"`
		Fields        map[string]struct {
			MaxRunes int `json:"max_runes"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(b, &native); err != nil {
		t.Fatalf("parse constraints.json: %v", err)
	}

	for service, policyCap := range platformLimits {
		plat, mapped := serviceToNativePlatform[service]
		if !mapped {
			continue // Buffer-only surface, no shared ceiling
		}
		pc, ok := native[plat]
		if !ok {
			t.Errorf("service %q maps to native platform %q which has no constraints row", service, plat)
			continue
		}
		ceiling := pc.Fields[pc.PostTextField].MaxRunes // cap on the caption/post-text field
		if ceiling == 0 {
			t.Errorf("%s: native post-text field %q has no cap", plat, pc.PostTextField)
			continue
		}
		if policyCap > ceiling {
			t.Errorf("buffer policy cap %s=%d EXCEEDS native %s ceiling %d — a post Buffer accepts would be rejected by the platform",
				service, policyCap, plat, ceiling)
		}
	}
}
