# Threads Stats — Feasibility Investigation

**Date:** 2026-05-19
**Task:** #374 — Determine whether per-post engagement on Threads is fetchable via API for closed-loop attribution on the `mikelady` and `enterprisevibecode` accounts.
**Verdict:** **FEASIBLE.** Recommend implementation in the same shape as `/linkedin-stats`.

## Current API Capabilities (2026)

Meta ships a first-party Threads Graph API with a dedicated per-post insights endpoint that covers exactly the metrics we need for closed-loop attribution. The relevant endpoint is `GET https://graph.threads.net/v1.0/{threads-media-id}/insights?metric=views,likes,replies,reposts,quotes,shares&access_token=...`. Per-post metrics returned are `views`, `likes`, `replies`, `reposts`, `quotes`, and `shares` (the last two are the newest additions; `views` and `shares` are marked "in development" in Meta's docs but are live in practice). To enumerate the posts to ask about, use `GET /v1.0/me/threads` and page through `id`s. Account-level metrics (`followers_count`, follower demographics) are also available via `GET /v1.0/me/threads_insights`. Known limitation: nested-reply metrics are not aggregated into the root post, and `REPOST_FACADE` items return empty arrays (posts you reposted from others — not ours, so irrelevant).

Auth is OAuth 2.0 against `https://threads.net/oauth/authorize` → `POST https://graph.threads.net/oauth/access_token`. Scopes required: `threads_basic` + `threads_manage_insights`. Short-lived tokens last 1 hour; exchange for a long-lived token (60-day TTL, refreshable via the `th_refresh_token` grant before expiry — same pattern as the IG Graph API). Each Threads account needs its own token, so we'll run the flow twice (once for `mikelady`, once for `enterprisevibecode`). Rate limit on publishing is 250 posts/24h; insights reads fall under the standard Graph API per-app/per-user call budget (no posted hard cap for read-only metric polling at our volume — daily polling of ~hundreds of posts is well within the platform's BUC limits).

**App review caveat — and why it does not block us:** the `threads_manage_insights` permission normally requires Meta App Review (2–4 weeks, screencasts, privacy policy). However, in **development mode** the app owner and any users added as app testers can call all permissioned endpoints against **their own** accounts without review. Since we only need our two owned accounts, we register both as testers on a single Meta developer app and skip the review process entirely. This is the same loophole the LinkedIn skill relies on (personal-app posture). If we ever want to read someone else's Threads, that's when review kicks in — not our use case.

## Recommended Implementation Path

Mirror `/linkedin-stats` structure:

- `threads-stats/SKILL.md` — daily-pull, per-post-metrics-by-id, weekly-rollup commands
- `threads-stats/config.json` — `{ accounts: [{ handle, user_id, long_lived_token, token_expires_at }], app: { client_id, client_secret } }`
- `threads-stats/cache/` — JSON-per-post snapshots keyed by Threads media id, with a `history[]` array so we can chart engagement-over-time deltas
- Token refresh: lazy on read; if `token_expires_at < now + 7d`, hit the refresh grant before the data call
- Closed-loop join: match cached posts to Buffer-sent posts via the `[opus:<id>]` in-text tag the user already embeds, exactly like `/linkedin-stats` does
- Surface in `/flywheel` Priority 1 alongside LinkedIn newsletter and YT analytics

**Setup cost (one-time, ~1 hour):** create Meta developer app → enable Threads use case → add both Threads accounts as testers → walk OAuth once per account → store long-lived tokens in `config.json`. After that it's just daily polling.

## Fallback (only if setup blocks)

If the Meta app/tester flow turns out to be unexpectedly painful, the `[opus:<id>]` in-text tag in every Threads post we publish via Buffer is sufficient for manual recovery — user pastes engagement numbers from the Threads app once a week and we backfill the cache. But this is plan B; the API path is genuinely cheap.

## Sources Consulted

- https://developers.facebook.com/docs/threads/insights/ — per-post insights endpoint, metrics list, REPOST_FACADE limitation
- https://developers.facebook.com/docs/threads/get-started/get-access-tokens-and-permissions/ — OAuth flow, scopes
- https://developers.facebook.com/docs/threads/get-started/long-lived-tokens/ — 60-day long-lived tokens, `th_refresh_token` grant
- https://developers.facebook.com/docs/development/create-an-app/threads-use-case/ — development-mode tester access without app review
- https://www.socialmediatoday.com/news/meta-elements-threads-api-drive-engagement/721689/ — 2026 update adding `shares` metric
- https://www.ayrshare.com/blog/threads-api-integration-authorization-posting-analytics-with-ayrshare/ — third-party confirmation of metric coverage and OAuth pattern
- https://zernio.com/blog/threads-api — 2026 developer guide, rate-limit context
