---
name: tiktok-stats
description: Use when user wants TikTok per-post engagement stats for the @mikelady account — "tiktok stats", "how's my tiktok doing", "tiktok views", "tiktok analytics", "tiktok dashboard", "weekly tiktok report". Reads from TikTok Business / Display API once OAuth is configured.
user_invocable: true
---

# tiktok-stats

> **STATUS: NOT YET FUNCTIONAL — auth setup required.**
> This skill is scaffolded but cannot fetch data until the user completes **Phase 0 (Auth Setup)** below. Running `/tiktok-stats` before then will print the auth checklist and exit non-zero. Task #373 tracks the build-out; the fetch logic in Phases 1–3 is skeleton only and intentionally not wired against the live API.

Fetch per-post engagement (view_count, like_count, comment_count, share_count) for the user's @mikelady TikTok account via the TikTok for Developers API, extract closed-loop `[scheme:id]` tags from each caption, and snapshot results under `cache/snapshot-<date>.json` in the same shape `linkedin-stats` uses.

## Why this exists

The user publishes to TikTok via two paths:

1. **Buffer** (~1 post/day historically). Buffer's tag system would normally cover this — but **Buffer Analyze does not provide TikTok engagement metrics**, so `buffer-stats` has no data to attribute against.
2. **OpusClip native scheduler** (23 posts/cycle from the 2026-05-18 batch, scheduled 2026-05-19 → 2026-05-23). These never touch Buffer at all and have no `format:` tag.

Net effect today: **TikTok is a measurement blackhole.** No view counts, no engagement breakdown, no per-clip ROI signal. This skill closes that gap by reading directly from TikTok's API.

## Usage (once auth is configured)

`/tiktok-stats` — full report: profile stats + recent N posts + deltas
`/tiktok-stats --no-cache` — skip writing the snapshot (ad-hoc checks)
`/tiktok-stats --since YYYY-MM-DD` — compute delta against a specific snapshot
`/tiktok-stats --window-days N` — override the default 14-day fetch window
`/tiktok-stats --max-posts N` — cap how many posts are pulled (default 50)

## Phase 0 — Auth setup (USER ACTION REQUIRED)

This phase is **manual**. The skill cannot do it for you because TikTok's OAuth flow requires browser interaction and app registration in a portal that gates by business-account login.

### Step 0.1 — Register the developer app

