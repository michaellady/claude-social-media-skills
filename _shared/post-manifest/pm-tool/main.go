// pm-tool — read/reconcile helpers for the closed-loop post-manifest.
//
// The post-manifest write helpers (pm_init/pm_ensure_clip/pm_append_post) stay
// in post_manifest.sh. This Go tool adds the two operations that make the
// standing "always capture closed-loop ids" rule ENFORCEABLE rather than
// aspirational:
//
//	pm-tool lint     --root <dir>            # fail on scheduled_posts missing a scheduleId
//	pm-tool backfill --manifest <p> [--dry-run]   # recover pending scheduleIds from OpusClip
//
// A manifest entry's closed-loop id lives at scheduled_posts[].api_response.data.scheduleId
// (the exact path content-attribution's JOIN reads). A bare {label, scheduled_at_utc}
// entry is dark to attribution — the 2026-05-31 batch wrote 25 such entries and
// nobody noticed until the JOIN went silent. `lint` catches that immediately.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fail(64, "usage: pm-tool <lint|backfill> [flags]")
	}
	switch os.Args[1] {
	case "lint":
		runLint(os.Args[2:])
	case "backfill":
		runBackfill(os.Args[2:])
	default:
		fail(64, "unknown subcommand %q (lint|backfill)", os.Args[1])
	}
}

// ---- manifest shape (only the fields we touch) ----

type manifest struct {
	ProjectID string  `json:"project_id"`
	Clips     []*clip `json:"clips"`
	raw       map[string]any // full doc, for lossless rewrite on backfill
}

type clip struct {
	ClipID         string           `json:"clip_id"`
	ScheduledPosts []map[string]any `json:"scheduled_posts"`
}

func scheduleID(sp map[string]any) string {
	ar, ok := sp["api_response"].(map[string]any)
	if !ok {
		return ""
	}
	data, ok := ar["data"].(map[string]any)
	if !ok {
		return ""
	}
	id, _ := data["scheduleId"].(string)
	return id
}

func isManifest(doc map[string]any) bool {
	_, ok := doc["clips"].([]any)
	return ok
}

// ---- lint ----

type gap struct{ file, clip, label, at string }

// collectGaps walks <root> for manifests and splits bare (scheduleId-less)
// scheduled_posts into recoverable (future/pending → fixable) and stale
// (already-fired → unrecoverable). Pure + time-injected, so it's testable.
func collectGaps(root string, now time.Time) (recoverable, stale []gap, nManifests int) {
	files, _ := filepath.Glob(filepath.Join(root, "*.json"))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var doc map[string]any
		if json.Unmarshal(b, &doc) != nil || !isManifest(doc) {
			continue // skip non-manifest helper files (transcripts, platform-results, …)
		}
		nManifests++
		var m manifest
		_ = json.Unmarshal(b, &m)
		for _, c := range m.Clips {
			for _, sp := range c.ScheduledPosts {
				if scheduleID(sp) != "" {
					continue
				}
				label, _ := sp["label"].(string)
				at, _ := sp["scheduled_at_utc"].(string)
				g := gap{filepath.Base(f), c.ClipID, label, at}
				// Future (pending) bare entry = a LIVE capture failure → fail.
				// Past (already-fired) = unrecoverable (OpusClip's API is
				// pending-only) → warn; it attributes by [opus:] tag.
				if t, ok := parseUTC(at); ok && t.After(now) {
					recoverable = append(recoverable, g)
				} else {
					stale = append(stale, g)
				}
			}
		}
	}
	sortGaps := func(gs []gap) { sort.Slice(gs, func(i, j int) bool { return gs[i].at < gs[j].at }) }
	sortGaps(recoverable)
	sortGaps(stale)
	return recoverable, stale, nManifests
}

func runLint(args []string) {
	fs := flag.NewFlagSet("lint", flag.ExitOnError)
	root := fs.String("root", "", "directory to walk for *.json manifests")
	_ = fs.Parse(args)
	if *root == "" {
		fail(64, "lint: --root is required")
	}
	recoverable, stale, nFiles := collectGaps(*root, time.Now().UTC())

	if len(stale) > 0 {
		fmt.Printf("⚠ %d stale gap(s) — fired without a captured scheduleId (attribute by [opus:] tag only):\n", len(stale))
		for _, g := range stale {
			fmt.Printf("   %s  %s  %q  @%s\n", g.file, g.clip, g.label, g.at)
		}
	}
	if len(recoverable) > 0 {
		fmt.Printf("✗ %d RECOVERABLE gap(s) — pending posts missing a scheduleId (run `pm-tool backfill`):\n", len(recoverable))
		for _, g := range recoverable {
			fmt.Printf("   %s  %s  %q  @%s\n", g.file, g.clip, g.label, g.at)
		}
		os.Exit(65)
	}
	fmt.Printf("✓ closed-loop-id coverage OK (%d manifests; %d stale unrecoverable gaps)\n", nFiles, len(stale))
}

