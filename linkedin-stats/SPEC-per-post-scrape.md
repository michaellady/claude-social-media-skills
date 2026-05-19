# SPEC — linkedin-stats per-post engagement scrape (Phase 3b)

**Status:** Design — not yet implemented (2026-05-19).
**Motivation:** see [PATTERNS.md § Closed-loop post manifest](../PATTERNS.md) — the user's biggest-reach LinkedIn surface (LinkedIn Mike Lady personal, 2,115 followers as of 2026-05-18) has **zero per-post engagement data**. The current Phase 3 captures only the aggregate follower count. This spec adds the actual per-post scrape that closes the gap.

This is the **#1 priority closed-loop gap** in the repo:
- The user's deal-relevant audience lives on LinkedIn personal (Dan Uyemura attribution, Sign Lab / NetDocuments closes, etc.).
- 23 OpusClip posts × 5 future-content cycles will go through that channel.
- Every other channel the user posts to either has per-post engagement (Buffer Analyze, yt-analytics) or is small enough to deprioritize.

## What's broken today

`linkedin-stats/SKILL.md` Phase 3 mentions per-post scraping but doesn't implement it:

> Top posts: from Creator analytics posts view (already loaded in Phase 2 if we're doing the full report) — or navigate to `https://www.linkedin.com/in/<handle>/recent-activity/all/` and pull the first N entries with engagement counts.

In practice, snapshots emit `profile.top_posts: []` (empty array). `/buffer-stats` reports `has_per_post_engagement: false` for LinkedIn personal. `/flywheel` Priority 3 can only render aggregate newsletter subs + follower deltas — no signal on which posts moved them.

## What this spec adds

A new **Phase 3b** that scrapes `linkedin.com/in/mikelady/recent-activity/all/` for the most recent N posts and captures structured per-post engagement. Output lands in the existing snapshot JSON under `profile.recent_posts[]` (new field) so downstream consumers (`/buffer-stats`, `/flywheel`, future post-manifest fetcher) can join on it.

## Phase 3b — implementation

### URL + auth

```
https://www.linkedin.com/in/{handle}/recent-activity/all/
```

Where `{handle}` is `config.profile.handle` (already exists in `linkedin-stats/config.json`). Auth is the same gstack browser session Phases 1-3 already establish — no new credentials.

### DOM scrape strategy

LinkedIn renders the activity feed as a virtualized list of `<div class="feed-shared-update-v2">` cards (selector subject to LinkedIn renaming — fall back to `[data-id^="urn:li:activity:"]` which is more stable).

Each card surfaces (verified by inspecting `/in/mikelady/recent-activity/all/` 2026-05-18):

| Field | DOM landmark | Notes |
|---|---|---|
| `post_urn` | `[data-id^="urn:li:activity:"]` outer attr | Unique stable ID. Use as primary key. |
| `posted_at` | `.update-components-actor__sub-description time` `datetime` attr OR relative-time text ("2d", "1w") | If only relative is available, resolve against `fetched_at` |
| `text` | `.feed-shared-inline-show-more-text` innerText | Click "See more" to expand if truncated. Strip "…see more" suffix. |
| `reactions` | `.social-details-social-counts__reactions-count` numeric | Returns "0" if hidden |
| `comments` | `.social-details-social-counts__comments` numeric | |
| `reposts` | `.social-details-social-counts__item` containing "repost" | LinkedIn uses share/repost interchangeably |
| `impressions` | **NOT on the activity feed** | Available only on `/analytics/post/` per-post drilldown — see "Impressions: per-post analytics" below |
| `media_type` | inspect children: `.update-components-image` (image), `.update-components-linkedin-video` (video), `.update-components-article` (article) | Used to bucket per-format ROI |

### Pagination / lazy load

LinkedIn lazy-loads activity below the fold via Intersection Observer. To capture N posts:

```js
// Scroll the activity feed container, not document.body — LinkedIn nests the feed.
const container = document.querySelector('main') || document.scrollingElement;
for (let i = 0; i < ceil(N / POSTS_PER_VIEWPORT); i++) {
  container.scrollTop = container.scrollHeight;
  await new Promise(r => setTimeout(r, 1500));  // let lazy-load fire
}
// Then querySelectorAll across all cards
const cards = document.querySelectorAll('[data-id^="urn:li:activity:"]');
```

Cap at `config.profile.max_posts_per_scrape` (proposed default: **20**) to avoid full-feed scraping. Most analytic value lives in the last 7-30 days.

### Impressions: per-post analytics

The activity feed shows reactions/comments/reposts but **NOT impressions**. Impressions are only on LinkedIn's per-post analytics view: `https://www.linkedin.com/analytics/post/{post_urn}/`.

Hitting that URL one post at a time would be slow (20 navigations) and rate-limit-risky. **Two-tier approach:**

1. **Phase 3b (cheap path):** scrape the activity feed for everything except impressions. ~1 navigation, ~20 posts captured. Most useful per-post fields covered.
2. **Phase 3c (deep path, opt-in):** for each captured `post_urn`, navigate to `/analytics/post/{urn}/` and pull impressions. Triggered by a `--with-impressions` flag (off by default). ~20 navigations, ~30-60 sec.

The activity-feed snapshot is run weekly. The deep impressions pull is run monthly or on demand when a specific post needs reach context (e.g., for the `/flywheel` Priority 3 verdict).

### `[opus:<clip_id>]` tag recovery

For posts that came from the OpusClip manifest pattern, the post body contains `[opus:<clip_id>]` as the last line. Extract this as a separate field so the closed-loop join can happen:

```js
const m = text.match(/\[(opus|lp|gh|bh):([A-Za-z0-9_-]+)\]/);
if (m) record.source_tag = { scheme: m[1], id: m[2] };
```

Then `/flywheel` (or a dedicated post-manifest fetcher) can join LinkedIn engagement for clip `La4Wghg6IX` against the OpusClip manifest's record of that clip's title/source video/score.

### Snapshot schema additions

Extend `profile` in the snapshot JSON:

```jsonc
{
  "profile": {
    "url": "https://linkedin.com/in/mikelady",
    "handle": "mikelady",
    "followers": 2115,
    "recent_posts": [                              // NEW
      {
        "post_urn": "urn:li:activity:7196234567890123456",
        "posted_at": "2026-05-17T14:23:00Z",
        "posted_at_relative": "2d",                 // captured as resolved if absolute unavailable
        "text": "Your next 50% productivity gain isn't a new AI tool. Hormozi's math: ... [opus:La4Wghg6IX]",
        "text_truncated_chars": 180,                // length of innerText before See-more expansion
        "reactions": 12,
        "comments": 3,
        "reposts": 1,
        "engagement_total": 16,                     // sum of above
        "impressions": null,                        // populated by --with-impressions
        "media_type": "video",                      // image | video | article | text
        "source_tag": {                             // NEW — populated when [scheme:id] found
          "scheme": "opus",
          "id": "La4Wghg6IX"
        }
      }
    ]
  }
}
```

Bumps the snapshot's implicit schema. Existing consumers must tolerate the new fields (additive — no breaking change).

### Config additions

`linkedin-stats/config.json`:

```jsonc
{
  "profile": {
    "handle": "mikelady",
    "max_posts_per_scrape": 20,                    // NEW
    "include_impressions": false,                  // NEW — default off (slow)
    "skip_reposts": false                          // NEW — set true if reposts pollute the engagement signal
  }
}
```

### Failure modes + edges

| Symptom | Cause | Fix |
|---|---|---|
| `data-id` selector returns 0 cards | LinkedIn DOM renamed | Fall back to `[id^="urn:li:activity:"]` or hunt by `time[datetime]` ancestor |
| Reaction count shows "All" or "1.2K" instead of int | LinkedIn abbreviates ≥1000 | Parse: regex `([\d.]+)([KM]?)` then multiply |
| Lazy load doesn't fire | Activity feed scroll container changed | Try `document.scrollingElement` AND each `<main>` / `[role="main"]` descendant |
| `/analytics/post/{urn}/` returns "not your post" | URN scoped to the wrong account — confirm we're logged in as `mikelady` not `enterprisevibecode` | Phase 1 login verify catches this |
| `[opus:...]` tag not found in body but post is from OpusClip | OpusClip stripped the tag (LinkedIn 1300-char "see more" truncation eats trailing footer text?) | Pull `description` from OpusClip manifest by date match as fallback |

Each gets an `Edge: <label>` block in the implemented SKILL.md.

## Consumers (already pencil-wired)

- **`/buffer-stats`** Phase 4 — uses `profile.recent_posts` engagement totals to compute LinkedIn personal Channel ROI (currently null because no engagement data exists).
- **`/flywheel`** Phase 4 — LinkedIn signal currently uses only newsletter + follower aggregates; Priority 3 verdict ("LinkedIn newsletter weekly") becomes per-post-grounded with this in place.
- **Future opus-clips-performance fetcher** — reads the OpusClip manifest's `[opus:<id>]` tags, joins against `profile.recent_posts[].source_tag`, produces per-clip engagement attribution for LinkedIn personal.

## Out of scope

- LinkedIn company-page per-post analytics (Phase 4 already has page-admin analytics path; if needed, the same recent-activity scrape adapts trivially).
- LinkedIn pulse article analytics — handled by the existing newsletter scrape (Phase 2).
- LinkedIn comment threads (who replied, sentiment, etc.) — second-order, not blocking the closed-loop.

## Implementation effort estimate

- Phase 3b (activity feed scrape, no impressions): **~3-4 hours**. Selector discovery + lazy-scroll loop + schema extension + edge handling.
- Phase 3c (per-post impressions, opt-in): **~2-3 hours additional**. URL pattern is known; mostly auth-state handling + rate-limit pacing.

Total to close the LinkedIn personal measurement gap: **~5-7 hours of focused implementation**.

## Why this isn't already implemented

Best guess: when `linkedin-stats` was first written, the immediate need was the newsletter subscriber count (Priority 3 KPI). Per-post engagement was sketched in the SKILL.md as "do this eventually" but never built because the aggregate follower count was enough for the weekly report. The closed-loop architecture was added later and assumed the per-post data existed without verifying.

Today's session exposed that assumption — `top_posts: []` + `has_per_post_engagement: false` was the actual state. This spec is the answer.
