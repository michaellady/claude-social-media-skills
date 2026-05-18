---
name: flywheel
description: Use when user wants a weekly dashboard of the Enterprise Vibe Code growth flywheel — "flywheel report", "weekly rollup", "how's the flywheel spinning", "am I on track", "week over week growth", "priorities check". Produces a single markdown report against the 5 growth-plan priorities.
user_invocable: true
---

# flywheel

Aggregate signal from YouTube, beehiiv, LinkedIn, and the consulting log into one weekly report against the 5 priorities in `~/dev/youtube_analytics/enterprise_vibe_code_growth_priorities.md`. Answers "is the flywheel spinning this week?" with specific numbers, not vibes.

## Usage

`/flywheel` — read cached snapshots + compose report (fast — ~30 sec). Flags any source older than `stale_snapshot_days` (default 14d) but does NOT refresh.
`/flywheel --refresh` — refresh ALL upstream snapshots first (yt-analytics fetch + fetch-analytics --all + cohort auto, buffer-stats engagement scrape, linkedin-stats dashboard scrape), then compose. **The canonical Sunday weekly review** — single command, single artifact, everything fresh. ~10-15 min wall-clock end-to-end.
`/flywheel --refresh-stale` — only refresh upstream snapshots older than `stale_snapshot_days`. Cheaper than `--refresh`; flags any sources still stale after the conditional refresh.
`/flywheel --days 30` — custom window
`/flywheel --no-save` — produce the report but don't overwrite today's snapshot
`/flywheel --compare 2026-04-12` — diff against a specific older snapshot

### When to use which mode

