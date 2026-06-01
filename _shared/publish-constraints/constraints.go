package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

// constraints.json is the single source of truth for PLATFORM-NATIVE publish
// limits, per field. It is embedded so the binary is self-contained, and it is a
// committed data file so other modules (e.g. buffer-post-prep, P2) can read the
// same ceilings without a cross-module Go import.
//
// "Native ceiling" matters: a downstream router may impose a TIGHTER policy cap
// (Buffer caps Facebook posts at 500 even though Facebook itself allows far
// more). Those policy caps live in the router and must stay <= these ceilings;
// they are not represented here.
//
//go:embed constraints.json
var constraintsJSON []byte

// FieldConstraint is the limit for one named field (e.g. "title", "description").
type FieldConstraint struct {
	MaxRunes int `json:"max_runes"`
	// Required: the post fails if this field is empty. Catches the symmetric
	// bug — e.g. a YouTube post whose caption went into `description` while
	// `title` was left blank.
	Required bool `json:"required"`
	// ForbiddenRe: optional regexp of disallowed characters/sequences. Empty
	// for every platform today; the field exists so a real rejection (e.g. a
	// platform that forbids "<") becomes one data edit, not a code change.
	ForbiddenRe string `json:"forbidden_re,omitempty"`
}

// PlatformConstraints declares, per platform, the cap on each field AND which
// field the human-authored post-text/caption maps to. The whole 2026-05-31
// YouTube bug is the single fact `youtube_shorts.title_source == "separate"`:
// post-text goes to `description`, and `title` is a SEPARATE, 100-char-capped
// field — not the caption. On every other platform the post-text IS the title.
type PlatformConstraints struct {
	PostTextField string                     `json:"post_text_field"` // where the caption/post-text goes
	TitleSource   string                     `json:"title_source"`    // "separate" | "post_text"
	Fields        map[string]FieldConstraint `json:"fields"`
}

var constraints = mustLoadConstraints()

func mustLoadConstraints() map[string]PlatformConstraints {
	m := map[string]PlatformConstraints{}
	if err := json.Unmarshal(constraintsJSON, &m); err != nil {
		panic(fmt.Sprintf("publish-constraints: bad embedded constraints.json: %v", err))
	}
	return m
}

// runeLen counts CHARACTERS, not bytes — platform limits are in characters and
// the voice is full of em-dashes (3 bytes) and emoji (4 bytes). Same definition
// as _shared/buffer-post-prep/main.go.
func runeLen(s string) int { return utf8.RuneCountInString(s) }

// labelToPlatform maps an OpusClip schedule label (the human "<PLATFORM> <account>"
// string stored on each scheduled_posts[] entry) to the canonical platform key.
//
// CANONICAL COPY. _shared/content-attribution/main.go has an identical function
// for the JOIN; both are pinned to testdata/label-platform.golden.json by tests
// in each module. If you change one, change the golden file and both copies.
func labelToPlatform(label string) string {
	switch {
	case strings.HasPrefix(label, "YOUTUBE"):
		return "youtube_shorts"
	case strings.HasPrefix(label, "FACEBOOK_PAGE"):
		return "facebook_page"
	case strings.HasPrefix(label, "INSTAGRAM_BUSINESS"):
		return "instagram_business"
	case strings.HasPrefix(label, "LINKEDIN Mike Lady"):
		return "linkedin_personal"
	case strings.HasPrefix(label, "LINKEDIN"):
		return "linkedin_page"
	case strings.HasPrefix(label, "TIKTOK_BUSINESS"):
		return "tiktok_business"
	case strings.HasPrefix(label, "THREADS"):
		return "threads"
	default:
		return "unknown"
	}
}
