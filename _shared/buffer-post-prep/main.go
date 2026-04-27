// buffer-post-prep — validate and shape arguments for mcp__buffer__create_post.
//
// Pure transport. No cognition. Refuses invalid input; never decides content.
// Skills (running inside the Claude harness) call this binary, then pass its
// JSON output as arguments to the actual mcp__buffer__create_post call.
//
// Usage:
//
//	buffer-post-prep \
//	  --channel-id <24-hex-chars> \
//	  --service <linkedin|facebook|instagram|threads|twitter|bluesky|mastodon|pinterest> \
//	  --text "<post text including CTA>" \
//	  --format-tag <verbatim_quote|teaser|carousel|link_share|batch_summary|long_form_pulse> \
//	  [--image-url "<url>"] \
//	  [--image-alt "<alt text>"] \
//	  [--mode addToQueue|shareNow|customScheduled] \
//	  [--due-at <ISO 8601>] \
//	  [--pinterest-board-id <id>]
//
// Output: JSON dict ready to pass as create_post args.
// Exit codes: 0=ok, 64=usage error, 65=validation error.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Platform character limits (hard limits from each platform).
var platformLimits = map[string]int{
	"twitter":   280,
	"bluesky":   300,
	"pinterest": 300,
	"threads":   500,
	"facebook":  500,
	"mastodon":  500,
	"instagram": 2200,
	"tiktok":    2200,
	"linkedin":  3000,
}

// Valid format tag values (matches _shared/format_tags.json keys exactly).
// CALLER passes the underscored key (e.g. verbatim_quote); the binary maps it
// to the hyphenated Buffer tag value (e.g. format:verbatim-quote).
var validFormatTags = map[string]string{
	"verbatim_quote":  "format:verbatim-quote",
	"teaser":          "format:teaser",
	"carousel":        "format:carousel",
	"link_share":      "format:link-share",
	"batch_summary":   "format:batch-summary",
	"long_form_pulse": "format:long-form-pulse",
}

var channelIDRe = regexp.MustCompile(`^[a-f0-9]{24}$`)

