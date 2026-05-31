---
name: tiktok-stats
description: Use when user wants TikTok per-post engagement stats for the @mikelady account — "tiktok stats", "how are my tiktoks doing", "tiktok engagement", "tiktok views", "tiktok analytics", "tiktok dashboard", "weekly tiktok report". Browser-scrapes TikTok Studio via claude-in-chrome (NO OAuth) — same interactive pattern as linkedin-stats.
user_invocable: true
---

# tiktok-stats

Scrape TikTok Studio's content-analytics table in the user's logged-in Chrome via `claude-in-chrome` and produce a one-shot report: profile-level stats, per-post engagement (views / likes / comments / shares), closed-loop `[scheme:id]` tag extraction, and week-over-week deltas against the prior cached snapshot. Writes `cache/snapshot-<date>.json` in the shape the `_shared/content-attribution/` JOIN engine reads (mirrors `linkedin-stats`).

**No OAuth.** This skill does NOT use the TikTok for Developers API — that path (app registration, scope review, 24h token refresh) was abandoned. It drives the same logged-in TikTok Studio session a human uses, exactly like `linkedin-stats` drives the LinkedIn dashboard. The scrape recipe is **already proven**: it was first validated 2026-05-30 inside `opus-clips-performance` Phase 3 (~31.5K reach across 6 platforms), and is lifted here verbatim into a standalone measure skill.

Why this exists: TikTok was a measurement blackhole. The user publishes to TikTok via two paths — Buffer (~1/day historically) and the OpusClip native scheduler (batches of ~23 clips that never touch Buffer) — and **neither Buffer Analyze nor any cached snapshot carried TikTok engagement**, so `content-attribution`'s `tiktok_business` platform came back `pending #373` for every clip. This skill writes the snapshot that resolves that pending.

## Usage

