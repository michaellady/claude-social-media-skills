---
name: opus-clips-performance
description: Use when user wants engagement metrics for clips scheduled via OpusClip — "how did my clips do", "opus clip performance", "clip engagement report", "which clips landed", "per-clip stats", "opus-clips report". Walks post-manifests under `~/dev/youtube_analytics/data/opus_clips/` and joins each scheduled post against per-platform native analytics. Scaffolded 2026-05-19 (task #372); YouTube join is wired, other platforms gated on tasks #370, #371, #373.
user_invocable: true
---

# opus-clips-performance

Per-clip engagement report for OpusClip-scheduled posts. Reads the closed-loop **post-manifests** written by `/opus-clips` (see [`_shared/post-manifest/`](../_shared/post-manifest/README.md)) and joins each `scheduled_posts[]` entry against the matching platform's native analytics, emitting:

1. A markdown `report-<date>.md` next to the manifest — one section per clip with score, duration, scheduled fan-out, and fetched engagement.
2. An additive update to the manifest: each `scheduled_posts[]` entry gains an `engagement` block (or `engagement: null` + `pending_task` pointer for platforms not yet wired).

Why this exists: `/opus-clips` writes a publication ledger but no engagement data ("What's NOT in the manifest" — see post-manifest README). This skill is the fetcher that fills the gap. `/flywheel` Phase 4.55 will read these reports to credit clip output toward Priority 1 throughput.

## Usage

`/opus-clips-performance` — process every manifest under `~/dev/youtube_analytics/data/opus_clips/*.json` whose newest scheduled post is older than 24 h (gives platforms time to ingest analytics).
`/opus-clips-performance <project_id>` — process one manifest (`P3051823ab0w` or full path).
`/opus-clips-performance --since YYYY-MM-DD` — only manifests with scheduled posts on/after this date.
`/opus-clips-performance --dry-run` — render the report to stdout, don't mutate manifests or write files.
`/opus-clips-performance --platforms youtube` — restrict to one platform (today only `youtube` is wired; the flag is for forward-compat with #370/#371/#373).

## 🟢 Happy Path

For one fully-fanned-out OpusClip project (23 clips × 6 channels = 138 scheduled posts, the live `P3051823ab0w` shape) once 24 h have elapsed. ~30 sec wall-clock today (only the YouTube join hits disk; other platforms short-circuit).

**Phase 1 — Discover manifests.** Walk `~/dev/youtube_analytics/data/*/` for JSON files matching the post-manifest schema. The signature shape is `clips[].scheduled_posts[].api_response.data.scheduleId` — use `pm_count_scheduled` as a cheap probe (returns 0 for non-manifests).

**Phase 2 — YouTube join (the only platform wired today).** For each `scheduled_posts[]` whose `label` contains `YOUTUBE`, correlate against `~/dev/youtube_analytics/data/videos.json`:
1. **Primary:** scan video descriptions for `[opus:<clip_id>]`. Exact match wins.
2. **Fallback:** time-window match — YouTube `published_at` within ±2 h of manifest's `scheduled_at_utc`. If multiple candidates, prefer the one whose duration is closest to the clip's `duration_sec`.
3. Emit `engagement: { source: "youtube", join_method: "tag" | "time", views, likes, comments, subscribers_gained, estimated_revenue, video_id, fetched_at }`.

**Phase 3 — Other platforms (gated).** For `FACEBOOK_PAGE`, `INSTAGRAM_BUSINESS`, `LINKEDIN` (page + personal), `TIKTOK_BUSINESS`: do **not** fetch. Emit `engagement: null, pending_task: "#370"` (FB/IG), `"#371"` (LinkedIn), `"#373"` (TikTok). See **Out of scope** below for the dependency map.

**Phase 4 — Render report.** One markdown section per clip, sorted by score desc. Include: clip_id, title, score, duration, scheduled-posts table (label, scheduled_at_utc, engagement summary or `pending #NNN`). Write to `report-<YYYY-MM-DD>.md` next to the manifest.

**Phase 5 — Persist back to manifest.** Atomic write (tmp + mv, same pattern as `_pm_atomic_write`): add the `engagement` (or `engagement: null` + `pending_task`) field to each `scheduled_posts[]` entry. Additive only — never remove or rename existing fields (schema versioning rule from post-manifest README).

## Phases

### Phase 1 — Discover manifests

```bash
source ~/dev/claude-social-media-skills/_shared/post-manifest/post_manifest.sh

DATA_ROOT=~/dev/youtube_analytics/data
MANIFESTS=()
for f in "$DATA_ROOT"/*/*.json; do
  COUNT=$(pm_count_scheduled "$f" 2>/dev/null)
  [ "${COUNT:-0}" -gt 0 ] && MANIFESTS+=("$f")
done
```

If `<project_id>` was passed, resolve it to a single manifest path under `opus_clips/` and skip the walk.

For the `--since` filter, drop manifests whose newest `scheduled_at_utc` is older than the cutoff:

```bash
NEWEST=$(jq -r '[.clips[].scheduled_posts[].scheduled_at_utc] | max' "$f")
```

### Phase 2 — YouTube join

```bash
VIDEOS=~/dev/youtube_analytics/data/videos.json

# Build a clip_id -> youtube_video lookup by scanning descriptions for [opus:<clip_id>]
yt_by_tag() {
  local clip_id="$1"
  jq --arg cid "$clip_id" '
    (if type=="array" then . else (.videos // .items) end)
    | map(select(.description | test("\\[opus:" + $cid + "\\]"))) | .[0] // empty
  ' "$VIDEOS"
}

# Fallback: time-window match within ±2h of scheduled_at_utc, tie-break on closest duration
yt_by_time() {
  local scheduled_at="$1" target_dur="$2"
  jq --arg at "$scheduled_at" --argjson dur "$target_dur" '
    (if type=="array" then . else (.videos // .items) end)
    | map(select(.video_type == "short"))
    | map(. + {
        delta_sec: ((.published_at | fromdateiso8601) - ($at | fromdateiso8601) | fabs),
        dur_delta: ((.duration_seconds // 0) - $dur | fabs)
      })
    | map(select(.delta_sec <= 7200))
    | sort_by(.dur_delta, .delta_sec) | .[0] // empty
  ' "$VIDEOS"
}
```

For each `(clip × YOUTUBE post)`, try tag first, then time. Extract `{view_count, like_count, comment_count, subscribers_gained, estimated_revenue, id}` into an `engagement` block. If neither method matches, emit `engagement: null, pending_reason: "no_youtube_match"`.

**Why the tag-first / time-fallback split:** OpusClip's `[opus:<clip_id>]` footer is grep-able from any platform's native search — when present it's a clean exact join. But at manifest-write time we don't yet have the YouTube video ID (Buffer/OpusClip schedule first, YouTube assigns the ID on publish), so the manifest can't carry it. The time-window fallback exists for the case where the user manually re-titled the YouTube short and stripped the description footer in the studio editor.

### Phase 3 — Other platforms (pending)

Per the constraints, every non-YouTube label gets:

```json
{ "engagement": null, "pending_task": "#370" }
```

| Label prefix in manifest | Pending task | Why blocked |
|---|---|---|
| `FACEBOOK_PAGE` | **#370** | Need Meta Graph API page-level insights wiring (or scrape of FB Creator Studio). |
| `INSTAGRAM_BUSINESS` | **#370** | Same Meta auth path as FB — bundled under one task. |
| `LINKEDIN` (page + personal) | **#371** | `linkedin-stats` is per-channel snapshot today, not per-post; needs the per-post scrape from `linkedin-stats/SPEC-per-post-scrape.md`. |
| `TIKTOK_BUSINESS` | **#373** | TikTok Creator API access pending; manual export workaround not in scope here. |

When those tasks land, replace the `pending_task` stub with a real fetcher and re-run the skill — the manifest's `scheduleId` / `postId` from `api_response.data` is the join key those fetchers will use.

### Phase 4 — Render report

```
# OpusClip performance — <project_id>
Source: <source_video.title> (<source_video.id>)
Scheduled: <count> posts across <unique_label_count> channels
Window: <earliest scheduled_at_utc> → <latest>
Generated: <now>

## Top clips by score

### [99] La4Wghg6IX — "Your next 50% productivity gain..." (28s)
| Label                              | Scheduled (UTC)       | Engagement                                  |
|------------------------------------|-----------------------|---------------------------------------------|
| YOUTUBE Enterprise Vibe Code       | 2026-05-19T16:00:00Z  | 1,243 views · 87 likes · 12 cm · +4 subs    |
| FACEBOOK_PAGE Enterprise Vibe Code | 2026-05-19T16:00:00Z  | pending (#370)                              |
| INSTAGRAM_BUSINESS Enterprise...   | 2026-05-19T16:00:00Z  | pending (#370)                              |
| LINKEDIN Enterprise Vibe Code      | 2026-05-19T16:00:00Z  | pending (#371)                              |
| LINKEDIN Mike Lady                 | 2026-05-19T16:00:00Z  | pending (#371) [hasConflict=true]           |
| TIKTOK_BUSINESS mikelady           | 2026-05-19T16:00:00Z  | pending (#373)                              |

(repeat per clip, descending score)

## Aggregate (YouTube only — other platforms pending)
Total views: N · Total likes: N · Total comments: N · Subs gained: N · Est. revenue: $N.NN
```

Write to `~/dev/youtube_analytics/data/opus_clips/report-<YYYY-MM-DD>-<project_id>.md`.

### Phase 5 — Persist back to manifest

Use the `_pm_atomic_write` pattern from `post_manifest.sh` (tmp + mv). Update only the `engagement` and optional `pending_task` / `pending_reason` keys on each `scheduled_posts[]` entry. Do not touch any other field.

```bash
jq --argjson updates "$ENGAGEMENT_PATCH_JSON" '
  .clips |= map(
    .scheduled_posts |= map(
      . as $p
      | ($updates[$p.api_response.data.scheduleId] // null) as $u
      | if $u then . + {engagement: $u.engagement, pending_task: $u.pending_task} else . end
    )
  )
' "$MANIFEST" > "$MANIFEST.tmp" && mv "$MANIFEST.tmp" "$MANIFEST"
```

Key the patch by `scheduleId` (globally unique within a manifest).

## Out of scope

- **Fetching FB / IG / LinkedIn / TikTok engagement.** Gated on tasks **#370** (Meta Graph API for FB Pages + IG Business), **#371** (LinkedIn per-post scrape — see `linkedin-stats/SPEC-per-post-scrape.md`), **#373** (TikTok Creator API). When those land, this skill's Phase 3 expands; the manifest already carries the per-platform `scheduleId` / `postId` they'll need.
- **Mutating the source content.** This is read-side only; no re-renders or re-scheduling. For that, see `/opus-clips`.
- **Aggregating across projects.** One manifest = one report. Cross-project rollups belong in `/flywheel`.
- **Recomputing OpusClip's `score`.** That field is from OpusClip's viral-score model at curation time and is preserved verbatim.
- **Backfilling missing `[opus:<clip_id>]` footers.** If a user edited a post and stripped the tag, this skill falls back to time-matching; it does not try to re-write the post.

## Schema gaps noticed in the post-manifest

While scaffolding this consumer, three gaps surfaced that the manifest schema could close in a future minor revision (additive, non-breaking):

1. **No `source_video.duration_seconds`.** The YouTube time-fallback wants to bias toward clips whose duration is close to the manifest's `duration_sec`; having the source duration would let us reject "the long-form itself" as a false-positive match.
2. **No `clips[].published_url` placeholder.** Once a platform publishes, the eventual platform URL would be the cleanest join key. Today we re-derive it from the `[opus:<clip_id>]` tag scan. An additive `engagement.published_url` field (which this skill would populate) closes the loop without a schema bump.
3. **No `schema_version`.** The README acknowledges this. As soon as multiple consumers (`/flywheel` + this skill) read the manifest, divergent expectations become a real risk. Worth adding `"schema_version": "1"` at the top level even before any breaking change, just to give consumers a check-and-warn anchor.

None of these block the current scaffold — they're future-work notes.

## Related skills

- **`/opus-clips`** — upstream; writes the manifests this skill reads.
- **`_shared/post-manifest/`** — the shape contract; helper functions (`pm_count_scheduled`, `pm_schedule_ids`, `pm_find_clip`) are sourced here.
- **`/flywheel`** — downstream; Phase 4.55 will read this skill's reports to credit clip output toward Priority 1 (long-form throughput, since each clip is a derivative of a long-form essay).
- **`/yt-analytics`** — owns `~/dev/youtube_analytics/data/videos.json`; refresh it (`go run . fetch-analytics --all`) before running this skill if YouTube data is stale.
- **`/linkedin-stats`** — when task **#371** lands, its `SPEC-per-post-scrape.md` becomes Phase 3's LinkedIn fetcher.
- **`/buffer-stats`** — does NOT cover these posts (they bypass Buffer); that's the entire reason post-manifests exist.