func main() {
	var (
		channelID      = flag.String("channel-id", "", "Buffer channel ID (24 hex chars)")
		service        = flag.String("service", "", "platform service name")
		text           = flag.String("text", "", "post text (must include CTA if applicable)")
		formatTag      = flag.String("format-tag", "", "format tag key (see _shared/format_tags.json)")
		imageURL       = flag.String("image-url", "", "optional image URL")
		imageAlt       = flag.String("image-alt", "", "optional image alt text (required if image-url set)")
		mode           = flag.String("mode", "addToQueue", "scheduling mode (addToQueue|shareNow|shareNext|customScheduled|recommendedTime)")
		dueAt          = flag.String("due-at", "", "ISO 8601 dueAt (required if mode=customScheduled)")
		pinterestBoard = flag.String("pinterest-board-id", "", "Pinterest board service ID (required for service=pinterest)")
	)
	flag.Parse()

	// --- Validation (pure transport — refuse invalid input, don't fix it) ---

	if *channelID == "" || !channelIDRe.MatchString(*channelID) {
		fail(64, "invalid --channel-id: must be 24 hex chars")
	}
	limit, ok := platformLimits[*service]
	if !ok {
		fail(64, "invalid --service: must be one of "+strings.Join(keys(platformLimits), ", "))
	}
	if strings.TrimSpace(*text) == "" {
		fail(64, "missing --text")
	}
	if len(*text) > limit {
		fail(65, fmt.Sprintf("text length %d exceeds %s hard limit %d", len(*text), *service, limit))
	}
	tagValue, ok := validFormatTags[*formatTag]
	if !ok {
		fail(64, "invalid --format-tag: must be one of "+strings.Join(keys(validFormatTags), ", "))
	}
	if *imageURL != "" && *imageAlt == "" {
		fail(64, "--image-alt is required when --image-url is set")
	}
	if *service == "instagram" && *imageURL == "" {
		fail(65, "instagram posts require --image-url")
	}
	if *mode == "customScheduled" && *dueAt == "" {
		fail(64, "--due-at is required when --mode=customScheduled")
	}
	if *service == "pinterest" && *pinterestBoard == "" {
		fail(65, "pinterest posts require --pinterest-board-id")
	}
	if *service == "pinterest" && *imageURL == "" {
		fail(65, "pinterest posts require --image-url (pins need an image)")
	}
	validModes := map[string]bool{"addToQueue": true, "shareNow": true, "shareNext": true, "customScheduled": true, "recommendedTime": true}
	if !validModes[*mode] {
		fail(64, "invalid --mode")
	}

	// --- Shape the args dict (pure transport — deterministic output) ---

	args := map[string]any{
		"channelId":      *channelID,
		"text":           *text,
		"mode":           *mode,
		"schedulingType": "automatic",
	}

	// Buffer's CreatePostInput requires tagIds: [TagId!] (24-char hex IDs),
	// NOT tag name strings. The earlier "tags": []string{tagValue} field
	// was silently dropped by Buffer (the schema has no "tags" parameter).
	// Look up the ID from tag-ids.local.json (gitignored — IDs are
	// per-organization). If the file is missing or the key isn't in it,
	// emit the post WITHOUT a tagId and warn on stderr — better to ship
	// untagged than fail the whole post.
	tagID := lookupTagID(*formatTag, tagValue)
	if tagID != "" {
		args["tagIds"] = []string{tagID}
	}
	if *dueAt != "" {
		args["dueAt"] = *dueAt
	}
	if *imageURL != "" {
		args["assets"] = map[string]any{
			"images": []map[string]any{
				{
					"url":      *imageURL,
					"metadata": map[string]any{"altText": *imageAlt},
				},
			},
		}
	}
	// Platform-specific metadata
	metadata := map[string]any{}
	switch *service {
	case "facebook":
		metadata["facebook"] = map[string]any{"type": "post"}
	case "instagram":
		metadata["instagram"] = map[string]any{"type": "post", "shouldShareToFeed": true}
	case "threads":
		metadata["threads"] = map[string]any{"type": "post"}
	case "pinterest":
		metadata["pinterest"] = map[string]any{"boardServiceId": *pinterestBoard}
	}
	if len(metadata) > 0 {
		args["metadata"] = metadata
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(args); err != nil {
		fail(70, "json encode: "+err.Error())
	}
}

func fail(code int, msg string) {
	fmt.Fprintln(os.Stderr, "buffer-post-prep: "+msg)
	os.Exit(code)
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// lookupTagID reads tag-ids.local.json (next to this binary) and returns the
// 24-char-hex Buffer Tag ID for the given format-tag key. Returns "" if the
// file is missing, the key isn't there, or the ID doesn't look valid — and
// warns on stderr. Skill keeps shipping the post (untagged) so closed-loop
// attribution degrades gracefully instead of blocking publication.
//
// Expected file shape (gitignored — IDs are per-organization):
//
//	{"verbatim_quote": "abc123...", "teaser": "def456...", ...}
//
// Setup once: create the format:* tags in Buffer's web UI, then look them up
// via mcp__buffer__execute_query with `posts {... tags { id name } ...}` on
// any post that has them, and paste the IDs here. See
// _shared/buffer-post-prep/tag-ids.example.json.
func lookupTagID(formatKey, tagValue string) string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return ""
	}
	cfgPath := filepath.Join(filepath.Dir(resolved), "tag-ids.local.json")
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr,
			"buffer-post-prep: WARN no %s — closed-loop attribution will be empty for tag %q. See tag-ids.example.json for setup.\n",
			cfgPath, tagValue)
		return ""
	}
	var ids map[string]string
	if err := json.Unmarshal(raw, &ids); err != nil {
		fmt.Fprintf(os.Stderr, "buffer-post-prep: WARN failed to parse %s: %v\n", cfgPath, err)
		return ""
	}
	id, ok := ids[formatKey]
	if !ok || id == "" {
		fmt.Fprintf(os.Stderr,
			"buffer-post-prep: WARN no Tag ID for format key %q in %s — post will ship untagged.\n",
			formatKey, cfgPath)
		return ""
	}
	if !channelIDRe.MatchString(id) {
		fmt.Fprintf(os.Stderr,
			"buffer-post-prep: WARN Tag ID for %q (%q) is not 24 hex chars — Buffer will reject. Skipping tag.\n",
			formatKey, id)
		return ""
	}
	return id
}