`/tiktok-stats` — full report (profile + recent posts + deltas)
`/tiktok-stats --no-cache` — skip writing the snapshot (ad-hoc checks that shouldn't disturb trend tracking)
`/tiktok-stats --since YYYY-MM-DD` — compute delta against a specific snapshot instead of the 7-day default
`/tiktok-stats --max-posts N` — cap how many posts are scraped (default 50)

## ⚠️ Interactive only — cannot go headless

Like `linkedin-stats`' headless-detection note and `opus-clips-performance` Phase 3: **this skill needs an open, logged-in TikTok Studio session in the user's Chrome.** It reads the live DOM via `claude-in-chrome`; there is no headless/API path. If the session is non-interactive (e.g. `/flywheel`'s unattended Sunday run), this skill must be skipped — a later interactive run fills in the snapshot, and the engine reports `tiktok_business` as `pending #373` until then. Do not attempt to log in programmatically; if TikTok Studio shows a login wall, hand control back to the user and ask them to log in, then continue.

## 🟢 Happy Path (read first; everything below is edge-case detail)

For a full `/tiktok-stats` run when nothing goes wrong. ~30-60 sec wall-clock. Each step links to a labeled edge case (`Edge: <name>`) you only need to read if that step fails.

**Phase 0 — Load config + chrome tools (2 sec).** Read `config.local.json` if present, else `config.json`. Pull `tiktok_handle`, `tiktok_studio_content_url`, `tiktok_profile_url`, `max_posts_per_scrape`, `delta_window_days`, `tag_regex`. Load the chrome tools (see Phase 0 below).

**Phase 1 — Browser + login check (5-10 sec).** Confirm a connected browser, navigate to the TikTok Studio content URL, and read the page. If a login wall appears, hand off to the user (Phase 1). TikTok Studio requires the user's own logged-in session — see `Edge: login-wall`.

**Phase 2 — Scrape the content table (15-30 sec).** This is the **proven recipe**. The content table at `tiktok.com/tiktokstudio/content` is a **virtualized list** — only the rows near the viewport are in the DOM at any moment. Scroll in **modest steps** (big jumps skip rows), reading via `get_page_text` after each step, and **dedupe by caption tag** (or post URL) as you accumulate. Each row carries the caption (with the `[opus:<clip_id>]` footer tag), Views, Likes, Comments columns. See `Edge: virtualized-skips` and `Edge: column-drift`.

**Phase 3 — Extract closed-loop tags + build records (2 sec).** For each scraped post, regex the caption for the closed-loop tag (`[(opus|lp|gh|bh):<id>]`). Build a `recent_posts[]` record per the schema below — each record carries `caption`, `source_tag: {scheme, id}` (the first/primary tag, or `null`), `closed_loop_tags[]` (all tags), `posted_at`, `share_url`, and an `engagement` object (`views`, `likes`, `comments`, `shares`).

**Phase 4 — Profile stats (5 sec).** Read profile-level numbers (follower count, total likes, video count) from TikTok Studio's home/overview or the public profile page. These are best-effort and `null`-safe — see `Edge: profile-tile-drift`.

**Phase 5 — Delta vs cached snapshot (2 sec).** Find the newest `cache/snapshot-*.json` older than `delta_window_days` (default 7). Diff today's `profile.follower_count` and total views against it. First run has no prior snapshot → deltas render as `—` (expected — see `Edge: delta-bootstrap`).

**Phase 6 — Render report.** Single markdown block: profile (followers + Δ), recent posts table sorted by views, closed-loop coverage (tagged vs organic), and notable flags (viral outliers, 0-view stalls).

**Phase 7 — Write snapshot (1 sec).** Unless `--no-cache`, write `cache/snapshot-$(date -u +%Y-%m-%d).json` with the schema below. Cache dir is gitignored; `/flywheel` and `_shared/content-attribution/`'s `tiktokMatch` read the newest snapshot here.

### Edge labels (jump to these only when you hit the matching failure signal)

| Label | Symptom |
|---|---|
| `Edge: login-wall` | TikTok Studio shows a login / "Log in" screen instead of the content table |
| `Edge: virtualized-skips` | Scraped fewer posts than expected, or rows are missing from the middle of the list |
| `Edge: column-drift` | A metric column (Views/Likes/Comments) is blank or shifted because TikTok re-skinned the table |
| `Edge: shares-not-in-table` | `shares` comes back `null` because the content table doesn't expose a Shares column |
| `Edge: profile-tile-drift` | Follower/likes/video-count tiles return null because the overview page changed |
| `Edge: source-tag-missing` | A post is from OpusClip but `[opus:<id>]` isn't in the visible caption (TikTok truncated it) |
| `Edge: delta-bootstrap` | Deltas render as `—` because there's no prior snapshot to compare against |

Each label corresponds to a heading in **Known issues / robustness notes** below.

## Config

The skill reads config from (in priority order):
1. `~/dev/claude-social-media-skills/tiktok-stats/config.local.json` (gitignored — put personal overrides here)
2. `~/dev/claude-social-media-skills/tiktok-stats/config.json` (committed defaults)

Fields:
- `tiktok_handle` — `@`-stripped handle (`mikelady`)
- `tiktok_profile_url` — full public profile URL (`https://www.tiktok.com/@mikelady`)
- `tiktok_studio_content_url` — the proven scrape target: `https://www.tiktok.com/tiktokstudio/content`
- `tiktok_studio_home_url` — TikTok Studio overview/home for profile tiles (`https://www.tiktok.com/tiktokstudio`)
- `max_posts_per_scrape` — hard cap on rows scraped (default 50)
- `delta_window_days` — compare against the newest snapshot older than this (default 7)
- `tag_regex` — closed-loop tag pattern (`\[(opus|lp|gh|bh):([A-Za-z0-9_-]+)\]`)

Load config at the start of every run:

```bash
CONFIG_DIR=~/dev/claude-social-media-skills/tiktok-stats
if [ -f "$CONFIG_DIR/config.local.json" ]; then CONFIG_FILE="$CONFIG_DIR/config.local.json"; else CONFIG_FILE="$CONFIG_DIR/config.json"; fi
HANDLE=$(jq -r .tiktok_handle "$CONFIG_FILE")
STUDIO_URL=$(jq -r .tiktok_studio_content_url "$CONFIG_FILE")
PROFILE_URL=$(jq -r .tiktok_profile_url "$CONFIG_FILE")
MAX_POSTS=$(jq -r '.max_posts_per_scrape // 50' "$CONFIG_FILE")
DELTA_DAYS=$(jq -r '.delta_window_days // 7' "$CONFIG_FILE")
```

## Process

### Phase 0 — Load config + chrome tools

The chrome tools are MCP tools that must be loaded before use. Load them first:

```
ToolSearch select:mcp__claude-in-chrome__tabs_context_mcp,mcp__claude-in-chrome__navigate,mcp__claude-in-chrome__get_page_text,mcp__claude-in-chrome__computer,mcp__claude-in-chrome__list_connected_browsers
```

**Use `get_page_text`, not screenshots** — it returns the full caption (with the `[opus:<clip_id>]` tag intact for exact matching) plus the metric columns. Screenshots truncate long captions and lose the tag, which breaks the join.

### Phase 1 — Connect browser + verify login

```
mcp__claude-in-chrome__list_connected_browsers   # confirm a browser is connected; if none, ask the user to open Chrome
mcp__claude-in-chrome__tabs_context_mcp          # see open tabs
mcp__claude-in-chrome__navigate → $STUDIO_URL    # go to tiktok.com/tiktokstudio/content
mcp__claude-in-chrome__get_page_text             # read the page
```

Logged-in markers: the content table header (Posts / Views / Likes / Comments columns), an "Upload" button. Not-logged-in markers: "Log in", "Sign up", a QR-code login panel.

If the login wall appears, **do not try to log in programmatically** — hand off:

> "TikTok Studio needs you to be logged in. Please log into tiktok.com in this Chrome window, then tell me to continue."

After the user confirms, re-navigate and re-read. (`Edge: login-wall`.)

### Phase 2 — Scrape the content table (PROVEN recipe)

The target is `tiktok.com/tiktokstudio/content` — TikTok Studio's per-post content list with **Views / Likes / Comments** columns. Validated 2026-05-30 in `opus-clips-performance` Phase 3 (TikTok had the highest engagement of any platform that run).

**The two things that make this work (don't skip either):**

1. **Virtualized list → scroll in modest steps.** TikTok Studio renders the content list as a virtualized list: only the rows near the viewport exist in the DOM. A big scroll jump (e.g. straight to the bottom) **skips the rows in between** — they're never rendered, so `get_page_text` never sees them. Scroll a screen-or-so at a time, reading after each step.
2. **Dedupe by tag (or post URL) as you accumulate.** Because the virtualized window overlaps between scroll steps, the same row appears in consecutive reads. Maintain an accumulating map keyed by the post's `[opus:<id>]` tag (or, for untagged posts, the post's TikTok video URL / a caption-prefix hash) and skip rows you've already captured.

Loop until you've captured `max_posts_per_scrape` distinct posts OR the post set stops growing across two consecutive scroll steps (you've hit the bottom):

```
# Pseudocode for the scrape loop (you drive these tool calls; the engine is your accumulating map):
seen = {}                      # key: tag-or-url  →  value: {caption, views, likes, comments, shares?, posted_at, share_url}
prev_count = -1
while len(seen) < MAX_POSTS:
    text = get_page_text()                    # read the currently-rendered rows
    for each post row parsed from `text`:
        key = post.tag or post.share_url or hash(caption_prefix)
        if key not in seen: seen[key] = post
    if len(seen) == prev_count: break         # no new rows two reads running → bottom reached
    prev_count = len(seen)
    computer(action="scroll", direction="down", amount=modest)   # ~one viewport, NOT to the bottom
```

Use `mcp__claude-in-chrome__computer` with a scroll action for the modest step (scroll the content pane, not the whole window if the table has its own scroll container). After each scroll, give the virtualized list a moment to render before the next `get_page_text`.

**Per-row fields to parse from `get_page_text`:**

| Field | Source in the table | Notes |
|---|---|---|
| `caption` | The post's description/caption text | Carries the `[opus:<clip_id>]` footer tag — the join key. Keep it verbatim. |
| `views` | Views column | Integer; TikTok may render `1.2K` / `3.4M` — normalize (×1e3 / ×1e6). |
| `likes` | Likes column | Same abbreviation handling. |
| `comments` | Comments column | Same. |
| `shares` | Shares column **if present** | The content table may not expose Shares — see `Edge: shares-not-in-table`. If absent, set `shares: null` (do NOT fabricate). **First-run-tunable.** |
| `posted_at` | The post's date/time | Convert to ISO-8601 UTC if a full timestamp is shown; if only a relative ("3d ago") or date is shown, store what's visible and note it's approximate. **First-run-tunable** — confirm the exact format on the first live run. |
| `share_url` | The post's link (`tiktok.com/@<handle>/video/<id>`) | Used as the dedupe fallback key for untagged posts and as `post_id` source. |

> **Validated vs first-run-tunable.** The **Views / Likes / Comments** columns and the virtualized-scroll-and-dedupe-by-tag flow are the **proven** parts (validated 2026-05-30). The `shares` column, the exact `posted_at` format, and the profile tiles in Phase 4 are **first-run-tunable** — confirm them against the live DOM on the first run and tighten the parse, rather than assuming a shape. Anything that doesn't render comes back `null`, never fabricated.

### Phase 3 — Extract closed-loop tags + build records

For each scraped post, pull every closed-loop tag from the caption (convention from `_shared/post-manifest/README.md` — schemes `opus` / `lp` / `gh` / `bh`):

```bash
TAG_RE='\[(opus|lp|gh|bh):([A-Za-z0-9_-]+)\]'
```

- `closed_loop_tags[]` — ALL `{scheme, id}` matches in the caption (zero is normal for organic/manual posts; multiple is rare but possible).
- `source_tag` — the **first/primary** match as a `{scheme, id}` object, or `null` if none. **This field is the engine's match target** (it mirrors `liPersonalMatch`'s `source_tag` check) — emit it even though `closed_loop_tags[]` is the superset, so the engine can match on `source_tag.scheme == "opus" && source_tag.id == <clipID>` without parsing the caption.

