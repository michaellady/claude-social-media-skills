# Architecture

The skills in this repo are designed as a **closed loop**, not as independent one-shot tools. Every post the compose skills create is tagged at compose time — either with a Buffer `format:<name>` tag (Buffer-routed posts) or with an in-text `[scheme:id]` footer + JSON post-manifest (non-Buffer posts) — so the analytics skills can later attribute engagement back to the source content that produced it, and use that attribution to recommend skill-config changes for the next promotion cycle.

See also: [`CLOSED-LOOP-UNIFICATION-PLAN.md`](CLOSED-LOOP-UNIFICATION-PLAN.md) (the 2026-05-19 unification plan that this document now incorporates), [`PATTERNS.md` § Closed-loop post manifest](PATTERNS.md#pattern-closed-loop-post-manifest-for-non-buffer-scheduling), and [`_shared/post-manifest/README.md`](_shared/post-manifest/README.md).

## The loop

Two parallel attribution paths converge at `/flywheel`. The Buffer-routed path has been working since 2026-04-27; the non-Buffer path is being wired up as of 2026-05-18/19.

```
                Compose (promote-*, tease-*, carousel-*, opus-clips, …)
                                ↓
                  Adversarial review (fresh agent)
                                ↓
                  User review + publish (Phase 5+6)
                ┌───────────────┴───────────────┐
                ↓                               ↓
   Buffer-routed path                Non-Buffer path
   (4 of 6 compose skills)           (opus-clips today;
   - format:<name> Buffer tag         direct-publish future)
                ↓                    - [scheme:id] in-text tag
   audit-buffer-queue (weekly)       - post-manifest JSON ledger
                ↓                               ↓
   buffer-stats (Insights +          per-platform stats fetchers
   Analyze + GraphQL tagIds)         (yt-analytics, linkedin-stats,
                ↓                     tiktok-stats, threads-stats)
   per-(channel, format) ROI                    ↓
                └───────────────┬───────────────┘
                                ↓
                _shared/content-attribution/ (JOIN engine, in dev — #381)
                                ↓
                            /flywheel (per-source-content closed-loop report)
                                ↓
                Recommend skill changes (buffer-stats Phase 5b + flywheel ROI)
                                ↓
                User accepts → SKILL.md edits committed → next batch better
                                ↓
                              [loop]
```

## The skills, by role

### Compose-and-publish (write side)

| Skill | What it produces | Format tag |
|---|---|---|
| [`promote-newsletter`](promote-newsletter/SKILL.md) | Verbatim quotes from a beehiiv article fanned out to Buffer | `format:verbatim-quote` |
| [`tease-newsletter`](tease-newsletter/SKILL.md) | Original teaser hooks per channel (no verbatim drift) | `format:teaser` |
| [`carousel-newsletter`](carousel-newsletter/SKILL.md) | 10-slide illustrated carousel with Gemini-generated art | `format:carousel` |
| [`promote-github`](promote-github/SKILL.md) | Impact-framed posts about GitHub contributions | `format:link-share` (individual) or `format:batch-summary` (batched) |
| [`crosspost-newsletter`](crosspost-newsletter/SKILL.md) | Full-article syndication to LinkedIn pulse, Substack, Medium + link submissions to HN, Reddit | (none — publishes directly to platform native editors, not Buffer; closed-loop attribution comes from `linkedin-stats` for the LinkedIn pulse + accompanying post) |

Every compose-and-publish skill has these required phases:
- **Adversarial review** (spawn fresh subagent to audit drafts against source + skill rules before user sees them; catches fabrications)
- **Phase 6 — Buffer create_post** with `tagIds: [<format:<name> Tag ID>]` (attribution prereq — Buffer's `CreatePostInput` requires Tag IDs not name strings; lookup table at `_shared/buffer-post-prep/tag-ids.local.json`)

### Measure (read side)

| Skill | Surface scraped | Output | Status |
|---|---|---|---|
| [`buffer-stats`](buffer-stats/SKILL.md) | Buffer Insights (cross-channel, all 6 channels) + Buffer Analyze (per-post, 3 channels) + Buffer GraphQL (tagIds via MCP `list_posts`) | Per-(channel, format) engagement + channel ROI bucketing + auto skill recs | Live |
| [`linkedin-stats`](linkedin-stats/SKILL.md) | linkedin.com/dashboard/ + /analytics/creator/* | Followers + impressions + per-post engagement deltas (Phase 3b lands per-post for LinkedIn personal) | Live |
| [`yt-analytics`](https://github.com/oven-sh/youtube_analytics) (external Go binary at `~/dev/youtube_analytics/`) | YouTube Data API + Analytics API | Per-video metrics; reads OpusClip in-text `[opus:<clip_id>]` tag in Short descriptions | Live |
| [`threads-stats`](threads-stats/SKILL.md) | Meta Graph API (Threads Insights) | Per-post engagement for Threads profiles | **Scaffold-only** (OAuth gated; pending #373-class task) |
| [`tiktok-stats`](tiktok-stats/SKILL.md) | TikTok Business API | Per-post engagement for TikTok business | **Scaffold-only** (OAuth gated) |
| [`flywheel`](flywheel/SKILL.md) | Aggregates everything above + beehiiv via `_shared/content-attribution/` JOIN engine | Weekly priorities-keyed report w/ per-source-content ROI (Phase 4.55 — in dev) | Partial — JOIN layer pending #381 |

### Hygiene + adapt (close-the-loop side)

| Skill | What it does |
|---|---|
| [`audit-buffer-queue`](audit-buffer-queue/SKILL.md) | Inspects the queue for bunching, theme over-saturation, untagged posts, dead channels, below-threshold channels. Recommends 1-click cancel/reschedule/tag actions. |
| `buffer-stats` Phase 5b | Auto-generates skill-config recommendations from this week's format-performance data. User reviews → accepts → triggers SKILL.md edits → commits. |

## The format tag values

These are the only valid `format:<name>` tag values as of 2026-04-27. The compose skills tag posts with these; `buffer-stats` Phase 5 groups posts by these for engagement attribution.

| Tag | Produced by | What it represents |
|---|---|---|
| `format:verbatim-quote` | promote-newsletter | Direct excerpts from a newsletter article |
| `format:teaser` | tease-newsletter | Original copy summarizing without spoiling |
| `format:carousel` | carousel-newsletter | 10-slide illustrated swipe post |
| `format:link-share` | promote-github | Single GitHub contribution as a post |
| `format:batch-summary` | promote-github | Multiple contributions unified by a theme sentence |
| `format:long-form-pulse` | (future-reserved) | Reserved for a future skill that schedules a Buffer companion post for a published LinkedIn pulse article. `crosspost-newsletter` publishes pulse articles directly to LinkedIn, NOT via Buffer; pulse-post engagement is attributed via `linkedin-stats` instead of `buffer-stats` |

If you add a new compose skill, define a new format tag and update this table + `buffer-stats` Phase 5's expected tag list.

## Engagement data sources

The repo touches three distinct Buffer surfaces plus a growing set of platform-native APIs. They are NOT interchangeable; each covers a different slice. Conflating them was the root cause of several attribution gaps before 2026-05-18.

| Source | URL / API | Granularity | Coverage | Notes |
|---|---|---|---|---|
| **Buffer Insights** | `publish.buffer.com/insights` | Aggregate per channel (30d window) | All 6 channels including Threads + LinkedIn personal | Only source that surfaces Threads + LinkedIn-personal engagement; no per-post |
| **Buffer Analyze** | `analyze.buffer.com` | Per-post (impressions, reach) | IG business + FB page + LinkedIn page only | Does NOT cover Threads, LinkedIn personal, or any non-Buffer surface |
| **Buffer GraphQL** (via MCP `list_posts`) | Buffer Public API | Per-post metadata only (text, channelId, tagIds) | All Buffer-routed posts | No engagement counts — the tagIds JOIN (#371) bridges per-post metadata to per-post engagement |
| **Platform-native APIs / scrapes** | per-platform | Per-post engagement | YouTube (yt-analytics), LinkedIn personal (linkedin-stats), Threads (threads-stats, scaffold), TikTok (tiktok-stats, scaffold) | Required for everything Buffer Analyze can't see |

A canonical snapshot shape lives at `buffer-stats/cache/snapshot-<date>.json`. Each `channels[i].engagement` block declares `available: true/false` with a `reason` — consumers must check this rather than assume coverage.

## Closed-loop paths

Two parallel attribution paths feed the same downstream analysis. Use whichever matches how the post got published.

### Buffer-routed path (existing — working since 2026-04-27)

```
compose skill → mcp__buffer__create_post(tagIds: [<format:<name>>])
              → Buffer publishes
              → buffer-stats reads Insights + Analyze + GraphQL tagIds
              → per-(channel, format) ROI
```

Used by: `promote-newsletter`, `tease-newsletter`, `carousel-newsletter`, `promote-github`.

### Non-Buffer path (new since 2026-05-18)

```
compose skill → publishes via OpusClip native / LinkedIn pulse API / direct
              → writes post-manifest JSON ledger (api response IDs)
              → embeds [scheme:id] in-text tag in post body
              → per-platform stats fetcher reads platform API
              → JOIN engine (_shared/content-attribution/) correlates back to source
```

Used by: `opus-clips` (today), `crosspost-newsletter` (writes manifest; LinkedIn pulse engagement via linkedin-stats). Future direct-publishing skills follow the same pattern.

The in-text tag is **defense in depth** — the manifest is machine-readable but lives on disk; the in-text tag travels with the post itself so a future fetcher can recover the source link via platform search even if the manifest is unavailable. Schemes: `opus:` (OpusClip clip), `lp:` (LinkedIn pulse), `gh:` (GitHub PR/SHA), `bh:` (beehiiv slug). See [`_shared/post-manifest/README.md`](_shared/post-manifest/README.md) for the JSON shape + sourceable bash helpers.

## Per-source-content attribution (the unified analysis layer)

The data-collection skills (yt-analytics, buffer-stats, linkedin-stats, tiktok-stats, threads-stats) stay platform-specific — each writes its own snapshot, none talk to each other. What unifies the loop is a separate **analysis layer** that JOINs across snapshots by the keys above.

Three layers, clear contracts:

1. **Data collection** (existing/planned `*-stats` skills) — platform-specific fetchers. Each writes a snapshot under a known path (`<skill>/cache/snapshot-*.json` or `~/dev/youtube_analytics/data/`). No skill reads another's cache.
2. **JOIN engine** (`_shared/content-attribution/`, in dev — #381) — reads snapshots + post-manifests, correlates by these keys in priority order:
   1. `[scheme:id]` in-text tag (highest signal)
   2. Buffer `format:<name>` tag (per-format aggregation)
   3. `scheduleId` / `postId` from post-manifest
   4. ±2h time-window (fallback)
   5. `?utm_content` URL param (future)
3. **Analysis** (`/flywheel` Phase 4.55 + `/opus-clips-performance`) — consumes JOIN output, applies user priority weighting + ROI bucketing, renders the report.

Net effect: a long-form essay's true ROI = source-video metrics PLUS every derivative clip's metrics across every platform. Today the OpusClip→YouTube-Short→LinkedIn-personal chain has all three snapshots but no JOIN; #380/#381 close that gap. Full design + worked example in [`CLOSED-LOOP-UNIFICATION-PLAN.md`](CLOSED-LOOP-UNIFICATION-PLAN.md).

## Dead channel awareness

Buffer Insights surfaces engagement per channel over a 30-day window. A channel that publishes consistently but produces zero reactions/impressions over that window is **dead** — continuing to fan out to it burns compose-skill budget for no return.

Current dead channels (as of 2026-05-19, from `buffer-stats/cache/snapshot-2026-05-18.json` + Insights 30d):

| Channel | Posts (30d) | Reactions (30d) | Verdict |
|---|---|---|---|
| Threads `enterprisevibecode` | 64 | 0 | Dead — pause fan-out; investigate algorithmic / content-fit cause |

When a dead channel is identified, document the investigation at `audits/<channel>-dead-channel.md` (e.g. `audits/threads-enterprisevibecode-dead-channel.md`) — root cause, evidence, remediation options, decision. `audit-buffer-queue` flags below-threshold channels each weekly run; the audit doc is the deeper one-time investigation. `/flywheel` channel-ROI scoring (yellow_mid / red_dead buckets in `channel_roi[]`) is the surfacing mechanism.

## Defaults baked in 2026-04-27

These were derived from the LinkedIn /dashboard/ + Buffer Insights data collected during one heavy promotion week. They're the loop's *current state* — not eternal truths. The expectation is they shift as more data flows through the system.

| Default | Value | Reason (data citation) |
|---|---|---|
| `max_posts_per_channel_per_article` | **ask user up-front** ("1/ch, 3/ch, or all snippets/ch?") | Buffer Insights: reactions ↓52% M-o-M while posts ↑24.5% suggests caution past ~3/ch — but major-launch articles often want max saturation. Surface the choice rather than baking in a number; the user picks per-article. |
| `min_followers_to_promote` | 50 | EVC LinkedIn page (28 followers) got max 54 imps per post and +1 follower in 8 days — not worth fan-out cost |
| LinkedIn channels default to `tease-newsletter` | (over `promote-newsletter`) | Top 3 LinkedIn posts past 7d were 0% verbatim quotes; LinkedIn pulse (essentially a teaser) ranked #1 by impressions |
| LinkedIn pulse runs FIRST in `crosspost-newsletter` | (Phase 4 platform order) | LinkedIn pulse drove the #1-impressions LinkedIn post within hours; primes algorithm for later carousel/snippet posts |
| Carousel runs AFTER pulse | (`carousel-newsletter` "When to use" section) | Re-engages a primed audience rather than a cold one |
| Adversarial review required in every compose skill | (architecture rule) | User caught a fabrication ("every leader I respect keeps a token on their desk") manually on 2026-04-26 — agent should catch the next one automatically |

## Why this matters

Without the closed loop, every promotion run is a one-shot decision based on intuition. Run `/promote-newsletter`, hope it works, never know if it did.

With the closed loop, **every promotion run feeds the next one's defaults.** The system gets better as more data flows through it. The first round of defaults (above) was derived from one week of data; subsequent rounds will refine them as the format-performance evidence accumulates.

## When you add a new skill to this repo

If the new skill **creates posts** (compose-and-publish):
1. Define a new format tag (`format:<name>`) and add it to the table above
2. Tag every Buffer post the skill creates with `tagIds: [<format:<name> Tag ID>]` at `mcp__buffer__create_post` time. **Important:** Buffer's `CreatePostInput` schema requires Tag IDs (24-char hex), not name strings. The `tags: [...]` field is silently dropped. Tag IDs are per-organization and live in `_shared/buffer-post-prep/tag-ids.local.json` (gitignored). New tags must be created in Buffer's web UI first (no public `createTag` mutation); the buffer-post-prep helper does the name→ID lookup automatically. See `_shared/buffer-post-prep/README.md` for the one-time setup.
3. Add an Adversarial Review step (spawn a fresh subagent to audit drafts against source + skill rules before user sees them) — use existing skills as templates
4. Document the format in `buffer-stats` Phase 5's expected-tags list

If the new skill **reads engagement** (measure side):
1. Output should produce a per-(channel, format) engagement aggregation
2. Cache snapshots to a gitignored `cache/` directory keyed by date
3. Surface week-over-week deltas vs the prior cached snapshot
4. Feed into `flywheel` via stable JSON shape

If the new skill is a **closing-the-loop tool** (hygiene, recommendation, audit):
1. Read the latest `buffer-stats` snapshot
2. Don't re-scrape the underlying surfaces (slow); use the cached data
3. Output recommendations as actionable JSON with citation back to the data point that justifies the recommendation

## Related memory

For Claude sessions running with this user's profile: see `~/.claude/projects/-Users-mikelady-dev-claude-social-media-skills/memory/feedback_closed_loop_architecture.md` for the same architecture from the session-context perspective.
