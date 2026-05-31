---
name: threads-stats
description: Use when user wants Threads per-post engagement stats for the @mikelady account — "threads stats", "how are my threads doing", "threads engagement", "threads views", "threads analytics", "threads insights", "weekly threads report". Browser-scrapes Threads Insights via claude-in-chrome (NO OAuth) — same interactive pattern as linkedin-stats.
user_invocable: true
---

# threads-stats

Scrape **Threads Insights** in the user's logged-in Chrome via `claude-in-chrome` and produce a one-shot report: account-level stats (Views / Interactions / Followers), per-post engagement (views / likes / replies), closed-loop `[scheme:id]` tag extraction, and week-over-week deltas against the prior cached snapshot. Writes `cache/snapshot-<date>.json` in the shape the `_shared/content-attribution/` JOIN engine reads (mirrors `linkedin-stats` / `tiktok-stats`).

**No OAuth.** This skill does NOT use the Meta Graph / Threads API — that path (app review, OAuth consent, token refresh) was abandoned. It drives the same logged-in Threads session a human uses, exactly like `linkedin-stats` drives the LinkedIn dashboard.

**Target the LIVE `@mikelady` account only.** The `@enterprisevibecode` Threads account is DEAD and paused (`audits/threads-evc-dead-channel.md`, enforced via the buffer-post-prep deny-list, #588) — do not scrape or report it.

Why this exists: Threads was a measurement blackhole — neither Buffer Analyze nor any cached snapshot carried Threads engagement, so `content-attribution`'s `threads` platform came back `pending #375` for every clip/post. This skill writes the snapshot that resolves that pending.

## Usage

`/threads-stats` — full report (account summary + recent posts + deltas)
`/threads-stats --no-cache` — skip writing the snapshot (ad-hoc checks that shouldn't disturb trend tracking)
`/threads-stats --since YYYY-MM-DD` — compute delta against a specific snapshot instead of the 7-day default
`/threads-stats --max-posts N` — cap how many posts are scraped (default 50)

## ⚠️ Interactive only — cannot go headless

Like `linkedin-stats` / `tiktok-stats`: **this skill needs an open, logged-in Threads session in the user's Chrome.** It reads the live DOM via `claude-in-chrome`; there is no headless/API path. If the session is non-interactive (e.g. `/flywheel`'s unattended Sunday run), this skill must be skipped — a later interactive run fills in the snapshot, and the engine reports `threads` as `pending #375` until then. Never log in programmatically; if a login wall appears, hand control back to the user.

## 🟢 Happy Path (read first; everything below is edge-case detail)

For a full `/threads-stats` run when nothing goes wrong. ~30-60 sec wall-clock. The two scrape surfaces below were **verified directly against the live `@mikelady` session on 2026-05-30** (URLs + on-page structure confirmed, not inferred).

**Phase 0 — Load config + chrome tools (2 sec).** Read `config.local.json` if present, else `config.json`. Pull `profile_handle`, `insights_url`, `posts_insights_url`, `profile_url`, `max_posts_per_scrape`, `delta_window_days`, `tag_regex`. Load the chrome tools (see Phase 0 below).

**Phase 1 — Browser + login check (5-10 sec).** Confirm a connected browser, navigate to `insights_url`, read the page. Logged-in marker: the "Insights" heading with Summary cards (Views / Interactions / Followers). Login wall → hand off to the user (`Edge: login-wall`).

**Phase 2 — Account summary (5 sec).** On `https://www.threads.com/insights` read the three Summary cards: **Views**, **Interactions**, **Followers** (each a number; the page defaults to a "Last 30 days" range, with a dropdown to change it). These are the account-level rollup.

**Phase 3 — Scrape the per-post list (15-30 sec).** Navigate to `https://www.threads.com/insights/posts?days=30` (the "Top content" full list). Each row carries: handle, `MM/DD/YY` date, the **full caption** (with the `[opus:<clip_id>]` footer tag intact — the join key), **views**, **likes**, and **replies**. The list **loads more on scroll** — scroll in modest steps, read via `get_page_text` after each, and **dedupe by caption tag / post date** as you accumulate (same discipline as the tiktok-stats virtualized scrape). See `Edge: list-skips`.

**Phase 4 — Extract closed-loop tags + build records (2 sec).** For each post, regex the caption for `[(opus|lp|gh|bh):<id>]`. Build a `recent_posts[]` record per the schema below — each carries `caption`, `source_tag: {scheme, id}` (primary, or `null`), `closed_loop_tags[]` (all), `posted_at` (from the `MM/DD/YY` date), and an `engagement` object (`views`, `likes`, `replies`, plus `reposts`/`quotes` which are **first-run-tunable** — see `Edge: reposts-not-in-list`).

**Phase 5 — Delta vs cached snapshot (2 sec).** Find the newest `cache/snapshot-*.json` older than `delta_window_days` (default 7). Diff today's `profile.follower_count` and total views against it. First run → deltas render as `—` (`Edge: delta-bootstrap`).

**Phase 6 — Render report.** Single markdown block: account summary (followers + Δ, 30-day views, interactions), recent posts table sorted by views, closed-loop coverage (tagged vs organic), notable flags.

**Phase 7 — Write snapshot (1 sec).** Unless `--no-cache`, write `cache/snapshot-$(date -u +%Y-%m-%d).json` with the schema below. Cache dir is gitignored; `/flywheel` and `_shared/content-attribution/`'s `threadsMatch` read the newest snapshot here.

### Edge labels (jump to these only when you hit the matching failure signal)

| Label | Symptom |
|---|---|
| `Edge: login-wall` | Threads shows a "Log in" screen instead of the Insights page |
| `Edge: list-skips` | Scraped fewer posts than expected, or rows missing from the middle of the list |
| `Edge: reposts-not-in-list` | `reposts`/`quotes` come back `null` because the Top-content list only shows views/likes/replies |
| `Edge: source-tag-missing` | A post is from OpusClip but `[opus:<id>]` isn't in the visible caption |
| `Edge: domain-redirect` | `threads.com` redirects to `threads.net` (or vice-versa) — both are valid; follow the redirect |
| `Edge: delta-bootstrap` | Deltas render as `—` because there's no prior snapshot to compare against |

## Config

The skill reads config from (in priority order):
1. `~/dev/claude-social-media-skills/threads-stats/config.local.json` (gitignored — personal overrides)
2. `~/dev/claude-social-media-skills/threads-stats/config.json` (committed defaults)

Fields (all **verified** 2026-05-30):
- `profile_handle` — `@`-stripped handle (`mikelady`)
- `profile_url` — public profile (`https://www.threads.com/@mikelady`)
- `insights_url` — account-level Insights (`https://www.threads.com/insights`) — Summary cards live here
- `posts_insights_url` — per-post "Top content" list (`https://www.threads.com/insights/posts?days=30`) — the per-post scrape surface
- `max_posts_per_scrape` — hard cap on rows scraped (default 50)
- `delta_window_days` — compare against the newest snapshot older than this (default 7)
- `tag_regex` — closed-loop tag pattern (`\[(opus|lp|gh|bh):([A-Za-z0-9_-]+)\]`)

```bash
CONFIG_DIR=~/dev/claude-social-media-skills/threads-stats
if [ -f "$CONFIG_DIR/config.local.json" ]; then CONFIG_FILE="$CONFIG_DIR/config.local.json"; else CONFIG_FILE="$CONFIG_DIR/config.json"; fi
HANDLE=$(jq -r .profile_handle "$CONFIG_FILE")
INSIGHTS_URL=$(jq -r .insights_url "$CONFIG_FILE")
POSTS_URL=$(jq -r .posts_insights_url "$CONFIG_FILE")
PROFILE_URL=$(jq -r .profile_url "$CONFIG_FILE")
MAX_POSTS=$(jq -r '.max_posts_per_scrape // 50' "$CONFIG_FILE")
DELTA_DAYS=$(jq -r '.delta_window_days // 7' "$CONFIG_FILE")
```

## Process

### Phase 0 — Load config + chrome tools

```
ToolSearch select:mcp__claude-in-chrome__tabs_context_mcp,mcp__claude-in-chrome__navigate,mcp__claude-in-chrome__get_page_text,mcp__claude-in-chrome__computer,mcp__claude-in-chrome__list_connected_browsers
```

**Use `get_page_text`, not screenshots** — it returns the full caption (with the `[opus:<clip_id>]` tag intact) plus the per-post metrics. (Verified: `get_page_text` on the Top-content list returns each post's full multi-paragraph caption, the date, and the views/likes counts as plain text.)

### Phase 1 — Connect browser + verify login

```
mcp__claude-in-chrome__list_connected_browsers   # confirm a browser is connected
mcp__claude-in-chrome__tabs_context_mcp          # see open tabs (create a fresh tab; don't hijack the user's)
mcp__claude-in-chrome__navigate → $INSIGHTS_URL  # https://www.threads.com/insights
mcp__claude-in-chrome__get_page_text
```

Logged-in markers: the "Insights" heading + "Summary" with Views / Interactions / Followers cards. Not-logged-in markers: "Log in", "Sign up". If a login wall appears, **do not log in programmatically** — hand off:

> "Threads needs you to be logged in. Please log into threads.com in this Chrome window, then tell me to continue."

(`Edge: login-wall`. Note `Edge: domain-redirect` — `threads.com` and `threads.net` both serve the same app; follow whichever the session lands on.)

### Phase 2 — Account summary (verified surface)

On `https://www.threads.com/insights` (default range "Last 30 days"; dropdown changes it), read the Summary cards:

| Card | What | Parse |
|---|---|---|
| Views | Total post views in the window | integer (normalize `1.2K`/`3.4M`) |
| Interactions | Likes + replies + reposts + quotes in the window | integer |
| Followers | Current follower count | integer |

A daily Views bar chart sits under the Views card (May 1 → May 30 on the 30-day range) — not scraped per-day in v1; the totals are enough for the rollup. To change the window, click the "Last 30 days" dropdown (top of the page) — the per-post list also takes `?days=7|30|90` in the URL (`posts_insights_url`).

### Phase 3 — Scrape the per-post list (verified surface)

Navigate to `$POSTS_URL` (`https://www.threads.com/insights/posts?days=30`). This is the "Top content" full list — the per-post analog of TikTok Studio's content table. It has filter chips at the top: **range** ("Last 30 days"), **content type** ("Posts"), **sort metric** ("Views"), and **scope** ("All posts" — keep this so you get every post in the window, not just top-ranked).

**Verified per-row shape** (from `get_page_text` on 2026-05-30):
```
mikelady
05/16/26
<full multi-paragraph caption, including any gist/github link and the [opus:<id>] footer tag>
<likes>            ← e.g. "3"
<views>  views     ← e.g. "359  views"
```
Replies show as the 💬 count in the post's engagement row (often 0). Some rows render a link-preview card between caption and metrics — skip it when parsing.

**The two things that make this work (don't skip either):**
1. **Loads-more-on-scroll → scroll in modest steps.** The list lengthens as you scroll; scroll ~one viewport at a time and `get_page_text` after each step.
2. **Dedupe as you accumulate.** Keep an accumulating map keyed by the post's `[opus:<id>]` tag (or, for untagged posts, the `MM/DD/YY` date + a caption-prefix hash) and skip rows already captured.

Loop until you've captured `max_posts_per_scrape` distinct posts OR the set stops growing across two consecutive reads (bottom reached):

```
seen = {}                      # key: tag-or-(date+caption_prefix) → {caption, date, views, likes, replies}
prev_count = -1
while len(seen) < MAX_POSTS:
    text = get_page_text()                       # read currently-rendered rows
    for each post row parsed from `text`:
        key = post.tag or (post.date + hash(caption_prefix))
        if key not in seen: seen[key] = post
    if len(seen) == prev_count: break            # no new rows two reads running → bottom
    prev_count = len(seen)
    computer(action="scroll", direction="down", amount=modest)
```

| Field | Source | Notes |
|---|---|---|
| `caption` | The post text | Carries `[opus:<clip_id>]` — the join key. Keep verbatim. |
| `views` | The "N views" line (right of the row) | Integer; normalize `1.2K`/`3.4M`. |
| `likes` | The ❤ count in the engagement row | Integer. |
| `replies` | The 💬 count in the engagement row | Integer; often 0. |
| `reposts` / `quotes` | NOT in the Top-content list | Set `null` (do NOT fabricate). The per-post detail panel has them — opt-in/slow. **First-run-tunable.** (`Edge: reposts-not-in-list`) |
| `posted_at` | The `MM/DD/YY` date | Convert to ISO date (`20YY-MM-DD`). The list shows date only (no time) — store the date; mark time unknown. |

> **Verified vs first-run-tunable.** Views / likes / replies + the two surfaces' URLs and structure are **verified** (live session 2026-05-30). `reposts`/`quotes` (not in the list view) and the exact infinite-scroll container behavior are **first-run-tunable** — confirm on the first run and tighten; anything absent is `null`, never fabricated.

### Phase 4 — Extract closed-loop tags + build records

```bash
TAG_RE='\[(opus|lp|gh|bh):([A-Za-z0-9_-]+)\]'
```
- `closed_loop_tags[]` — ALL `{scheme, id}` matches in the caption (zero is normal for organic posts).
- `source_tag` — the first/primary match as `{scheme, id}`, or `null`. **This is the engine's match target** (mirrors `liPersonalMatch` / `tiktokMatch`'s `source_tag` check).

### Phase 5 — Delta vs cached snapshot

```bash
CACHE_DIR=~/dev/claude-social-media-skills/threads-stats/cache
mkdir -p "$CACHE_DIR"
CUTOFF=$(date -v-${DELTA_DAYS}d -u +%Y-%m-%d 2>/dev/null || date -d "$DELTA_DAYS days ago" -u +%Y-%m-%d)
PRIOR_SNAP=$(ls -1 "$CACHE_DIR"/snapshot-*.json 2>/dev/null | awk -v c="$CACHE_DIR/snapshot-$CUTOFF" '$0 <= c' | tail -1)
# follower_delta / total_views_delta vs today; first run → PRIOR_SNAP empty → render —
```

### Phase 6 — Render report

```
Threads — weekly snapshot (YYYY-MM-DD)

Account (@mikelady):
  Followers:     N (+Δ vs last week | —)
  Views (30d):   N
  Interactions:  N

Recent posts (top by views):
  YYYY-MM-DD   V views · L likes · R replies
    "<caption snippet>"   [opus:<id> | —]

Closed-loop coverage:
  Posts with a closed-loop tag: X / Y
  Untagged (organic / Buffer / manual): Z

Notes:
  - <flag any post with ≥10× the median view count — viral signal>
  - <flag any post >24h old still at 0 views — possible publish failure>
  - <if reposts/quotes absent this run, note "reposts/quotes: n/a (not in list view)">
```

### Phase 7 — Write snapshot (unless `--no-cache`)

Write `cache/snapshot-$(date -u +%Y-%m-%d).json` with the schema below (tmp + mv for atomicity). Cache dir is gitignored.

## Snapshot schema (the engine reads this — keep it stable)

**Identical container shape to `tiktok-stats`** so `_shared/content-attribution/`'s `threadsMatch(clipID)` mirrors `tiktokMatch`: read the newest `cache/snapshot-*.json` via `newestSnapshot(threadsCacheDir())`, match `[opus:<clipID>]` against each **top-level `recent_posts[]`** record by `source_tag` first, then `strings.Contains(caption, "[opus:"+clipID+"]")`.

```jsonc
{
  "fetched_at": "2026-05-30T19:00:00Z",
  "platform": "threads",
  "handle": "mikelady",
  "source": "threads_insights_scrape",
  "profile": {
    "follower_count": 1196,        // from the Followers Summary card / profile
    "views_30d": 1130,             // Views Summary card (window total); null-safe
    "interactions_30d": 14         // Interactions Summary card; null-safe
  },
  "recent_posts": [                 // TOP-LEVEL array — the engine's match target
    {
      "post_id": "",               // Threads post id if resolvable from the post URL; "" otherwise
      "share_url": "https://www.threads.com/@mikelady/post/<shortcode>",  // best-effort; "" if not surfaced in the list
      "posted_at": "2026-05-16",   // date from the MM/DD/YY column (time unknown)
      "caption": "Spent the last few weeks dissecting that 1M-line … [opus:La4Wghg6IX]",
      "source_tag": { "scheme": "opus", "id": "La4Wghg6IX" },   // PRIMARY tag, or null — engine matches on this
      "closed_loop_tags": [ { "scheme": "opus", "id": "La4Wghg6IX" } ],
      "engagement": {
        "views": 359,
        "likes": 3,
        "replies": 0,
        "reposts": null,           // not in the Top-content list (first-run-tunable; null, never fabricated)
        "quotes": null
      }
    }
    // … one record per scraped post
  ],
  "deltas_vs_prior_snapshot": {     // null on first run
    "prior_snapshot": "snapshot-2026-05-23.json",
    "follower_delta": 0,
    "total_views_delta": 0
  }
}
```

**Field contract for the engine match (mirror of `liPersonalMatch` / `tiktokMatch`):**
- Path to posts: `recent_posts[]` (top level).
- Match key per post: `source_tag.{scheme,id}` (primary) OR raw `[opus:<id>]` substring in `caption`.
- Metrics live inside `engagement` (engine returns `{engagement: {views, likes, replies, reposts, quotes, join_method:"tag"}}`, consistent with the YouTube/LinkedIn/TikTok wrap convention).

## Known issues / robustness notes

- **Interactive only / login wall.** Threads Insights requires the user's logged-in session. The skill never logs in programmatically — it hands off and resumes. Unattended runs (e.g. `/flywheel`) skip this skill; the engine reports `threads` as `pending #375` until a later interactive run writes a snapshot.
  *Label: `Edge: login-wall`*
- **List skips.** The Top-content list loads more on scroll; big jumps can skip un-rendered rows. Scroll in modest steps, read after each, dedupe by tag/date. If the count is suspiciously low, reset to the top and scroll more gently.
  *Label: `Edge: list-skips`*
- **Reposts/quotes not in the list.** The Top-content list exposes views/likes/replies only. Reposts and quotes are in the per-post detail panel (opt-in, slow); otherwise emit `null`. **First-run-tunable.**
  *Label: `Edge: reposts-not-in-list`*
- **Domain redirect.** `threads.com` ⇄ `threads.net` serve the same app. Follow whichever the session lands on; both URLs in config are interchangeable.
  *Label: `Edge: domain-redirect`*
- **Source-tag missing.** If a post is from OpusClip but `[opus:<id>]` isn't visible (caption truncated), open the post to read the full caption, or leave `source_tag: null` — the engine falls back to its time-window match.
  *Label: `Edge: source-tag-missing`*
- **Top-content vs all-posts.** The summary page's "Top content" preview is ranked; the `/insights/posts` list with the "All posts" scope chip is the complete set for the window. Always scrape the latter for closed-loop coverage.
- **Delta bootstrap.** First run has no prior snapshot → deltas render as `—`.
  *Label: `Edge: delta-bootstrap`*

## Downstream consumers

- **`_shared/content-attribution/`** — `threadsMatch(clipID)` reads the newest snapshot's `recent_posts[]` and resolves the `threads` platform record (today hard-wired to `pending #375`). This skill writing snapshots unblocks that pending.
- **`/flywheel`** — pulls `profile.follower_count` + `profile.views_30d` into the weekly rollup (via the engine, same as the other platforms).
- **`/opus-clips-performance`** — Phase 3's overlay can read these snapshots for Threads-published clips instead of (or in addition to) re-scraping.

## Provenance

The scrape recipe — account Insights at `https://www.threads.com/insights` (Views / Interactions / Followers Summary cards + range dropdown) and the per-post "Top content" list at `https://www.threads.com/insights/posts?days=30` (date / caption / views / likes / replies, loads-more-on-scroll) — was **verified directly against the live authenticated `@mikelady` session on 2026-05-30**. Unlike TikTok (lifted from the proven `opus-clips-performance` Phase 3 recipe), Threads had no prior recipe in this repo; this skill is the first Threads scrape and its surfaces were confirmed on a real session, not inferred. `reposts`/`quotes` per post are the only fields not on the verified surface — first-run-tunable.