Then assemble each `recent_posts[]` record per the schema below.

### Phase 4 — Profile stats (best-effort, null-safe)

Read profile-level numbers. Prefer the public profile page (stable text), fall back to the Studio overview:

```
mcp__claude-in-chrome__navigate → $PROFILE_URL          # tiktok.com/@mikelady
mcp__claude-in-chrome__get_page_text
```

Parse from the visible text (all `null`-safe — emit `null`, not 0, when a number isn't found):
- `follower_count` — `/([\d.,KM]+)\s+Followers/i`
- `likes_count` — `/([\d.,KM]+)\s+Likes/i`
- `following_count` — `/([\d.,KM]+)\s+Following/i`
- `video_count` — count of post tiles, or a Studio overview tile if present (**first-run-tunable**)

Normalize `1.2K` / `3.4M` to integers (×1e3 / ×1e6). (`Edge: profile-tile-drift`.)

### Phase 5 — Delta vs cached snapshot

```bash
CACHE_DIR=~/dev/claude-social-media-skills/tiktok-stats/cache
mkdir -p "$CACHE_DIR"

CUTOFF=$(date -v-${DELTA_DAYS}d -u +%Y-%m-%d 2>/dev/null || date -d "$DELTA_DAYS days ago" -u +%Y-%m-%d)
PRIOR_SNAP=$(ls -1 "$CACHE_DIR"/snapshot-*.json 2>/dev/null | awk -v c="$CACHE_DIR/snapshot-$CUTOFF" '$0 <= c' | tail -1)

if [ -n "$PRIOR_SNAP" ]; then
  PRIOR_FOLLOWERS=$(jq -r '.profile.follower_count // empty' "$PRIOR_SNAP")
  PRIOR_VIEWS=$(jq -r '[.recent_posts[].engagement.views // 0] | add' "$PRIOR_SNAP")
  # FOLLOWER_DELTA / VIEWS_DELTA computed against today's numbers
else
  PRIOR_SNAP=""   # first run → deltas render as —
fi
```

### Phase 6 — Render report

```
TikTok — weekly snapshot (YYYY-MM-DD)

Profile (@mikelady):
  Followers:     N (+Δ vs last week | —)
  Total likes:   N
  Videos:        N

Recent posts (top by views):
  YYYY-MM-DD   V views · L likes · C comments · S shares
    "<caption snippet>"   [opus:<id> | —]

Closed-loop coverage:
  Posts with a closed-loop tag: X / Y
  Untagged (organic / Buffer / manual): Z

Notes:
  - <flag any post with ≥10× the median view count — viral signal>
  - <flag any post >24h old still at 0 views — possible publish failure / shadow issue>
  - <if shares column absent this run, note "shares: n/a (not in content table)">
```

### Phase 7 — Write snapshot (unless `--no-cache`)

Write `cache/snapshot-$(date -u +%Y-%m-%d).json` with the schema below. Use a normal write (or tmp + mv for atomicity). The cache directory is gitignored — snapshots stay local and private.

## Snapshot schema (the engine reads this — keep it stable)

The `_shared/content-attribution/` JOIN engine's `tiktokMatch(clipID)` will read **the newest `cache/snapshot-*.json`** (via `newestSnapshot(tiktokCacheDir())`) and match `[opus:<clipID>]` against each post — modeled exactly on `liPersonalMatch`. Two differences from LinkedIn that the engine author must know:

1. **The per-post array is TOP-LEVEL `recent_posts[]`** (NOT nested under `profile.recent_posts[]` like LinkedIn). The engine's `tiktokMatch` should read `doc["recent_posts"]`, not `doc["profile"]["recent_posts"]`.
2. **Each post carries `source_tag: {scheme, id}`** (primary tag) AND a `caption` text field — so the engine can match on `source_tag` first and fall back to a `strings.Contains(caption, "[opus:"+clipID+"]")` substring check, exactly like `liPersonalMatch` does on `body`/`text`.

```jsonc
{
  "fetched_at": "2026-05-30T19:00:00Z",   // ISO-8601 UTC, when this snapshot was scraped
  "platform": "tiktok",
  "handle": "mikelady",
  "source": "tiktok_studio_scrape",        // provenance: browser scrape, not API
  "profile": {
    "follower_count": 1234,                 // null if not scraped
    "following_count": 56,                  // null-safe
    "likes_count": 7890,                    // total profile likes; null-safe
    "video_count": 142                      // null-safe
  },
  "recent_posts": [                          // TOP-LEVEL array — the engine's match target
    {
      "post_id": "7234567890123456789",     // TikTok video id (from share_url); "" if unknown
      "share_url": "https://www.tiktok.com/@mikelady/video/7234567890123456789",
      "posted_at": "2026-05-30T16:00:00Z",  // ISO-8601 UTC if a full timestamp; else best-available (first-run-tunable)
      "caption": "Your next 50% productivity gain isn't a new AI tool … [opus:La4Wghg6IX]",
      "source_tag": { "scheme": "opus", "id": "La4Wghg6IX" },   // PRIMARY tag, or null — engine matches on this
      "closed_loop_tags": [                  // ALL tags found in the caption (superset of source_tag)
        { "scheme": "opus", "id": "La4Wghg6IX" }
      ],
      "engagement": {                        // the metrics object — matches the linkedin/engine convention
        "views": 539,
        "likes": 2,
        "comments": 0,
        "shares": null                       // null when the content table has no Shares column (do not fabricate)
      }
    }
    // … one record per scraped post
  ],
  "deltas_vs_prior_snapshot": {              // null on first run (no prior snapshot)
    "prior_snapshot": "snapshot-2026-05-23.json",
    "follower_delta": 0,
    "total_views_delta": 0
  }
}
```

**Field contract for the engine match (mirror of `liPersonalMatch`):**
- Path to posts: `recent_posts[]` (top level).
- Match key per post: `source_tag.{scheme,id}` (primary) OR raw `[opus:<id>]` substring in `caption`.
- Metrics live INSIDE `engagement` (so the engine returns `{engagement: {views, likes, comments, shares, join_method:"tag"}}`, consistent with how YouTube/LinkedIn records wrap metrics).
- `post_id` / `share_url` give the engine a stable per-post identity if a future field needs it (analogous to LinkedIn's `urn`/`post_urn`).

> A reference jq that builds one record from a scraped row (illustrative — adapt to the live parse):
> ```bash
> jq -n --arg cap "$CAPTION" --arg url "$SHARE_URL" --arg pid "$POST_ID" \
>   --arg posted "$POSTED_AT" --argjson v "$VIEWS" --argjson l "$LIKES" \
>   --argjson c "$COMMENTS" --argjson s "${SHARES:-null}" '
>   ($cap | [scan("\\[(opus|lp|gh|bh):([A-Za-z0-9_-]+)\\]")]) as $raw
>   | ([$raw[] | {scheme: .[0], id: .[1]}]) as $tags
>   | {post_id:$pid, share_url:$url, posted_at:$posted, caption:$cap,
>      source_tag: ($tags[0] // null), closed_loop_tags: $tags,
>      engagement: {views:$v, likes:$l, comments:$c, shares:$s}}'
> ```

## Known issues / robustness notes

- **Interactive only / login wall.** TikTok Studio requires the user's own logged-in session and shows a QR/login wall otherwise. The skill never logs in programmatically — it hands off to the user and resumes. Unattended runs (e.g. `/flywheel`) skip this skill entirely; the engine reports `tiktok_business` as `pending #373` until a later interactive run writes a snapshot.
  *Label: `Edge: login-wall`*
- **Virtualized-list skips.** The content table is a virtualized list — big scroll jumps skip un-rendered rows. Always scroll in modest (≈one-viewport) steps, read after each, and dedupe by tag/URL. If the captured count is suspiciously low, you probably jumped too far; reset to the top and scroll more gently.
  *Label: `Edge: virtualized-skips`*
- **Column drift.** If a metric column comes back blank or shifted, TikTok re-skinned the table. Fall back to reading the per-post detail (open a post and read its analytics panel) or `handoff` a note asking the user to eyeball the row. Never silently emit a stale/zero value — emit `null`.
  *Label: `Edge: column-drift`*
- **Shares not in the content table.** The list view may expose only Views/Likes/Comments. If there's no Shares column, set `engagement.shares: null` (the per-post detail panel has Shares if a precise number is required — opt-in, slow). **First-run-tunable.**
  *Label: `Edge: shares-not-in-table`*
- **Profile-tile drift.** Follower/likes/video tiles move between the public profile and the Studio overview. Try the public profile first (stabler text); emit `null` for any tile not found rather than 0.
  *Label: `Edge: profile-tile-drift`*
- **Source-tag missing.** If a post is from OpusClip but `[opus:<id>]` isn't in the visible caption, TikTok truncated the caption in the list view. Open that post's detail to read the full caption, or leave `source_tag: null` — the engine then falls back to its time-window match for that clip.
  *Label: `Edge: source-tag-missing`*
- **View-count lag.** TikTok view counts lag 30-60 min for fresh posts. Flag any post posted within the last hour as "still settling" in the report; the snapshot stores the live number as-is.
- **Captions are mutable.** Users can edit TikTok captions after posting; an edited-out tag breaks the join. The skill snapshots the caption on every run so historical joins stay stable even if the live caption later changes.
  *Label: `Edge: source-tag-missing`*
- **Delta bootstrap.** The first run has no prior snapshot, so deltas render as `—`. After one week of snapshots the numbers mean something.
  *Label: `Edge: delta-bootstrap`*

## Downstream consumers

Once snapshots exist here, these read them:

- **`_shared/content-attribution/`** — `tiktokMatch(clipID)` reads the newest snapshot's `recent_posts[]` and resolves the `tiktok_business` platform record (today hard-wired to `pending #373`). This skill writing snapshots is what unblocks that pending. See the schema contract above.
- **`/opus-clips-performance`** — Phase 3's interactive TikTok browser overlay can **defer to this snapshot** once it's populated: instead of re-scraping TikTok Studio inline, it (or the engine) reads `recent_posts[].engagement` matched by `[opus:<clip_id>]`. The overlay remains as a fallback for runs where no fresh snapshot exists.
- **`/flywheel`** — pulls `profile.follower_count` and the sum of `recent_posts[].engagement.views` into the weekly Priority-1 throughput rollup (via the engine, same as the other platforms).
- **`/buffer-stats`** — for the ~1/day historical Buffer→TikTok posts (which Buffer Analyze can't measure), can join on `caption` substring or `share_url` against this snapshot to attribute Buffer-scheduled TikTok posts.

## Provenance

The scrape recipe (TikTok Studio → `tiktok.com/tiktokstudio/content`; Views/Likes/Comments columns; virtualized-scroll-in-modest-steps + dedupe-by-tag; captions carry `[opus:<clip_id>]`) was **first validated 2026-05-30** inside `opus-clips-performance` Phase 3 — total ~31.5K reach across 6 platforms, with TikTok the highest-engagement platform that run. This skill lifts that proven recipe into a standalone, snapshot-writing measure skill so the engine can read TikTok engagement directly instead of relying on the inline overlay.
