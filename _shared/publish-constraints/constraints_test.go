package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

type goldenPair struct {
	Label    string `json:"label"`
	Platform string `json:"platform"`
}

func loadGolden(t *testing.T) []goldenPair {
	t.Helper()
	b, err := os.ReadFile("testdata/label-platform.golden.json")
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	var g []goldenPair
	if err := json.Unmarshal(b, &g); err != nil {
		t.Fatalf("parse golden: %v", err)
	}
	return g
}

// labelToPlatform here and in content-attribution must agree with the golden.
func TestLabelToPlatform_MatchesGolden(t *testing.T) {
	for _, p := range loadGolden(t) {
		if got := labelToPlatform(p.Label); got != p.Platform {
			t.Errorf("labelToPlatform(%q) = %q, want %q", p.Label, got, p.Platform)
		}
	}
}

// Completeness: the set of platforms the labels resolve to must exactly equal
// the set of constraints rows — so a new connected account can't map to a
// platform with no validation, and a stale row can't linger unreferenced.
func TestEveryGoldenPlatformHasConstraintsRow(t *testing.T) {
	golden := map[string]bool{}
	for _, p := range loadGolden(t) {
		golden[p.Platform] = true
		if _, ok := constraints[p.Platform]; !ok {
			t.Errorf("golden platform %q has no constraints row", p.Platform)
		}
	}
	for plat := range constraints {
		if !golden[plat] {
			t.Errorf("constraints row %q is not referenced by any golden label", plat)
		}
	}
}

// Every required field must declare a positive cap (catches a typo'd 0 cap).
func TestConstraints_RequiredFieldsHaveCaps(t *testing.T) {
	for plat, pc := range constraints {
		if _, ok := pc.Fields[pc.PostTextField]; !ok {
			t.Errorf("%s: post_text_field %q is not a declared field", plat, pc.PostTextField)
		}
		for name, fc := range pc.Fields {
			if fc.Required && fc.MaxRunes <= 0 {
				t.Errorf("%s.%s is required but has no positive max_runes", plat, name)
			}
		}
	}
}

// THE bug: the long caption passed as the YouTube --title (214 chars) must fail.
func TestValidate_YouTubeOverLongTitleFails(t *testing.T) {
	longCaption := strings.Repeat("a", 214)
	vs := validateCell(cell{ClipID: "X", Label: "YOUTUBE Enterprise Vibe Code", Title: longCaption, Description: longCaption})
	if !hasVerdict(vs, "youtube_shorts", "title", "exceeds") {
		t.Fatalf("expected a youtube_shorts title over-cap verdict, got %+v", vs)
	}
}

// The asymmetry: the SAME long text is a valid caption (title field) on Facebook.
func TestValidate_CaptionAsTitleAsymmetry(t *testing.T) {
	caption := strings.Repeat("b", 214) // > 100 (YT title) but < 2200 (FB caption)
	if vs := validateCell(cell{Label: "FACEBOOK_PAGE Enterprise Vibe Code", Title: caption}); len(vs) != 0 {
		t.Errorf("facebook caption of 214 chars should pass, got %+v", vs)
	}
	if vs := validateCell(cell{Label: "YOUTUBE Enterprise Vibe Code", Title: caption, Description: caption}); len(vs) == 0 {
		t.Errorf("youtube title of 214 chars should fail")
	}
}

// The fix shape: short title + long description on YouTube passes.
func TestValidate_YouTubeValidPasses(t *testing.T) {
	vs := validateCell(cell{
		Label:       "YOUTUBE Enterprise Vibe Code",
		Title:       "Factory-farming code", // 20 chars
		Description: strings.Repeat("c", 240),
	})
	if len(vs) != 0 {
		t.Errorf("valid youtube cell should pass, got %+v", vs)
	}
}

// Symmetric guard: caption put only in description, title left blank → fail.
func TestValidate_YouTubeMissingTitleFails(t *testing.T) {
	vs := validateCell(cell{Label: "YOUTUBE Enterprise Vibe Code", Title: "", Description: "the whole caption"})
	if !hasVerdict(vs, "youtube_shorts", "title", "required") {
		t.Fatalf("expected required-title verdict, got %+v", vs)
	}
}

// Fail closed: a label no row covers must not pass silently.
func TestValidate_UnknownLabelFailsClosed(t *testing.T) {
	vs := validateCell(cell{Label: "MASTODON some-acct", Title: "hi"})
	if !hasVerdict(vs, "unknown", "label", "no constraints row") {
		t.Fatalf("expected fail-closed verdict for unknown label, got %+v", vs)
	}
}

func hasVerdict(vs []verdict, platform, field, reasonSubstr string) bool {
	for _, v := range vs {
		if v.Platform == platform && v.Field == field && strings.Contains(v.Reason, reasonSubstr) {
			return true
		}
	}
	return false
}
