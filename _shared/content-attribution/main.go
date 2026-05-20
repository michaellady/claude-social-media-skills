// content-attribution — the JOIN engine for the unified closed-loop.
//
// Reads post-manifests + per-platform stats snapshots, correlates a source
// piece of content to its derivatives' engagement across every platform, and
// emits a unified per-source record. Pure transport (no cognition) per the
// Primitive Test — the judgment of what to DO with the numbers lives in the
// caller skill (/flywheel, /opus-clips-performance).
//
// Replaces the original bash module: a Go binary runs identically regardless
// of the caller's shell (the bash version broke under zsh — nomatch globs +
// mangled sourced-function output).
//
// Subcommands (all emit JSON to stdout unless noted):
//
//	content-attribution sources
//	content-attribution join     --source-id <id> [--source-type T] [--manifest P]
//	content-attribution report   --source-id <id> [--format md|json]
//	content-attribution extract-tag <text...>
//
// Exit codes: 0=ok, 64=usage error.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ---------- Paths (env-overridable, sane defaults) ----------

func home() string {
	h, _ := os.UserHomeDir()
	return h
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func ytVideosPath() string {
	return envOr("CA_YT_VIDEOS", filepath.Join(home(), "dev/youtube_analytics/data/videos.json"))
}
func opusManifestsDir() string {
	return envOr("CA_OPUS_MANIFESTS_DIR", filepath.Join(home(), "dev/youtube_analytics/data/opus_clips"))
}
func manifestsRoot() string {
	return envOr("CA_MANIFESTS_ROOT", filepath.Join(home(), "dev/youtube_analytics/data"))
}
func bufferCacheDir() string {
	return envOr("CA_BUFFER_CACHE_DIR", filepath.Join(home(), "dev/claude-social-media-skills/buffer-stats/cache"))
}
func linkedinCacheDir() string {
	return envOr("CA_LINKEDIN_CACHE_DIR", filepath.Join(home(), "dev/claude-social-media-skills/linkedin-stats/cache"))
}
func tiktokCacheDir() string {
	return envOr("CA_TIKTOK_CACHE_DIR", filepath.Join(home(), "dev/claude-social-media-skills/tiktok-stats/cache"))
}
func threadsCacheDir() string {
	return envOr("CA_THREADS_CACHE_DIR", filepath.Join(home(), "dev/claude-social-media-skills/threads-stats/cache"))
}

// Pending-task pointers for platforms not yet wired.
const (
	pendingTiktok          = "#373"
	pendingThreads         = "#375"
	pendingLinkedInPerPost = "#370"
	pendingBufferFormat    = "#371"
)

var tagRe = regexp.MustCompile(`\[(opus|lp|gh|bh):([^\]]+)\]`)

// ---------- JSON helpers ----------

func readJSONFile(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func num(m map[string]any, key string) float64 {
	if m == nil {
		return 0
	}
	if v, ok := m[key]; ok {
		if f, ok := v.(float64); ok {
			return f
		}
	}
	return 0
}

func str(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// loadVideos returns the videos array regardless of top-level shape
// ([...] | {videos:[...]} | {items:[...]}).
func loadVideos() []map[string]any {
	b, err := os.ReadFile(ytVideosPath())
	if err != nil {
		return nil
	}
	var raw any
	if json.Unmarshal(b, &raw) != nil {
		return nil
	}
	return coerceArray(raw)
}

func coerceArray(raw any) []map[string]any {
	switch t := raw.(type) {
	case []any:
		return toMaps(t)
	case map[string]any:
		if v, ok := t["videos"].([]any); ok {
			return toMaps(v)
		}
		if v, ok := t["items"].([]any); ok {
			return toMaps(v)
		}
	}
	return nil
}

func toMaps(arr []any) []map[string]any {
	out := make([]map[string]any, 0, len(arr))
	for _, e := range arr {
		if m, ok := e.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// newestSnapshot returns the lexicographically-newest snapshot-*.json in dir, or "".
func newestSnapshot(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if strings.HasPrefix(n, "snapshot-") && strings.HasSuffix(n, ".json") {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	return filepath.Join(dir, names[len(names)-1])
}

// allManifests returns every post-manifest path: files under opus_clips/ (and
// future scheduler dirs) whose clips[] has at least one entry with a
// scheduled_posts key. That contract excludes compose-phase staging files
// (*-proposed-copy.json, *-transcripts.json) that also carry a clips[] array.
func allManifests() []string {
	var candidates []string
	dirs := []string{
		opusManifestsDir(),
		filepath.Join(manifestsRoot(), "linkedin_pulses"),
		filepath.Join(manifestsRoot(), "medium_posts"),
		filepath.Join(manifestsRoot(), "substack_posts"),
	}
	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".json") {
				candidates = append(candidates, filepath.Join(d, e.Name()))
			}
		}
	}
	var out []string
	for _, f := range candidates {
		var doc map[string]any
		if readJSONFile(f, &doc) != nil {
			continue
		}
		clips, ok := doc["clips"].([]any)
		if !ok {
			continue
		}
		for _, c := range clips {
			if cm, ok := c.(map[string]any); ok {
				if _, has := cm["scheduled_posts"]; has {
					out = append(out, f)
					break
				}
			}
		}
	}
	return out
}

// ---------- Platform record construction ----------

// platformRecord is the normalized shape for every (clip × platform) entry.
// Engagement is non-nil only when a real match was found; the metric fields
// live INSIDE it (consumers always check .engagement != null then read
// .engagement.*). This fixes the bash version's inline-vs-wrapped split.
type platformRecord struct {
	Engagement     map[string]any `json:"engagement"`
	Pending        bool           `json:"pending,omitempty"`
	PendingTask    string         `json:"pending_task,omitempty"`
	Reason         string         `json:"reason,omitempty"`
	ScheduledAtUTC string         `json:"scheduled_at_utc,omitempty"`
}

func pendingRec(task string) platformRecord {
	return platformRecord{Engagement: nil, Pending: true, PendingTask: task}
}
func notAiredRec(at string) platformRecord {
	return platformRecord{Engagement: nil, Pending: true, Reason: "not_yet_aired", ScheduledAtUTC: at}
}
func noMatchRec() platformRecord {
	return platformRecord{Engagement: nil, Reason: "no_match"}
}

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

func parseISO(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// ytMatch: tag match by [opus:<clipID>] in description, then ±2h time-window
// fallback (video_type=short, tie-break on closest duration).
func ytMatch(clipID, schedAt string, targetDur float64) platformRecord {
	vids := loadVideos()
	if vids == nil {
		return noMatchRec()
	}
	tag := "[opus:" + clipID + "]"

	for _, v := range vids {
		if strings.Contains(str(v, "description"), tag) {
			return platformRecord{Engagement: ytEng(v, "tag")}
		}
	}

	// Time-window fallback.
	at, ok := parseISO(schedAt)
	if !ok {
		return noMatchRec()
	}
	type cand struct {
		v        map[string]any
		deltaSec float64
		durDelta float64
	}
	var cands []cand
	for _, v := range vids {
		if str(v, "video_type") != "short" {
			continue
		}
		pub, ok := parseISO(str(v, "published_at"))
		if !ok {
			continue
		}
		d := math.Abs(pub.Sub(at).Seconds())
		if d > 7200 {
			continue
		}
		cands = append(cands, cand{v, d, math.Abs(num(v, "duration_seconds") - targetDur)})
	}
	if len(cands) == 0 {
		return noMatchRec()
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].durDelta != cands[j].durDelta {
			return cands[i].durDelta < cands[j].durDelta
		}
		return cands[i].deltaSec < cands[j].deltaSec
	})
	return platformRecord{Engagement: ytEng(cands[0].v, "time")}
}

func ytEng(v map[string]any, joinMethod string) map[string]any {
	return map[string]any{
		"video_id":          str(v, "id"),
		"views":             num(v, "view_count"),
		"likes":             num(v, "like_count"),
		"comments":          num(v, "comment_count"),
		"subs_gained":       num(v, "subscribers_gained"),
		"estimated_revenue": num(v, "estimated_revenue"),
		"join_method":       joinMethod,
	}
}

// liPersonalMatch: read newest linkedin snapshot, match [opus:<clipID>] in
// profile.recent_posts[].{body,text}. Pending when snapshot/recent_posts absent.
func liPersonalMatch(clipID string) platformRecord {
	snap := newestSnapshot(linkedinCacheDir())
	if snap == "" {
		return pendingRec(pendingLinkedInPerPost)
	}
	var doc map[string]any
	if readJSONFile(snap, &doc) != nil {
		return pendingRec(pendingLinkedInPerPost)
	}
	profile, _ := doc["profile"].(map[string]any)
	if profile == nil {
		return pendingRec(pendingLinkedInPerPost)
	}
	posts, _ := profile["recent_posts"].([]any)
	if len(posts) == 0 {
		return pendingRec(pendingLinkedInPerPost)
	}
	tag := "[opus:" + clipID + "]"
	for _, p := range posts {
		pm, ok := p.(map[string]any)
		if !ok {
			continue
		}
		// match by source_tag.id OR raw [opus:] in body/text
		body := str(pm, "body") + str(pm, "text")
		matched := strings.Contains(body, tag)
		if st, ok := pm["source_tag"].(map[string]any); ok {
			if str(st, "scheme") == "opus" && str(st, "id") == clipID {
				matched = true
			}
		}
		if matched {
			urn := str(pm, "urn")
			if urn == "" {
				urn = str(pm, "post_urn")
			}
			if urn == "" {
				urn = str(pm, "id")
			}
			return platformRecord{Engagement: map[string]any{
				"urn":         urn,
				"reactions":   num(pm, "reactions"),
				"comments":    num(pm, "comments"),
				"reposts":     num(pm, "reposts"),
				"join_method": "tag",
			}}
		}
	}
	return noMatchRec()
}

// bufferMatch: Buffer snapshot doesn't yet carry per-post records keyed on
// scheduleId (gated on #371). Probe; pending until present.
func bufferMatch(scheduleID string) platformRecord {
	snap := newestSnapshot(bufferCacheDir())
	if snap == "" {
		return pendingRec(pendingBufferFormat)
	}
	b, err := os.ReadFile(snap)
	if err != nil {
		return pendingRec(pendingBufferFormat)
	}
	// Cheap probe: does the snapshot mention scheduleId anywhere?
	if !strings.Contains(string(b), "scheduleId") {
		return pendingRec(pendingBufferFormat)
	}
	var doc any
	if json.Unmarshal(b, &doc) != nil {
		return pendingRec(pendingBufferFormat)
	}
	if hit := findByScheduleID(doc, scheduleID); hit != nil {
		hit["join_method"] = "schedule_id"
		return platformRecord{Engagement: hit}
	}
	return noMatchRec()
}

// findByScheduleID walks an arbitrary JSON tree for an object whose
// "scheduleId" == target. Forward-compat for when #371 adds per-post records.
func findByScheduleID(node any, target string) map[string]any {
	switch t := node.(type) {
	case map[string]any:
		if sid, ok := t["scheduleId"].(string); ok && sid == target {
			return t
		}
		for _, v := range t {
			if hit := findByScheduleID(v, target); hit != nil {
				return hit
			}
		}
	case []any:
		for _, v := range t {
			if hit := findByScheduleID(v, target); hit != nil {
				return hit
			}
		}
	}
	return nil
}

func tiktokMatch() platformRecord  { return pendingRec(pendingTiktok) }
func threadsMatch() platformRecord { return pendingRec(pendingThreads) }

// platformLookup dispatches to the right matcher, then applies the
// not-yet-aired override (future schedule + no_match → not_aired).
func platformLookup(platform, clipID, schedAt string, dur float64, scheduleID string) platformRecord {
	var rec platformRecord
	switch platform {
	case "youtube_shorts":
		rec = ytMatch(clipID, schedAt, dur)
	case "linkedin_personal":
		rec = liPersonalMatch(clipID)
	case "facebook_page", "instagram_business", "linkedin_page":
		rec = bufferMatch(scheduleID)
	case "tiktok_business":
		rec = tiktokMatch()
	case "threads":
		rec = threadsMatch()
	default:
		rec = noMatchRec()
	}

	if schedAt != "" && rec.Reason == "no_match" {
		if at, ok := parseISO(schedAt); ok && at.After(time.Now().UTC()) {
			rec = notAiredRec(schedAt)
		}
	}
	return rec
}

// ---------- Source resolution ----------

type source struct {
	Type            string   `json:"type"`
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	URL             string   `json:"url"`
	PublishedAt     *string  `json:"published_at"`
	DurationSeconds *float64 `json:"duration_seconds"`
	ManifestPath    string   `json:"manifest_path,omitempty"`
}

func findSource(id string) *source {
	// 1) YouTube videos.json by .id
	for _, v := range loadVideos() {
		if str(v, "id") == id {
			typ := "long_form"
			if str(v, "video_type") == "short" {
				typ = "short"
			}
			s := &source{
				Type:  typ,
				ID:    id,
				Title: str(v, "title"),
				URL:   "https://www.youtube.com/watch?v=" + id,
			}
			if pa := str(v, "published_at"); pa != "" {
				s.PublishedAt = &pa
			}
			if d, ok := v["duration_seconds"].(float64); ok {
				s.DurationSeconds = &d
			}
			return s
		}
	}
	// 2) Manifests by .source_video.id
	for _, m := range allManifests() {
		var doc map[string]any
		if readJSONFile(m, &doc) != nil {
			continue
		}
		sv, _ := doc["source_video"].(map[string]any)
		if sv == nil || str(sv, "id") != id {
			continue
		}
		s := &source{Type: "long_form", ID: id, Title: str(sv, "title"), URL: str(sv, "url"), ManifestPath: m}
		return s
	}
	return nil
}

// ---------- The JOIN ----------

type joinResult struct {
	Source            *source        `json:"source"`
	Derivatives       []any          `json:"derivatives"`
	SourceEngagement  map[string]any `json:"source_engagement"`
	DerivedEngagement map[string]any `json:"derived_engagement"`
	AmplificationRow  any            `json:"amplification_ratio"`
	Pending           bool           `json:"pending,omitempty"`
	Reason            string         `json:"reason,omitempty"`
	Status            string         `json:"status,omitempty"`
}

func joinEngagement(sourceID string) joinResult {
	src := findSource(sourceID)
	if src == nil {
		return joinResult{
			Source:            &source{ID: sourceID, Type: "unknown"},
			Derivatives:       []any{},
			SourceEngagement:  nil,
			DerivedEngagement: map[string]any{"reach": 0.0, "reactions": 0.0, "comments": 0.0, "subs_gained": 0.0, "estimated_revenue": 0.0},
			AmplificationRow:  nil,
			Pending:           true,
			Reason:            "source_not_found",
		}
	}

	// Source engagement (YouTube-side).
	var srcEng map[string]any
	for _, v := range loadVideos() {
		if str(v, "id") == sourceID {
			srcEng = map[string]any{
				"views":             num(v, "view_count"),
				"likes":             num(v, "like_count"),
				"comments":          num(v, "comment_count"),
				"subs_gained":       num(v, "subscribers_gained"),
				"estimated_revenue": num(v, "estimated_revenue"),
			}
			break
		}
	}

	// Walk manifests pointing at this source.
	derivatives := []any{}
	derived := map[string]float64{"reach": 0, "reactions": 0, "comments": 0, "subs_gained": 0, "estimated_revenue": 0}

	for _, m := range allManifests() {
		var doc map[string]any
		if readJSONFile(m, &doc) != nil {
			continue
		}
		sv, _ := doc["source_video"].(map[string]any)
		if sv == nil || str(sv, "id") != sourceID {
			continue
		}
		clips, _ := doc["clips"].([]any)
		for _, c := range clips {
			clip, ok := c.(map[string]any)
			if !ok {
				continue
			}
			cid := str(clip, "clip_id")
			dur := num(clip, "duration_sec")
			platforms := map[string]any{}
			dReach, dReact, dComments := 0.0, 0.0, 0.0

			sps, _ := clip["scheduled_posts"].([]any)
			for _, spAny := range sps {
				sp, ok := spAny.(map[string]any)
				if !ok {
					continue
				}
				label := str(sp, "label")
				schedAt := str(sp, "scheduled_at_utc")
				scheduleID := ""
				if ar, ok := sp["api_response"].(map[string]any); ok {
					if data, ok := ar["data"].(map[string]any); ok {
						scheduleID = str(data, "scheduleId")
					}
				}
				pkey := labelToPlatform(label)
				rec := platformLookup(pkey, cid, schedAt, dur, scheduleID)
				platforms[pkey] = rec

				if rec.Engagement != nil {
					dReach += num(rec.Engagement, "views") + num(rec.Engagement, "impressions")
					dReact += num(rec.Engagement, "likes") + num(rec.Engagement, "reactions")
					dComments += num(rec.Engagement, "comments")
					derived["subs_gained"] += num(rec.Engagement, "subs_gained")
					derived["estimated_revenue"] += num(rec.Engagement, "estimated_revenue")
				}
			}

			derived["reach"] += dReach
			derived["reactions"] += dReact
			derived["comments"] += dComments

			deriv := map[string]any{
				"type":             "opus_clip",
				"clip_id":          cid,
				"title":            str(clip, "title"),
				"score":            clip["score"],
				"duration_seconds": dur,
				"platforms":        platforms,
				"derivative_engagement_total": map[string]any{
					"reach": dReach, "reactions": dReact, "comments": dComments,
				},
			}
			derivatives = append(derivatives, deriv)
		}
	}

	derivedOut := map[string]any{
		"reach": derived["reach"], "reactions": derived["reactions"], "comments": derived["comments"],
		"subs_gained": derived["subs_gained"], "estimated_revenue": derived["estimated_revenue"],
	}

	var amp any
	srcViews := num(srcEng, "views")
	if srcViews > 0 {
		amp = math.Round(derived["reach"]/srcViews*10) / 10
	}

	res := joinResult{
		Source: src, Derivatives: derivatives, SourceEngagement: srcEng,
		DerivedEngagement: derivedOut, AmplificationRow: amp,
	}
	if len(derivatives) == 0 {
		res.Status = "no_derivatives_yet"
	}
	return res
}

// ---------- ca_list_sources ----------

type sourceSummary struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	URL             string `json:"url"`
	ManifestPath    string `json:"manifest_path"`
	NDerivatives    int    `json:"n_derivatives"`
	NScheduledPosts int    `json:"n_scheduled_posts"`
}

func listSources() []sourceSummary {
	seen := map[string]bool{}
	var out []sourceSummary
	for _, m := range allManifests() {
		var doc map[string]any
		if readJSONFile(m, &doc) != nil {
			continue
		}
		sv, _ := doc["source_video"].(map[string]any)
		if sv == nil {
			continue
		}
		id := str(sv, "id")
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		clips, _ := doc["clips"].([]any)
		nPosts := 0
		for _, c := range clips {
			if cm, ok := c.(map[string]any); ok {
				if sp, ok := cm["scheduled_posts"].([]any); ok {
					nPosts += len(sp)
				}
			}
		}
		out = append(out, sourceSummary{
			ID: id, Title: str(sv, "title"), URL: str(sv, "url"),
			ManifestPath: m, NDerivatives: len(clips), NScheduledPosts: nPosts,
		})
	}
	if out == nil {
		out = []sourceSummary{}
	}
	return out
}

// ---------- Rendering ----------

func renderMarkdown(r joinResult) string {
	var b strings.Builder
	title := r.Source.Title
	if title == "" {
		title = r.Source.ID
	}
	fmt.Fprintf(&b, "# Source-content closed-loop: %s\n\n", title)
	fmt.Fprintf(&b, "- ID: `%s`  (%s)\n", r.Source.ID, r.Source.Type)
	fmt.Fprintf(&b, "- URL: %s\n", orNA(r.Source.URL))
	if r.Source.PublishedAt != nil {
		fmt.Fprintf(&b, "- Published: %s\n", *r.Source.PublishedAt)
	}
	b.WriteString("\n## Source engagement\n")
	if r.SourceEngagement != nil {
		fmt.Fprintf(&b, "- Views: %s | Likes: %s | Comments: %s | Subs gained: %s | Est. revenue: $%s\n",
			fnum(r.SourceEngagement, "views"), fnum(r.SourceEngagement, "likes"), fnum(r.SourceEngagement, "comments"),
			fnum(r.SourceEngagement, "subs_gained"), fnum(r.SourceEngagement, "estimated_revenue"))
	} else {
		b.WriteString("- (no source engagement available)\n")
	}
	b.WriteString("\n## Derived engagement (sum across derivatives)\n")
	fmt.Fprintf(&b, "- Reach: %s | Reactions: %s | Comments: %s | Subs gained: %s | Est. revenue: $%s\n",
		fnum(r.DerivedEngagement, "reach"), fnum(r.DerivedEngagement, "reactions"), fnum(r.DerivedEngagement, "comments"),
		fnum(r.DerivedEngagement, "subs_gained"), fnum(r.DerivedEngagement, "estimated_revenue"))
	ampStr := "n/a"
	if r.AmplificationRow != nil {
		ampStr = fmt.Sprintf("%v×", r.AmplificationRow)
	}
	fmt.Fprintf(&b, "- Amplification ratio: %s\n", ampStr)
	fmt.Fprintf(&b, "\n## Derivatives (%d)\n\n", len(r.Derivatives))
	for _, dAny := range r.Derivatives {
		d, _ := dAny.(map[string]any)
		fmt.Fprintf(&b, "### %s  (score %v, %vs)\n\n", strOrID(d), d["score"], d["duration_seconds"])
		plats, _ := d["platforms"].(map[string]any)
		keys := sortedKeys(plats)
		for _, k := range keys {
			rec, _ := plats[k].(platformRecord)
			b.WriteString("- **" + k + "**: " + renderPlatform(plats[k]) + "\n")
			_ = rec
		}
		b.WriteString("\n")
	}
	return b.String()
}

func renderPlatform(v any) string {
	// v is a platformRecord round-tripped through interface{} → re-marshal to read it
	bs, _ := json.Marshal(v)
	var rec platformRecord
	_ = json.Unmarshal(bs, &rec)
	if rec.Engagement != nil {
		jm := str(rec.Engagement, "join_method")
		parts := []string{}
		for _, k := range sortedKeys(rec.Engagement) {
			if k == "join_method" || k == "video_id" || k == "urn" || k == "post_id" {
				continue
			}
			parts = append(parts, fmt.Sprintf("%s=%v", k, rec.Engagement[k]))
		}
		return fmt.Sprintf("join=%s | %s", jm, strings.Join(parts, " "))
	}
	if rec.Reason == "no_match" {
		return "_no_match_"
	}
	if rec.Reason == "not_yet_aired" {
		return "_scheduled " + rec.ScheduledAtUTC + "_"
	}
	if rec.Pending {
		t := rec.PendingTask
		if t == "" {
			t = rec.Reason
		}
		return "_pending " + t + "_"
	}
	return "_unknown_"
}

func sortedKeys(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func strOrID(d map[string]any) string {
	if t := str(d, "title"); t != "" {
		return t
	}
	return str(d, "clip_id")
}

func orNA(s string) string {
	if s == "" {
		return "n/a"
	}
	return s
}

func fnum(m map[string]any, k string) string {
	return fmt.Sprintf("%v", trimFloat(num(m, k)))
}

func trimFloat(f float64) any {
	if f == math.Trunc(f) {
		return int64(f)
	}
	return f
}

// ---------- CLI ----------

func emit(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "sources":
		emit(listSources())
	case "join":
		emit(joinEngagement(parseSourceID(os.Args[2:])))
	case "report":
		fs := flag.NewFlagSet("report", flag.ExitOnError)
		sid := fs.String("source-id", "", "source content ID")
		format := fs.String("format", "md", "md|json")
		_ = fs.Parse(os.Args[2:])
		id := *sid
		if id == "" && fs.NArg() > 0 {
			id = fs.Arg(0)
		}
		res := joinEngagement(id)
		if *format == "json" {
			emit(res)
		} else {
			fmt.Print(renderMarkdown(res))
		}
	case "extract-tag":
		text := strings.Join(os.Args[2:], " ")
		m := tagRe.FindStringSubmatch(text)
		if m == nil {
			fmt.Println("null")
		} else {
			emit(map[string]string{"scheme": m[1], "id": m[2]})
		}
	default:
		usage()
	}
}

// parseSourceID accepts `<id>` or `--source-id <id>` (plus ignored --source-type/--manifest hints).
func parseSourceID(args []string) string {
	id := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--source-id":
			if i+1 < len(args) {
				id = args[i+1]
				i++
			}
		case "--source-type", "--manifest":
			i++ // skip value
		default:
			if !strings.HasPrefix(args[i], "--") && id == "" {
				id = args[i]
			}
		}
	}
	if id == "" {
		fmt.Fprintln(os.Stderr, "join: source-id required (positional or --source-id)")
		os.Exit(64)
	}
	return id
}

func usage() {
	fmt.Fprintln(os.Stderr, `content-attribution — closed-loop JOIN engine
usage:
  content-attribution sources
  content-attribution join     --source-id <id> [--source-type T] [--manifest P]
  content-attribution report   --source-id <id> [--format md|json]
  content-attribution extract-tag <text...>`)
	os.Exit(64)
}
