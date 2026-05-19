# Closed-loop unification plan

**Drafted 2026-05-19.** How to bring YouTube Shorts (and all other surfaces) under a single per-source-content closed-loop analysis in `/flywheel`, without merging the underlying data-collection skills.

## The asymmetry today

Two different worlds:

**Buffer-routed posts** (4 of 6 compose skills go this way) have a working closed-loop:
1. Compose with `format:<name>` Buffer tag
2. Buffer publishes
3. `buffer-stats` reads engagement + tagIds back via Buffer GraphQL
4. Engagement attributed per (channel, format)

**Non-Buffer-routed posts** (OpusClip native, future direct-publishing) have a closed-loop *being built*:
1. Compose with `[scheme:id]` in-text tag
2. OpusClip/etc. publishes
3. Per-platform stats skill scrapes engagement (linkedin-stats ✓, threads-stats ⏸️, tiktok-stats ⏸️, yt-analytics ✓)
4. Engagement *should be* attributed back to source content — **but the JOIN isn't built yet**

The user noticed today: **YouTube Shorts from OpusClip live in yt-analytics**, but yt-analytics doesn't know about OpusClip. The Short's description has `[opus:<clip_id>]`, but no current code does the JOIN. This plan fixes that, and unifies the whole loop.

## Don't merge the data-collection skills

The temptation is: "if `/flywheel` analyzes everything, put all the data collection IN `/flywheel`." That's wrong.

- `yt-analytics` is a substantial codebase (Go binary, YouTube API, title/retention/cohort analysis). It has its own life. Moving it into a markdown skill is regression.
- Each `*-stats` skill has platform-specific scraping concerns (gstack browser, OAuth, rate limits) that don't generalize.
- The "stats" skills are **fetchers**. Their job is platform-specific data acquisition. They should stay platform-specific.

What needs to unify is **the analysis layer** — the JOIN that maps a source piece of content to its derivatives' engagement across every platform.

## The proposed unified architecture

```
                            ┌────────────────────────────────────────────────┐
                            │       _shared/content-attribution/             │
                            │  (the JOIN engine — pure jq/awk over snapshots) │
                            └────────────────┬───────────────────────────────┘
                                             │ consumed by
                                             ↓
                                        ┌───────────┐
                                        │ /flywheel │── per-source-content closed-loop report
                                        └─────┬─────┘
                                              │ also consumed by
                                              ↓
                                     ┌────────────────────────┐
                                     │ /opus-clips-performance │
                                     │ (per-clip rollups)      │
                                     └─────────────────────────┘

   ┌────────────┐  ┌─────────────┐  ┌────────────────┐  ┌────────────┐  ┌───────────────┐
   │ yt-        │  │ buffer-stats│  │ linkedin-stats │  │ tiktok-    │  │ threads-stats │
   │ analytics  │  │   (Buffer   │  │  (LinkedIn     │  │ stats      │  │  (Meta API)   │
   │ (YouTube)  │  │  Insights + │  │   personal +   │  │ (TikTok    │  │               │
   │            │  │   Analyze)  │  │   newsletter)  │  │  API)      │  │               │
   └─────┬──────┘  └──────┬──────┘  └────────┬───────┘  └─────┬──────┘  └──────┬────────┘
         │                │                  │                │                │
         │ data/videos    │ cache/snap-      │ cache/snap-    │ cache/snap-    │ cache/snap-
         │ .json          │ shot-*.json      │ shot-*.json    │ shot-*.json    │ shot-*.json
         │                │                  │                │                │
         └────────────────┴──────────────────┴────────────────┴────────────────┘
                                             ▲
                                             │ ALL data-collection skills
                                             │ write to their own snapshot dirs.
                                             │ NONE talk to each other.
                                             │
   ┌──────────────────────────┐
   │ ~/dev/youtube_analytics/  │
   │ data/opus_clips/*.json    │── post-manifests (publication ledger,
   │ (post-manifests)          │   written by /opus-clips at schedule time)
   └──────────────────────────┘
                                             ▲
                                             │
                            join keys (in priority order):
                            1. [scheme:id] in-text tag
                            2. Buffer format:<name> tag
                            3. ±2h time-window
                            4. ?utm_content URL param (future)
```

**Three layers, clear contracts:**

1. **Data-collection layer** (existing + planned skills) — each writes its own snapshot under a known path. No skill reads from another skill's cache.
2. **JOIN layer** (new — `_shared/content-attribution/`) — reads snapshots + manifests, correlates by tag/format/time, emits per-source-content records.
3. **Analysis layer** (`/flywheel` + `/opus-clips-performance`) — consumes the JOIN output, applies user-specific judgment (priority weighting, ROI bucketing), renders the report.

## YouTube Shorts from OpusClip — concrete example

A long-form essay (`uEposKmbFvY` = "How to Scale Without the Slop") produces 23 clips via OpusClip. Each clip gets fanned out to 6 channels. One of those channels is **YouTube** — the clip becomes a YouTube Short.

**Before this plan**: yt-analytics sees a new Short with title "Your next 50% productivity gain isn't a new AI tool" and description containing `[opus:La4Wghg6IX]`. No code joins this back to the source long-form.

**After this plan**:

1. `_shared/content-attribution/` walks `~/dev/youtube_analytics/data/opus_clips/P3051823ab0w.json` and finds `clips[].clip_id = "La4Wghg6IX"`.
2. It scans `~/dev/youtube_analytics/data/videos.json` for any Short whose `description` matches `\[opus:La4Wghg6IX\]`. Finds the Short, captures `views, likes, comments, subscribers_gained, estimated_revenue`.
3. It does the same for the LinkedIn-personal scrape (joins on `source_tag.id == "La4Wghg6IX"`), the Buffer-routed channels (joins on `scheduleId` against the manifest), etc.
4. Output record:

```json
{
  "source": {
    "type": "long_form",
    "id": "uEposKmbFvY",
    "title": "How to Scale Without the Slop",
    "url": "https://www.youtube.com/watch?v=uEposKmbFvY",
    "published_at": "2026-05-15T20:00:38Z",
    "duration_seconds": 939
  },
  "derivatives": [
    {
      "type": "opus_clip",
      "clip_id": "La4Wghg6IX",
      "title": "Your next 50% productivity gain isn't a new AI tool",
      "score": 99,
      "duration_seconds": 28,
      "platforms": {
        "youtube_shorts": {"video_id": "abc123", "views": 1240, "likes": 18, "comments": 3, "subs_gained": 4, "join_method": "tag"},
        "linkedin_personal": {"urn": "urn:li:activity:7462532108851462144", "reactions": 1, "comments": 0, "reposts": 0, "rel": "3h", "join_method": "tag"},
        "instagram_business": {"post_id": "...", "impressions": 157, "likes": 8, "comments": 0, "join_method": "schedule_id"},
        "facebook_page": {"post_id": "...", "reactions": 2, "comments": 0, "join_method": "schedule_id"},
        "linkedin_page": {"post_id": "...", "impressions": 13, "reactions": 1, "join_method": "schedule_id"},
        "tiktok_business": {"engagement": null, "pending_task": "#373"}
      },
      "derivative_engagement_total": {"reach": 1410, "reactions": 30, "comments": 3}
    }
    // ... 22 more clips
  ],
  "source_engagement": {"views": 425, "likes": 12, "comments": 1, "subs_gained": 0, "estimated_revenue": 0.05},
  "derived_engagement": {"reach": 18234, "reactions": 540, "comments": 41, "subs_gained": 19, "estimated_revenue": 1.83},
  "amplification_ratio": 42.9
}
```

That's the closed loop. **A long-form essay's true ROI is now the source video's metrics PLUS every derivative clip's metrics across every platform.**

## What changes vs what stays

**Stays the same:**
- All existing `*-stats` skills (yt-analytics, buffer-stats, linkedin-stats, tiktok-stats, threads-stats) — they fetch, they don't analyze
- The `format:<name>` Buffer tag system — still works for Buffer-routed posts
- The `[scheme:id]` in-text tag convention from `_shared/post-manifest/` — still works for non-Buffer posts
- `/buffer-stats` Phase 3.5 tagId JOIN (#371) — stays in buffer-stats; the result feeds the unified JOIN layer

**Adds:**
- `_shared/content-attribution/` — new shared module, the JOIN engine (#381)
- `/flywheel` Phase 4.55 extension — calls the JOIN engine, emits the unified report (#380)
- `/opus-clips-performance` — refactors to use the JOIN engine (today's scaffold has its own join logic; this consolidates)

**Removes / consolidates:**
- Nothing removed. The architecture is additive.

## Execution sequence (no parallel work this time — dependencies)

1. **#371 buffer-stats tagId JOIN** — DONE 2026-05-19. Real `format:<name>` per Buffer post.
2. **#370 linkedin-stats Phase 3b** — DONE 2026-05-19. Per-post engagement for LinkedIn personal.
3. **#377 buffer-stats extend to all 6 channels (Insights)** — needs to land before the JOIN engine can join all surfaces uniformly. ~2-3 hours.
4. **#381 _shared/content-attribution/ extract JOIN engine** — the core. ~4-5 hours.
5. **#380 /flywheel Phase 4.55 unified per-source-content JOIN** — consumes the engine. ~6-8 hours.
6. **#378 ARCHITECTURE.md update** — document the unified loop. Land before or after #380; either works. ~1-2 hours.
7. **Parallel / opportunistic:** #372 opus-clips-performance refactor to use the JOIN engine; #379 dead Threads channel investigation.

Total: **~15-20 hours** of focused work to reach a working unified closed-loop. Most of it is the JOIN engine + flywheel integration.

## What this DOESN'T solve

- **OAuth gates** — tiktok-stats and threads-stats need user-driven setup before their snapshots exist. The JOIN engine handles missing snapshots gracefully (emits `engagement: null, pending_task: "<id>"`).
- **Per-post backfill** — historical posts (pre-tag-system) won't retroactively get attribution. We accept that loss; new posts inherit the loop.
- **Buffer Insights granularity** — Insights gives 30-day aggregates per channel, not per-post. The JOIN engine treats Insights as a "denominator" source (was the channel alive?) and per-post engagement as a separate read.

## Open questions worth surfacing

1. **Where should `_shared/content-attribution/` live?** — `_shared/` makes sense (matches voice-corpus, post-manifest, buffer-post-prep). But it's the most consequential shared module so far; consider if it deserves its own top-level dir.
2. **Should the JOIN engine be a Go binary** (like voice-corpus, buffer-post-prep) or stay as bash/jq? — Bash is fine for current scale. If we add Markov-style cohort analysis or larger-N joins, Go.
3. **Is `/opus-clips-performance` redundant after #380 + #381?** — Maybe. If `/flywheel` Phase 4.55 produces the per-clip rollup, the standalone skill becomes a thin wrapper. Could deprecate later, or keep as a faster ad-hoc query path.