- **Daily / mid-week check:** `/flywheel` (cached, fast — answers "where do we stand right now")
- **Sunday weekly review:** `/flywheel --refresh` (canonical — everything fresh, replaces running each sub-skill manually)
- **Catch-up after a few days:** `/flywheel --refresh-stale` (only refreshes what's actually stale)

## 🟢 Happy Path (read first; everything below is edge-case detail)

The Sunday weekly review flow when every source is healthy. ~10-15 min wall-clock with `--refresh`, ~30 sec cached.

**Phase 0 — Refresh upstream snapshots (skip on plain `/flywheel`).** On `--refresh`, run all three sub-skills in sequence; on `--refresh-stale`, run only those whose cache is older than `stale_snapshot_days` (default 14d):
- YouTube: `cd ~/dev/youtube_analytics && go run . fetch && go run . fetch-analytics --all && go run . cohort auto` (~3-5 min, no browser).
- Buffer: invoke `/buffer-stats` (~3-5 min, gstack browser; cookies carry from buffer.com — see `Edge: buffer-snapshot-stale`).
- LinkedIn: invoke `/linkedin-stats` (~2-3 min, gstack browser — see `Edge: linkedin-snapshot-stale`).

**Phase 1 — Resolve window.** Default `DAYS=7`; compute `SINCE` / `UNTIL` and `REPORT=$SNAP_DIR/$(date -u +%Y-%m-%d).md`.

**Phase 2 — YouTube.** `go run . analyze --since $SINCE` in `~/dev/youtube_analytics` (reads `data/videos.json` — see `Edge: youtube-videos-json-stale`). Grep the formatted output for streams/long-form/shorts/views/revenue/subs-gained. Compute Priority 1 (long-form 2-3/wk — counts essays + newsletters) + Priority 4 (1 livestream/wk as supplement, not primary). **Strategy pivoted 2026-05-18** — see `project_content_strategy_pivot_2026_05_18.md` memory.

**Phase 3 — beehiiv.** Two MCP calls: `beehiiv_stats` (current subs + delta) and `beehiiv_attribution` (source mix). If the tool is missing, hit `Edge: beehiiv-mcp-restart-required`. Compute Priority 2 pace toward 1,800 in 12 months and YouTube attribution %.

**Phase 4 — LinkedIn.** Read the latest `~/dev/claude-social-media-skills/linkedin-stats/cache/snapshot-*.json` (cached only — don't re-scrape). Pull newsletter subs, profile followers, company followers for Priority 3.

**Phase 4.5 — Buffer.** Read the latest `~/dev/claude-social-media-skills/buffer-stats/cache/snapshot-*.json`. Render the buffer-tracked subset as `BF_BUFFER_TRACKED_FOLLOWERS` — never call it the cross-channel total.

**Phase 4.7 — Cross-source reconciliation.** Compose the cross-channel reach table from the authoritative source per channel (LinkedIn personal/page → linkedin-stats; IG/FB → buffer-stats; YouTube → yt-analytics; beehiiv → beehiiv-mcp; Threads → unavailable). Annotate each row with its source.

**Phase 4.6 — Channel ROI.** Per Buffer-connected channel: `channel_roi_score = (avg_impressions_per_post * eng_rate * 100) / (sent_count + 1)`. Bucket into 🟢/🟡/🔴/⚪ and render the ROI table.

**Phase 5 — Consulting pipeline.** `(cd ~/dev/consulting-log && ./cl json)` (local-only — see `Edge: consulting-log-local-only`). Aggregate pipeline / realized revenue / content gaps for Priority 5.

**Phase 6 — Compose report.** Write the fixed-structure markdown to `$REPORT` and print to stdout. Also write the parallel `$SNAP_DIR/<date>.json` for bulletproof week-over-week diffing.

**Phase 7 — Week-over-week diff.** If `$SNAP_DIR/$(date -v-7d).md` exists, diff key numbers into the "Week-over-week delta" section.

## Data sources

| Source | How | What it gives |
|---|---|---|
| YouTube | `go run . analyze --since <date>` in `~/dev/youtube_analytics` | streams/week, long-form count, views, revenue, subs |
| beehiiv list | `mcp__beehiiv__beehiiv_stats` | current subscriber count |
| beehiiv attribution | `mcp__beehiiv__beehiiv_attribution` | source mix (YouTube vs LinkedIn vs direct) |
| LinkedIn | `~/dev/claude-social-media-skills/linkedin-stats/cache/snapshot-<latest>.json` | newsletter subs, profile + page followers |
| Buffer | `~/dev/claude-social-media-skills/buffer-stats/cache/snapshot-<latest>.json` | per-channel followers/engagement for IG/FB/Threads fan-out + queue health |
| Consulting | `(cd ~/dev/consulting-log && ./cl json)` | pipeline stages, realized revenue, content gaps |

If any source fails or is stale, note it in the report — don't silently drop the row.

## Process

### Phase 0 — (Optional) Refresh upstream snapshots

Skip when `/flywheel` was invoked without a refresh flag. Otherwise, decide which sub-skills need to run BEFORE composing the report.

**`--refresh` (full):** run all three unconditionally.
**`--refresh-stale`:** check each cache's mtime against `stale_snapshot_days`; only refresh the stale ones.

Sub-skill invocation order (each has its own auth + scrape; they don't share session):

1. **YouTube data refresh** (~3-5 min, no browser):
   ```bash
   cd ~/dev/youtube_analytics
   go run . fetch                       # video metadata, snapshots automatically
   go run . fetch-analytics --all       # aggregate + per-day + traffic-sources + sub-status
   go run . cohort auto                 # refresh cohort assignments from rules
   ```
   Cached at `data/videos.json` + `data/snapshots/videos-<UTC>.json`.

2. **Buffer engagement refresh** (~3-5 min, gstack browser):
   Invoke `/buffer-stats` skill. It writes `~/dev/claude-social-media-skills/buffer-stats/cache/snapshot-<date>.{json,md}`.
   Auth: `cookie-import-browser chrome buffer.com` (one-time picker click); cookies carry from buffer.com to publish.buffer.com and analyze.buffer.com.

3. **LinkedIn refresh** (~2-3 min, gstack browser):
   Invoke `/linkedin-stats` skill (or scrape `linkedin.com/dashboard` directly for the headline numbers if the full skill isn't required). Writes `~/dev/claude-social-media-skills/linkedin-stats/cache/snapshot-<date>.json`.
   Auth: gstack browser must be logged in to LinkedIn (cookies usually carry from a prior session).

4. **YouTube weekly review** (closed-loop, optional):
   ```bash
   cd ~/dev/youtube_analytics
   go run . insights pending            # past-due hypotheses; grade them in the report's narrative
   go run . cohort report --since <last-monday>
   ```

If a sub-skill fails (auth lapsed, cookie picker not closed, gstack process dropped), surface the failure clearly and continue with the OTHER sub-skills + cached data for the failed one. Don't abort the whole flywheel composition over one stale source.

After Phase 0 completes (or is skipped), proceed to Phase 1 with the freshly-written cache files in scope.

### Phase 1 — Resolve window

```bash
DAYS=${DAYS:-7}
UNTIL=$(date -u +%Y-%m-%d)
SINCE=$(date -v-${DAYS}d -u +%Y-%m-%d 2>/dev/null || date -d "$DAYS days ago" -u +%Y-%m-%d)
SNAP_DIR=~/dev/flywheel-snapshots
mkdir -p "$SNAP_DIR"
REPORT="$SNAP_DIR/$(date -u +%Y-%m-%d).md"
```

### Phase 2 — YouTube analytics

Run the existing CLI and capture its report:

```bash
YT_REPORT=$(cd ~/dev/youtube_analytics && go run . analyze --since "$SINCE" 2>&1)
```

Extract key numbers from the output:
- total streams in window
- total long-form in window (non-shorts, non-live)
- total shorts in window
- views
- revenue
- net subs gained

The `analyze` output is human-formatted — use simple grep/awk to pluck numbers. If the format changes, fall back to counting video entries in `data/videos.json` directly with jq.

**Priority 1 check** (long-form 2-3/week — pivoted 2026-05-18 from "streams 3-4×/week"):
- `long_form_per_week = (actual_long_form_videos + actual_newsletters) / DAYS * 7`
- Newsletters count toward this — long-form essays and newsletters are the same priority. Pull newsletter count from beehiiv stats (Phase 3): `new_subs_in_window > 0 OR recent_posts contains item in window`.
- target: 2-3/week combined long-form output (essays + newsletters)
- status: on_track if ≥2, behind otherwise

**Priority 4 check** (1 livestream/week as community surface — pivoted 2026-05-18 from "long-form 2-3/week"):
- `streams_per_week = actual_lives / DAYS * 7`
- target: 1/week (was 3-4/week pre-2026-05-18)
- status: on_track if ≥1, OR if `long_form_per_week ≥ 3` (the priority is "keep the surface alive"; if long-form output is strong, skipping the stream is fine)
- Skipping streams entirely for >2 consecutive weeks should flag as 🟡 (not 🔴 — Priority 1 is the primary now)

### Phase 3 — beehiiv

Two tool calls:

```
mcp__beehiiv__beehiiv_stats (window_days: DAYS)
mcp__beehiiv__beehiiv_attribution (window_days: DAYS)
```

From `beehiiv_stats`:
- current subscriber count
- `delta_count` vs last snapshot (if history_sufficient)

From `beehiiv_attribution`:
- total new subs in window
- % from YouTube (the Priority 2 success metric)

**Priority 2 check** (push viewers to Beehiiv):
- target trajectory: from today's count to 1,800 in 12 months
- needed per week = (1800 - current) / 52 weeks
- actual this window = attribution.total_subs_in_window
- status: on_track if actual ≥ needed, behind otherwise
- Also note: youtube % of new subs (healthy if ≥50%, worrying if <30%)

### Phase 4 — LinkedIn

Read the latest cached snapshot instead of re-scraping every run (LinkedIn scraping is slow + interactive):

```bash
LN_CACHE=~/dev/claude-social-media-skills/linkedin-stats/cache
LATEST_LN=$(ls -1 "$LN_CACHE"/snapshot-*.json 2>/dev/null | tail -1)
if [ -n "$LATEST_LN" ]; then
  LN_NL_SUBS=$(jq -r .newsletter.subscribers "$LATEST_LN")
  LN_PROFILE_FOLLOWERS=$(jq -r .profile.followers "$LATEST_LN")
  LN_COMPANY_FOLLOWERS=$(jq -r .company.followers "$LATEST_LN")
  LN_SNAP_DATE=$(basename "$LATEST_LN" .json | sed 's/snapshot-//')
else
  LN_NL_SUBS="unknown — run /linkedin-stats"
fi
```

If the latest LinkedIn snapshot is >14 days old, flag it — stale LinkedIn data is less useful than no LinkedIn data.

**Priority 3 check** (cross-post newsletter to LinkedIn weekly):
- requires evidence that a LinkedIn article was published in the window
- heuristic: newsletter subscriber count increased ≥N since last snapshot → posting is active
- if user wants per-article engagement, they run `/linkedin-stats` separately

### Phase 4.5 — Buffer (IG/FB/Threads fan-out)

Read the latest cached Buffer snapshot. Don't re-run `/buffer-stats` here — it's slow (scrapes Buffer Analyze) and the user runs it weekly:

```bash
BF_CACHE=~/dev/claude-social-media-skills/buffer-stats/cache
LATEST_BF=$(ls -1 "$BF_CACHE"/snapshot-*.json 2>/dev/null | tail -1)
if [ -n "$LATEST_BF" ]; then
  BF_SNAP_DATE=$(basename "$LATEST_BF" .json | sed 's/snapshot-//')
  # CRITICAL: distinguish "channels Buffer Analyze can scrape engagement for"
  # (subset — only FB pages, IG business, LinkedIn pages — NOT LinkedIn personal,
  # NOT Threads) from "channels we post to" (full set, includes everything).
  # Conflating these caused the 2026-05-03 flywheel report to show
  # total_followers=26 when actual was ~2,200 across all surfaces.
  BF_ENGAGEMENT_TRACKED_CHANNELS=$(jq -r '.engagement_tracked_channels // (.channels | length)' "$LATEST_BF")
  BF_POSTING_CHANNELS=$(jq -r '.posting_channels // .channels_active // (.channels | length)' "$LATEST_BF")
  BF_BUFFER_TRACKED_FOLLOWERS=$(jq -r '[.channels[].engagement.followers // 0] | add' "$LATEST_BF")
  BF_TOTAL_FOLLOWERS_DELTA=$(jq -r '[.channels[].engagement.followers_delta // 0] | add' "$LATEST_BF")
  BF_AVG_ENG_RATE=$(jq -r '[.channels[].engagement.engagement_rate // 0] | add / length' "$LATEST_BF")
  BF_TOP_POST=$(jq -r '.top_posts[0] | "\(.service): \(.text_snippet) (\(.engagement) engagement)"' "$LATEST_BF")
  # Stale-data flag (same 14-day threshold as LinkedIn)
  BF_STALE=$(( $(date -u +%s) - $(date -j -f "%Y-%m-%d" "$BF_SNAP_DATE" +%s 2>/dev/null || date -d "$BF_SNAP_DATE" +%s) > 14*86400 ))
else
  BF_BUFFER_TRACKED_FOLLOWERS=""; BF_STALE=1
fi
```

**Render the buffer-tracked subset as `BF_BUFFER_TRACKED_FOLLOWERS`, NOT as a channel-wide total.** The difference matters: today (2026-05-03) `BF_BUFFER_TRACKED_FOLLOWERS=26` (FB page + IG business + LinkedIn page only) but the actual cross-channel follower count is ~2,200 (LinkedIn personal alone is 2,104). Reporting "Total followers: 26" misleads.

If the latest Buffer snapshot is >14 days old, flag it. Note that Buffer is the fan-out layer (Priority 2's "push viewers to Beehiiv" uses Buffer as the distribution surface for IG/FB/Threads), so its health informs Priority 2's attribution mix — if IG/Threads followers are growing but beehiiv attribution shows 0% from those surfaces, that's a link-in-bio / call-to-action problem, not a Buffer problem.

### Phase 4.7 — Cross-source follower reconciliation

Each surface has its own follower count source — reconcile them before reporting any "total followers" number:

| Surface | Authoritative source | Why |
|---|---|---|
| LinkedIn personal | `linkedin-stats` (LinkedIn dashboard scrape) | Buffer Analyze does not track LI personal |
| LinkedIn EVC page | `linkedin-stats` company section | Buffer Analyze tracks but linkedin-stats has the canonical number |
| Instagram (EVC) | `buffer-stats` (Buffer Analyze) | only source we have for IG followers |
| Facebook (EVC) | `buffer-stats` (Buffer Analyze) | only source for FB |
| Threads (×2) | none — neither source tracks Threads followers | report as "unavailable" |
| YouTube subscribers | `yt-analytics` (`subscribers_gained` + lifetime) | independent surface |
| Beehiiv | `beehiiv-mcp` `subscribers.current` | authoritative |

**Compose the cross-channel total from the authoritative source per channel — never sum buffer-tracked alone and call it the total.** Document each row in the output with its source so the user can audit:

```markdown
## Cross-channel reach (with provenance)

| Channel | Followers | Δ7d | Source |
|---|---:|---:|:---:|
| LinkedIn personal | 2,104 | +4 | linkedin-stats |
| LinkedIn EVC page | 28 | +1 | linkedin-stats |
| Instagram (EVC) | 512 | +3 | buffer-stats |
| Facebook (EVC) | — | — | buffer-stats (not surfaced) |
| Threads (mikelady + EVC) | unavailable | — | — |
| YouTube | 800 | +12 | yt-analytics |
| Beehiiv | 186 | +7 | beehiiv-mcp |
| **Total (excluding unavailable)** | **3,630** | **+27** | reconciled |
```

When two sources disagree on the same channel (e.g. linkedin-stats says LinkedIn page has 28 followers, buffer-stats says 23), prefer the source with the more recent fetch timestamp; flag the discrepancy in the output.

### Phase 4.6 — Channel ROI score

For each Buffer-connected channel, compute a `channel_roi_score` to surface deprioritization candidates. The score is a per-post engagement-weighted measure adjusted for queue cost:

```
channel_roi_score = (avg_impressions_per_post * eng_rate * 100) / (sent_count_in_window + 1)
```

Higher = more reach + engagement per post relative to how often we publish there.

Then categorize:
- `channel_roi_score >= 100` → 🟢 **High ROI** — keep current cadence, consider increasing.
- `10 <= score < 100` → 🟡 **Mid ROI** — current cadence is fine.
- `score < 10 AND followers < 50` → 🔴 **Below threshold** — recommend dropping from fan-out (the `min_followers_to_promote` config in promote-* skills should already handle this; surface as a reminder).
- `score < 10 AND followers >= 50` → ⚪ **Diminishing returns** — recommend reducing fan-out volume on this channel; consider routing the same content through `tease-newsletter` instead of `promote-newsletter`.

Render as part of the report:
```markdown
### Channel ROI

| Channel | Followers | Posts (Nd) | Avg imps | ROI | Status |
|---|---:|---:|---:|---:|:---:|
| LinkedIn personal | 2,104 | 8 | 230 | 287 | 🟢 High |
| Instagram (EVC) | 512 | 5 | 859 | 158 | 🟢 High |
| Facebook (EVC) | — | 12 | 730 | 47 | 🟡 Mid |
| LinkedIn page (EVC) | 28 | 8 | 23 | 0.4 | 🔴 Below threshold (recommend skip) |
```

Use this signal to inform Priority 3 ("cross-post the newsletter to LinkedIn weekly") — if LinkedIn personal is High ROI but LinkedIn page (EVC) is Below threshold, the priority should specifically target LinkedIn personal.

### Phase 5 — Consulting pipeline

```bash
CL_JSON=$(cd ~/dev/consulting-log && ./cl json 2>/dev/null)
if [ -z "$CL_JSON" ]; then
  CL_STATUS="no deals logged"
else
  # Aggregate by stage and compute content gaps.
  TOTAL_PIPELINE=$(printf '%s' "$CL_JSON" | jq '[.[] | .estimated_value] | add // 0')
  TOTAL_REVENUE=$(printf '%s' "$CL_JSON" | jq '[.[] | .actual_revenue] | add // 0')
  # Exclude deals tagged `nda` — NDA-blocked engagements can't produce content, so flagging them as gaps
  # is noise. Surface them separately so the user knows the deals exist but the gap count isn't padded
  # by work that's structurally unpublishable.
  GAPS=$(printf '%s' "$CL_JSON" | jq '[.[] | select((.status == "delivered" or .status == "closed") and (.content_pieces | length) == 0 and ((.tags // []) | index("nda") | not))] | length')
  NDA_BLOCKED=$(printf '%s' "$CL_JSON" | jq '[.[] | select((.status == "delivered" or .status == "closed") and (.content_pieces | length) == 0 and ((.tags // []) | index("nda")))] | length')
fi
```

**Priority 5 check** (every engagement → content):
- gaps == 0 means fully on track
- status: each gap is a broken flywheel link
- `NDA_BLOCKED` deals are reported separately as "NDA-blocked, no gap to close" rather than counted as gaps. If you find yourself with many NDA-blocked deals, consider adding a meta-narrative content piece ("how I structure NDA'd engagements", "what I price training at", etc.) that's publishable without breaching any specific deal.

**Render the Priority 5 section like this when NDA-blocked deals exist:**

```markdown
## Priority 5 — Every engagement → content
- Active pipeline: $X (N deals)
- Realized revenue this window: $Y
- Content gaps: N deals delivered without attached content pieces (excludes NDA-blocked)
- NDA-blocked (no gap to close): N deals — content publication is structurally blocked
- Status: 🟢 zero gaps | 🟡 N gaps | 🔴 ≥3 gaps
```

### Phase 6 — Compose report

Build the markdown with a fixed structure so snapshots are diffable week-over-week:

```markdown
# Enterprise Vibe Code — Flywheel Snapshot

**Window:** YYYY-MM-DD → YYYY-MM-DD (N days)
**Generated:** YYYY-MM-DDTHH:MM:SSZ

## Priority 1 — Ship 2-3 long-form pieces/week
- Long-form videos this window: N
- Newsletters this window: N
- Combined long-form output: N (target: 2-3/week)
- Pace: X/week
- Status: [🟢 on track ≥2 | 🟡 under | 🔴 off]

## Priority 2 — Push viewers to Beehiiv
- Current subs: N (target: 1,800 in 12mo)
- Net new this window: +M
- Attribution: YouTube X%, LinkedIn Y%, Direct Z%, …
- Pace to target: needed ~K/week, getting M/week
- Status: [🟢 | 🟡 | 🔴]

## Priority 3 — LinkedIn newsletter weekly
- Newsletter subs: N (as of LN snapshot YYYY-MM-DD)
- Profile followers: N
- Company page followers: N
- Status: [🟢 | 🟡 | 🔴 | ⚪ no recent LN data]

## Fan-out (Buffer) — cross-channel reach
- Channels active: N (as of BF snapshot YYYY-MM-DD)
- Total followers: N (+Δ this week)
- Avg engagement rate: X%
- Top cross-channel post: <service>: <snippet> (<N> engagement)
- Status: [🟢 fresh | ⚪ no recent Buffer data | 🟡 stale (>14d)]

## Priority 4 — 1 livestream/week as community + breakout surface
- Streams this window: N (target: 1/week)
- Pace: X/week
- Status: [🟢 ≥1/wk OR Priority 1 ≥3 this week | 🟡 0 this week, ≤2 consecutive weeks | 🔴 0 for 3+ consecutive weeks]
- Skipping streams when long-form output is strong is acceptable — this priority is "keep the surface alive," not "force a cadence at the cost of long-form."

## Priority 5 — Every engagement → content
- Active pipeline: $X (N deals)
- Realized revenue this window: $Y
- Content gaps: N deals delivered without attached content pieces
- Status: [🟢 zero gaps | 🔴 N gaps]

## Cross-priority notes
_(free-form observations: spikes, correlations, concerns)_

## Week-over-week delta
_(diff vs last snapshot if available)_

## Raw numbers
_(appendix: the actual JSON from each source, for audit)_
```

Write the file to `$REPORT`, print it to stdout so the user sees it immediately.

### Phase 7 — Week-over-week diff

If there's a snapshot from 7 days ago, diff the key numbers and surface them in the "Week-over-week delta" section:

```bash
PRIOR_REPORT=$(date -v-7d -u +%Y-%m-%d 2>/dev/null).md
PRIOR_PATH="$SNAP_DIR/$PRIOR_REPORT"
if [ -f "$PRIOR_PATH" ]; then
  # Extract the key numbers from last week's report and compare.
  # Simplest: grep the "Current subs" line, the "Streams this window" line, etc.
  # For robustness, store a machine-readable JSON alongside the markdown.
fi
```

Consider also writing a parallel `$SNAP_DIR/<date>.json` with the raw numbers to make diffing bulletproof:

```json
{
  "window": { "since": "...", "until": "..." },
  "youtube": { "streams": N, "long_form": N, "shorts": N, "views": N, "revenue": F, "subs_gained": N },
  "beehiiv": { "total_subs": N, "new_subs_in_window": N, "attribution": { "youtube": N, "linkedin": N, "...": N } },
  "linkedin": { "newsletter_subs": N, "profile_followers": N, "company_followers": N, "snapshot_date": "..." },
  "buffer": { "channels": N, "total_followers": N, "total_followers_delta": N, "avg_engagement_rate": F, "snapshot_date": "..." },
  "consulting": { "pipeline": F, "realized_revenue": F, "content_gaps": N }
}
```

Diffing JSON is trivially reliable even if the markdown structure evolves.

## Growth-plan hook

After a few weeks of running `/flywheel` every Sunday, the snapshots directory becomes a diff-able history of the flywheel's state. A future `/flywheel --trend` mode can plot week-over-week acceleration across all five priorities.

## Known issues

- **beehiiv MCP requires Claude Code restart** after `make install`ing the server. If the attribution tool is missing, remind the user.
  *Label: `Edge: beehiiv-mcp-restart-required`*
- **LinkedIn snapshots must be current.** If `/linkedin-stats` hasn't been run recently, Priority 3 will show stale numbers. The report should flag any snapshot older than 14 days as unreliable.
  *Label: `Edge: linkedin-snapshot-stale`*
- **Buffer snapshots must be current.** Same 14-day staleness threshold. If no snapshot exists, the fan-out section shows `⚪ no recent Buffer data` and prompts the user to run `/buffer-stats`. Buffer feeds the fan-out context for Priority 2 — if missing, Priority 2's attribution analysis loses the IG/FB/Threads signal.
  *Label: `Edge: buffer-snapshot-stale`*
- **YouTube data.** `youtube_analytics` `analyze` reads `data/videos.json` which is only refreshed on `fetch`. If it's stale, the YouTube section will be too. Run `go run . fetch` in `~/dev/youtube_analytics` before running `/flywheel` if the numbers look off.
  *Label: `Edge: youtube-videos-json-stale`*
- **Consulting log is local-only.** No data migrates from other tools. If the user uses a CRM, they have to update the markdown files themselves.
  *Label: `Edge: consulting-log-local-only`*
