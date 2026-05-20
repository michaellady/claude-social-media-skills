# content-attribution

The **JOIN engine** for the unified closed-loop. Walks post-manifests and per-platform stats snapshots and correlates a source piece of content (long-form video, newsletter, GitHub PR) to its derivatives' engagement across every platform.

Pure transport per [PRIMITIVE-TEST.md](../../PRIMITIVE-TEST.md). Judgment about which sources matter, how to weight platforms, or how to bucket ROI belongs in caller skills (`/flywheel`, `/opus-clips-performance`). This module just performs the JOIN and emits a unified record.

Full architectural context: [CLOSED-LOOP-UNIFICATION-PLAN.md](../../CLOSED-LOOP-UNIFICATION-PLAN.md).

## When to use this (and when NOT to)

**Use this when** you have a source content ID (YouTube video ID, newsletter slug, GitHub PR ref) and you need a unified view of every derivative's engagement across platforms — the per-source-content closed loop.

**Do NOT use this for data collection.** This module is the *analysis layer*. It reads but never writes platform snapshots. If a snapshot is missing per-post engagement, the fix lives in the corresponding `*-stats` skill, not here.

| Want… | Use… |
|---|---|
| Per-source-content closed-loop record (the JOIN) | **This module.** |
| Buffer per-(channel, format) engagement | `buffer-stats` |
| YouTube long-form / Shorts metrics | `yt-analytics` |
| LinkedIn personal / page / newsletter metrics | `linkedin-stats` |
| TikTok / Threads metrics | `tiktok-stats` / `threads-stats` (when wired) |
| Publication ledger for non-Buffer-routed posts | [`_shared/post-manifest/`](../post-manifest/README.md) |

## JOIN keys (priority order)

When correlating a derivative post on platform X back to a source content, try these in order:

1. **`[scheme:id]` in-text tag** — most reliable. Travels with the post body across every platform. Schemes from [`_shared/post-manifest/`](../post-manifest/README.md#in-text-tag-convention): `opus:<clip_id>`, `lp:<pulse_slug>`, `gh:<pr_or_sha>`, `bh:<beehiiv_slug>`. Exact regex match wins.
2. **Buffer `format:<name>` tag** — for Buffer-routed posts. `buffer-stats` Phase 3.5 (#371) records per-post format. The JOIN engine maps `format:` back to source content via the manifest's `format_tag` annotation (when present).
3. **`scheduleId` against manifest** — for non-Buffer-routed posts whose scheduler returned an ID we recorded. Match `scheduled_posts[].api_response.data.scheduleId` to a stats-snapshot post-ID. Lossy across platforms but exact within one.
4. **±2h time-window match** — fallback. Useful when the user manually re-titled a YouTube Short and stripped the `[opus:]` footer in Studio. Tie-break on closest duration.
5. **`?utm_content` URL param** — future. Not implemented today.

The `join_method` field on every derivative record records which key matched (`"tag"`, `"format"`, `"schedule_id"`, `"time"`, `"utm"`), so consumers can weight confidence.

## Source snapshot paths

The JOIN engine reads from these (NEVER writes):

| Path | What | Status |
|---|---|---|
| `~/dev/youtube_analytics/data/videos.json` | YouTube long-form + Shorts engagement (per-video) | live |
| `~/dev/claude-social-media-skills/buffer-stats/cache/snapshot-*.json` | Buffer-routed engagement, channel ROI, format_engagement | live (per-format gated on #371) |
| `~/dev/claude-social-media-skills/linkedin-stats/cache/snapshot-*.json` | LinkedIn newsletter + company + personal per-post (`profile.recent_posts[]` post-#370 Phase 3b) | live (per-post gated on #370 Phase 3b production run) |
| `~/dev/youtube_analytics/data/opus_clips/*.json` | Post-manifests (publication ledger) | live |
| `~/dev/youtube_analytics/data/<scheduler>/*.json` | Post-manifests from other schedulers (future) | future |
| `~/dev/claude-social-media-skills/tiktok-stats/cache/snapshot-*.json` | TikTok per-post engagement | **pending #373** — handled gracefully if missing |
| `~/dev/claude-social-media-skills/threads-stats/cache/snapshot-*.json` | Threads per-post engagement | **pending #375** — handled gracefully if missing |

Always reads the **newest** snapshot per platform (`snapshot-*.json` sorted lexicographically — ISO-8601 dates sort correctly).

## Output shape

The JOIN emits one unified record per source content. Shape is from CLOSED-LOOP-UNIFICATION-PLAN.md's worked example:

```jsonc
{
  "source": {
    "type": "long_form",                                    // "long_form" | "newsletter" | "pull_request" | …
    "id": "uEposKmbFvY",
    "title": "How to Scale Without the Slop",
    "url": "https://www.youtube.com/watch?v=uEposKmbFvY",
    "published_at": "2026-05-15T20:00:38Z",
    "duration_seconds": 939
  },
  "derivatives": [
    {
      "type": "opus_clip",                                  // "opus_clip" | "buffer_post" | "linkedin_pulse" | …
      "clip_id": "La4Wghg6IX",
      "title": "Your next 50% productivity gain isn't a new AI tool",
      "score": 99,
      "duration_seconds": 28,
      "platforms": {
        "youtube_shorts":     {"video_id": "abc123", "views": 1240, "likes": 18, "comments": 3, "subs_gained": 4, "join_method": "tag"},
        "linkedin_personal":  {"urn": "urn:li:activity:…", "reactions": 1, "comments": 0, "reposts": 0, "join_method": "tag"},
        "instagram_business": {"post_id": "…", "impressions": 157, "likes": 8, "comments": 0, "join_method": "schedule_id"},
        "facebook_page":      {"post_id": "…", "reactions": 2, "comments": 0, "join_method": "schedule_id"},
        "linkedin_page":      {"post_id": "…", "impressions": 13, "reactions": 1, "join_method": "schedule_id"},
        "tiktok_business":    {"engagement": null, "pending": true, "pending_task": "#373"}
      },
      "derivative_engagement_total": {"reach": 1410, "reactions": 30, "comments": 3}
    }
    // … more derivatives
  ],
  "source_engagement": {"views": 425, "likes": 12, "comments": 1, "subs_gained": 0, "estimated_revenue": 0.05},
  "derived_engagement": {"reach": 18234, "reactions": 540, "comments": 41, "subs_gained": 19, "estimated_revenue": 1.83},
  "amplification_ratio": 42.9
}
```

A platform record may be:
- **Live data** — `{video_id|post_id|urn: "…", <metrics…>, join_method: "tag|format|schedule_id|time|utm"}`
- **Pending** — `{"engagement": null, "pending": true, "pending_task": "#NNN"}` when the underlying stats skill isn't wired yet, the snapshot file is missing, or the snapshot lacks per-post engagement
- **Scheduled-not-published** — `{"engagement": null, "scheduled_at_utc": "…", "pending": true, "reason": "not_yet_aired"}` for manifest entries whose `scheduled_at_utc` is still in the future
- **No match** — `{"engagement": null, "reason": "no_match"}` when the manifest expected this derivative but no JOIN key matched

`amplification_ratio` is `derived_engagement.reach / max(source_engagement.views, 1)`. Null when source has no view denominator.

## Helper functions (sourceable bash)

```bash
source ~/dev/claude-social-media-skills/_shared/content-attribution/content_attribution.sh
```

| Function | Purpose |
|---|---|
| `ca_find_source <source_id>` | Locate a source by ID across the snapshot universe; returns its metadata or empty. |
| `ca_join_engagement <source_id>` | The JOIN. Emit a unified JSON record per the output shape above. |
| `ca_render_report <source_id> [--format md\|json]` | Pretty-print the unified record. Default `md`. |
| `ca_list_sources` | Enumerate all source content with at least one derivative across snapshots. Drives `/flywheel`'s loop. |
| `ca_extract_tag <text>` | Utility: pull `[scheme:id]` from a body of text. Returns `{scheme, id}` or `null`. |

Thin jq wrappers. Same Primitive Test discipline as [`_shared/post-manifest/`](../post-manifest/post_manifest.sh): no judgment, just the shape contract.

## Consumers

- **`/flywheel` Phase 4.55** — calls `ca_list_sources` then `ca_join_engagement` per source to render the per-source-content closed-loop report (task #380).
- **`/opus-clips-performance`** — currently has its own join logic; refactor target to consume `ca_join_engagement` for any project whose source video ID is known (task #372).
- **Future per-newsletter reports** — e.g., a `/newsletter-loop` skill that takes a beehiiv slug and renders cross-platform reach.

## What's NOT in this module

- **Data collection.** The fetchers (`yt-analytics`, `buffer-stats`, `linkedin-stats`, future `tiktok-stats`/`threads-stats`) own all platform-specific scraping/OAuth/rate-limit concerns. This module only reads their cached snapshots.
- **ROI scoring / bucketing.** That's a caller (`/flywheel`) decision — different consumers may weight platforms differently.
- **Backfill of historical posts.** Pre-tag-system posts won't get attribution. We accept that loss; new posts inherit the loop.
- **Write side.** The JOIN engine is read-only over snapshots. It does not mutate manifests or platform caches.

## Versioning

No `schema_version` field today. The output shape is append-only; new fields fine, renames are breaking. If a breaking change becomes necessary, add `"schema_version": "2"` at the top level and have `/flywheel` + `/opus-clips-performance` branch on it.