// ---- backfill ----

const opusBase = "https://api.opus.pro/api"

func runBackfill(args []string) {
	fs := flag.NewFlagSet("backfill", flag.ExitOnError)
	mpath := fs.String("manifest", "", "manifest JSON to backfill")
	dry := fs.Bool("dry-run", false, "report changes without writing")
	_ = fs.Parse(args)
	if *mpath == "" {
		fail(64, "backfill: --manifest is required")
	}
	key := os.Getenv("OPUSCLIP_API_KEY")
	if key == "" {
		fail(64, "backfill: OPUSCLIP_API_KEY not set")
	}

	b, err := os.ReadFile(*mpath)
	if err != nil {
		fail(64, "read manifest: %v", err)
	}
	var doc map[string]any
	if json.Unmarshal(b, &doc) != nil || !isManifest(doc) {
		fail(64, "not a manifest (no clips[]): %s", *mpath)
	}
	pid, _ := doc["project_id"].(string)
	if pid == "" {
		fail(64, "manifest has no project_id")
	}
	now := time.Now().UTC()
	filled, skipped := 0, 0

	clips, _ := doc["clips"].([]any)
	for _, cAny := range clips {
		c, _ := cAny.(map[string]any)
		clipID, _ := c["clip_id"].(string)
		sps, _ := c["scheduled_posts"].([]any)

		var byPlat map[string]map[string]any
		for _, spAny := range sps {
			sp, _ := spAny.(map[string]any)
			if scheduleID(sp) != "" {
				continue
			}
			at, _ := sp["scheduled_at_utc"].(string)
			if t, ok := parseUTC(at); !ok || !t.After(now) {
				skipped++ // fired / undated → OpusClip won't return it
				continue
			}
			if byPlat == nil { // lazy fetch: one API call per clip, only if it has a gap
				byPlat = fetchByPlatform(key, pid, clipID)
			}
			label, _ := sp["label"].(string)
			plat := firstToken(label)
			rec, ok := byPlat[plat]
			if !ok {
				skipped++
				continue
			}
			sid, _ := rec["scheduleId"].(string)
			if sid == "" {
				skipped++
				continue
			}
			sp["api_response"] = map[string]any{"data": map[string]any{
				"scheduleId": sid,
				"publishAt":  rec["publishAt"],
				"platform":   rec["platform"],
				"extUserId":  rec["extUserId"],
				"extOrgId":   rec["extOrgId"],
			}}
			filled++
			fmt.Printf("  + %s %q -> %s\n", clipID, label, sid)
		}
	}

	fmt.Printf("backfill: %d filled, %d skipped (fired/undated/not-pending)\n", filled, skipped)
	if *dry {
		fmt.Println("(--dry-run: no write)")
		return
	}
	if filled == 0 {
		return
	}
	out, _ := json.MarshalIndent(doc, "", "  ")
	tmp := *mpath + ".tmp"
	if err := os.WriteFile(tmp, append(out, '\n'), 0o644); err != nil {
		fail(74, "write tmp: %v", err)
	}
	if err := os.Rename(tmp, *mpath); err != nil {
		fail(74, "atomic rename: %v", err)
	}
}

// fetchByPlatform queries OpusClip's pending-schedule endpoint for one clip and
// returns the live records keyed by platform (YOUTUBE, FACEBOOK_PAGE, …).
// Pending-only: a fired post is dropped from this endpoint (returns empty).
func fetchByPlatform(key, projectID, clipID string) map[string]map[string]any {
	u := fmt.Sprintf("%s/publish-schedules?q=findByProjectAndClip&projectId=%s&clipId=%s",
		opusBase, url.QueryEscape(projectID), url.QueryEscape(clipID))
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("User-Agent", "pm-tool/1.0") // a real UA or the API 403s
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	// response is {data:{data:[...]}} or {data:[...]}
	var env map[string]any
	if json.Unmarshal(body, &env) != nil {
		return nil
	}
	items := unwrapItems(env)
	out := map[string]map[string]any{}
	for _, it := range items {
		m, _ := it.(map[string]any)
		if plat, _ := m["platform"].(string); plat != "" {
			out[plat] = m // one record per platform in these manifests
		}
	}
	return out
}

func unwrapItems(env map[string]any) []any {
	d := env["data"]
	if inner, ok := d.(map[string]any); ok {
		if arr, ok := inner["data"].([]any); ok {
			return arr
		}
	}
	if arr, ok := d.([]any); ok {
		return arr
	}
	return nil
}

// ---- helpers ----

func parseUTC(s string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05Z", "2006-01-02T15:04:05.000Z"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

func firstToken(s string) string {
	if i := strings.IndexByte(s, ' '); i >= 0 {
		return s[:i]
	}
	return s
}

func fail(code int, format string, a ...any) {
	fmt.Fprintf(os.Stderr, "pm-tool: "+format+"\n", a...)
	os.Exit(code)
}