1. Go to <https://developers.tiktok.com/> and **sign in with the same TikTok account that owns @mikelady** (this matters — the app must be authorized by the account whose data you want to read).
2. Open the developer portal → **Manage apps** → **Connect an app**.
3. Fill in:
   - App name: `mikelady-stats` (or any internal label).
   - Category: Productivity / Analytics.
   - Redirect URI: `http://localhost:8000/callback` (we'll spin up a one-shot local listener to capture the code).
4. Under **Add products**, enable:
   - **Login Kit** (required to run OAuth).
   - **Display API** (read access to the user's videos).
   - Optionally: **Content Posting API** (not used by this skill but unlocks future publish-through-API).
5. Under **Scopes**, request:
   - `user.info.basic` — display name, avatar.
   - `user.info.stats` — follower_count, following_count, likes_count, video_count.
   - `video.list` — paginated list of the authorized user's own videos with engagement fields.
6. Save. Note the **Client Key** and **Client Secret** — you'll paste these into `~/.zshrc` in step 0.3.

> If TikTok asks for **app review / approval** before the requested scopes activate, submit the review. Until approval lands, the scopes return `scope_not_authorized` even with a valid token. For a single-account read-only personal-analytics app, review is usually fast (hours to a few days).

### Step 0.2 — Run the OAuth authorization-code flow

TikTok uses standard OAuth 2.0 authorization-code with PKCE. From a terminal, run a one-shot helper (we'll add `scripts/oauth.ts` in a follow-up; for now do it by hand):

```
# 1. Build the authorize URL
CLIENT_KEY=<from step 0.1>
REDIRECT=http://localhost:8000/callback
STATE=$(openssl rand -hex 16)
SCOPES="user.info.basic,user.info.stats,video.list"
open "https://www.tiktok.com/v2/auth/authorize/?client_key=$CLIENT_KEY&scope=$SCOPES&response_type=code&redirect_uri=$REDIRECT&state=$STATE"

# 2. After approving, TikTok redirects to http://localhost:8000/callback?code=<CODE>&state=...
#    Copy the `code` value from the URL.

# 3. Exchange code for an access token
curl -s -X POST 'https://open.tiktokapis.com/v2/oauth/token/' \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  -d "client_key=$CLIENT_KEY&client_secret=$CLIENT_SECRET&code=<PASTE_CODE>&grant_type=authorization_code&redirect_uri=$REDIRECT"
```

The response includes `access_token`, `refresh_token`, `expires_in` (seconds — typically 24h), and `refresh_expires_in` (typically 365d).

### Step 0.3 — Persist credentials to `~/.zshrc`

Same pattern as `OPUSCLIP_API_KEY`. Append to `~/.zshrc`:

```sh
export TIKTOK_CLIENT_KEY="<client key>"
export TIKTOK_CLIENT_SECRET="<client secret>"
export TIKTOK_ACCESS_TOKEN="<access token from step 0.2>"
export TIKTOK_REFRESH_TOKEN="<refresh token from step 0.2>"
export TIKTOK_TOKEN_EXPIRES_AT="<unix timestamp = now + expires_in>"
```

Then `source ~/.zshrc` (or open a new shell). Verify:

```sh
echo $TIKTOK_ACCESS_TOKEN | head -c 12 && echo "..."
```

### Step 0.4 — Confirm the skill can see the token

Once `$TIKTOK_ACCESS_TOKEN` is set, run `/tiktok-stats --auth-check` (skeleton command below). It will GET `/v2/user/info/?fields=display_name,follower_count,video_count` and print the result. If that succeeds, **remove the "NOT YET FUNCTIONAL" banner at the top of this file** and Phase 1 unlocks.

### Step 0.5 — Token refresh

Access tokens expire in ~24h. Phase 1 must check `$TIKTOK_TOKEN_EXPIRES_AT` and, if within 5 minutes of expiry, call:

```sh
curl -s -X POST 'https://open.tiktokapis.com/v2/oauth/token/' \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  -d "client_key=$TIKTOK_CLIENT_KEY&client_secret=$TIKTOK_CLIENT_SECRET&grant_type=refresh_token&refresh_token=$TIKTOK_REFRESH_TOKEN"
```

The response gives a fresh `access_token` + new `expires_in`. Rewrite the two lines in `~/.zshrc` and re-source. (The fetcher will eventually do this automatically; for now expect to refresh manually every day or two.)

## Phase 1 — Fetch recent videos (SKELETON, not wired)

Once Phase 0 is complete, the fetcher hits the **Display API** `/v2/video/list/` endpoint. Pseudocode:

```ts
// scripts/fetch.ts (TO BE WRITTEN once auth lands)
const cfg = loadConfig(); // config.local.json overrides config.json
const token = process.env[cfg.access_token_env];
if (!token) {
  console.error("TIKTOK_ACCESS_TOKEN not set — see Phase 0 in SKILL.md");
  process.exit(1);
}

const fields = cfg.video_fields.join(",");
const url = `${cfg.api_base}${cfg.video_list_endpoint}?fields=${fields}`;
const cutoff = Date.now() / 1000 - cfg.window_days * 86400;

let cursor: number | undefined;
const videos: TikTokVideo[] = [];
while (videos.length < cfg.max_posts_per_scrape) {
  const res = await fetch(url, {
    method: "POST",
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify({
      max_count: 20, // API max per page
      ...(cursor ? { cursor } : {}),
    }),
  });
  const json = await res.json();
  if (json.error?.code && json.error.code !== "ok") {
    throw new Error(`TikTok API error: ${json.error.code} ${json.error.message}`);
  }
  for (const v of json.data.videos) {
    if (v.create_time < cutoff) return videos; // hit the window edge
    videos.push(v);
  }
  if (!json.data.has_more) break;
  cursor = json.data.cursor;
}
return videos;
```

**Endpoint reference (confirmed via TikTok developer docs):**

- `POST https://open.tiktokapis.com/v2/video/list/` — paginated list of the authorized user's own posts. Body: `{ max_count, cursor? }`. Query: `?fields=<comma-sep-list>`. Requires scope `video.list`.
- `GET https://open.tiktokapis.com/v2/user/info/?fields=...` — profile-level stats (follower_count, likes_count, video_count). Requires scopes `user.info.basic` + `user.info.stats`.

Fields available on a video object: `id`, `create_time` (unix seconds), `video_description`, `title`, `duration`, `cover_image_url`, `share_url`, `view_count`, `like_count`, `comment_count`, `share_count`, `embed_html`, `embed_link`.

> **Uncertainty note:** TikTok has reorganized their API surface several times. The Display API `/v2/video/list/` shown above is the **current** surface as of the dev portal's published spec for personal/business apps. There is also a separate **Research API** (`/v2/research/video/query/`) that requires academic-style approval and is not the right path for a personal-analytics use case. If `/v2/video/list/` ever returns 404 / 410, check the dev portal for a renamed endpoint before assuming the token is broken.

## Phase 2 — Extract closed-loop tags from captions

Same convention as `_shared/post-manifest/README.md`. Every caption that originated from an instrumented scheduler (e.g. `opus-clips`) has a footer tag like `[opus:La4Wghg6IX]`. Pulling that ID out makes the cross-skill join possible.

```ts
const TAG_RE = /\[(opus|lp|gh|bh):([A-Za-z0-9_-]+)\]/g;

function extractTags(caption: string): { scheme: string; id: string }[] {
  const out: { scheme: string; id: string }[] = [];
  for (const m of caption.matchAll(TAG_RE)) {
    out.push({ scheme: m[1], id: m[2] });
  }
  return out;
}
```

A given post may have **zero or more** tags. Zero is normal for organic posts the user wrote by hand or that came from a non-instrumented surface. Multiple is rare but possible (e.g. an opus-clip about a specific PR could carry both `[opus:...]` and `[gh:...]`).

## Phase 3 — Snapshot output

Write `cache/snapshot-$(date -u +%Y-%m-%d).json`. Same top-level shape as `linkedin-stats` snapshots (`fetched_at`, `profile`, `recent_posts`) so `/flywheel` can consume both with one reader.

```jsonc
{
  "fetched_at": "2026-05-19T12:00:00Z",
  "platform": "tiktok",
  "handle": "mikelady",
  "profile": {
    "follower_count": 0,
    "following_count": 0,
    "likes_count": 0,
    "video_count": 0
  },
  "recent_posts": [
    {
      "post_id": "7234567890123456789",
      "posted_at": "2026-05-19T16:00:00Z",
      "caption": "Your next 50% productivity gain isn't a new AI tool ... [opus:La4Wghg6IX]",
      "share_url": "https://www.tiktok.com/@mikelady/video/7234567890123456789",
      "view_count": 0,
      "like_count": 0,
      "comment_count": 0,
      "share_count": 0,
      "duration_sec": 28,
      "closed_loop_tags": [
        { "scheme": "opus", "id": "La4Wghg6IX" }
      ]
    }
  ],
  "deltas_vs_prior_snapshot": {
    "prior_snapshot": "snapshot-2026-05-12.json",
    "follower_delta": 0,
    "total_views_delta": 0
  }
}
```

The cache directory is gitignored (see `.gitignore` in this skill dir).

## Phase 4 — Render report

Single markdown block (mirrors `linkedin-stats` output):

```
TikTok — weekly snapshot (YYYY-MM-DD)

Profile (@mikelady):
  Followers:     N (+Δ vs last week)
  Total likes:   N
  Video count:   N

Recent posts (last 14 days, top by views):
  YYYY-MM-DD  V views · L likes · C comments · S shares
    "<caption snippet>"  [opus:<id> | —]

Closed-loop coverage:
  Posts with opus tag: X / Y
  Posts with no tag:   Z   (organic / Buffer / manual)

Notes:
  - <flag any post with 10x average views — viral signal>
  - <flag any post with 0 views > 24h after posting — possibly shadow-banned>
```

## Downstream consumers

Once snapshots exist, these skills will read them:

- **`/flywheel`** — pulls `profile.follower_count` and the sum of `recent_posts[].view_count` into the weekly rollup, contributing to Priority 1 (long-form essays + newsletters + clips) throughput measurement.
- **`/buffer-stats`** — for the ~1/day historical Buffer→TikTok posts, joins on `caption` substring or `share_url` to attribute Buffer-scheduled posts (since Buffer Analyze can't). Will need a small bridge in `buffer-stats` to walk the TikTok snapshot.
- **`/opus-clips-performance`** (future) — walks `~/dev/youtube_analytics/data/opus_clips/*.json` manifests, finds entries with `scheduled_posts[].label` containing `TIKTOK`, then matches the `clip_id` against this snapshot's `recent_posts[].closed_loop_tags[].id` where `scheme === "opus"`. That join gives per-clip view counts — the actual ROI signal for the OpusClip native scheduler vs. Buffer-scheduled TikTok.

## Known issues / robustness notes

- **App review gate.** TikTok may require app review before activating the requested scopes for a freshly registered app. Until approved, `/v2/video/list/` returns `scope_not_authorized` even with a valid token. Expected during initial setup.
- **Token expiry.** 24h access token lifetime is short. Phase 0.5 covers manual refresh; auto-refresh is a follow-up (`scripts/refresh-token.ts`).
- **Rate limits.** TikTok's published rate limits are not consistent across docs versions. Default to **1 request/sec, 600/hour** as a defensive ceiling until empirically validated.
- **`view_count` lag.** TikTok view counts can lag 30–60 min behind reality, especially for posts under an hour old. The skill should flag any post with `posted_at` within the last hour as `view_count: <N> (still settling)`.
- **Captions are mutable.** Users can edit TikTok captions after posting. If a closed-loop tag is edited out, the join breaks. The skill snapshots captions on every run so historical joins remain stable even if the live caption changes.
- **OpusClip-scheduled posts may post under a different display name.** Verify the @handle returned by `/v2/user/info/` matches `config.json#tiktok_handle` on the first run — if OpusClip published under a sub-account, this skill won't see those posts.

## Config

Read from (in priority order):

1. `~/dev/claude-social-media-skills/tiktok-stats/config.local.json` (gitignored — personal overrides)
2. `~/dev/claude-social-media-skills/tiktok-stats/config.json` (committed defaults)

Fields:

- `tiktok_handle` — `@`-stripped handle (`mikelady`)
- `tiktok_profile_url` — full profile URL
- `api_base` — `https://open.tiktokapis.com`
- `video_list_endpoint` — `/v2/video/list/`
- `user_info_endpoint` — `/v2/user/info/`
- `oauth_scopes` — array of scope strings requested at OAuth time (informational)
- `video_fields` — array of field names passed to `?fields=` on `/v2/video/list/`
- `window_days` — how far back to fetch (default 14)
- `max_posts_per_scrape` — hard cap (default 50)
- `delta_window_days` — compare against newest snapshot older than this (default 7)
- `tag_regex` — closed-loop tag pattern
- `access_token_env` — name of the env var holding the access token (default `TIKTOK_ACCESS_TOKEN`)
