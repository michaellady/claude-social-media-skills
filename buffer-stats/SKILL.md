---
name: buffer-stats
description: Use when user wants Buffer operational + engagement stats across channels — "buffer stats", "how is my buffer doing", "buffer analytics", "buffer queue health", "how did my buffer posts perform", "weekly buffer report", "buffer insights".
user_invocable: true
---

# buffer-stats

Combine Buffer's publishing MCP (operational data) with a gstack scrape of Buffer's Analyze dashboard (engagement data) into a one-shot weekly report. Covers queue health, posting goal status, per-channel follower/engagement metrics, and top posts by engagement — plus week-over-week deltas.

Why this exists: Buffer is the fan-out layer for IG/LinkedIn/Facebook/Threads, but engagement on those surfaces is a blind spot in `/flywheel` today. The Buffer MCP alone only covers publishing state (queue, goals, send counts); it has no analytics fields. This skill closes the gap by scraping Analyze the same way `linkedin-stats` scrapes LinkedIn Creator.

## Usage

`/buffer-stats` — full report (operational + engagement across all connected channels) and write a snapshot
`/buffer-stats --no-cache` — print report without writing a snapshot (ad-hoc mid-week checks)
`/buffer-stats --days N` — override the default 7-day window
`/buffer-stats --compare YYYY-MM-DD` — diff against a specific historical snapshot instead of the newest
`/buffer-stats operational` — skip the Analyze scrape; MCP-only fast path

## 🟢 Happy Path (read first; everything below is edge-case detail)

For a weekly Buffer stats run when nothing goes wrong. ~2-4 min wall-clock. Each step links to a labeled edge case (`Edge: <name>`) you only need to read if that step fails.

**Step 1 — Load config (5 sec).** Read `config.local.json` if present, else `config.json`. Pull `organization_name`, `default_window_days` (7), `top_posts_limit` (10), `analyze_base_url`, `insights_base_url`. Set `DAYS=7`.

**Step 2 — Operational data via Buffer MCP (15-30 sec).**
- `mcp__buffer__get_account` → pick org by `config.organization_name` (or the only one). See `Edge: multi-org-selection`.
- `mcp__buffer__list_channels` → filter out `isDisconnected: true`, apply `channels_include`/`channels_exclude`.
- Per channel: `mcp__buffer__get_channel` for posting goal, schedule, paused flag. `mcp__buffer__list_posts` twice — `status: [scheduled]` for queue depth, `status: [sent]` with `createdAt.start` = N-days-ago for sent count and tag distribution.

**Step 3 — Engagement scrape: publish.buffer.com/insights (30-60 sec).** `$B goto https://publish.buffer.com/insights`, `sleep 4`, extract Posts/Followers/Reactions/Comments summary + top-5 posts via the regex blocks in Phase 2a. Default range is "Last 7 days" — leave it alone. **Also extract the per-channel aggregate rows** (each connected channel has a Posts/Reactions/Comments row in the Insights view — all 6 channels appear, including the 3 Buffer Analyze can't reach: LinkedIn personal + both Threads accounts). See **Phase 2a-bis** below for the per-channel-row extractor; this is the source of truth for `channel_roi[]` entries that don't have impressions. See `Edge: insights-row-extraction-failed`.

**Step 4 — Engagement scrape: analyze.buffer.com per-channel (60-90 sec).** `$B goto $ANALYZE_URL` (analyze home page lists channels via `a[href*=/overview/]` links — extract channelId from each). Cookies for `buffer.com` carry to `analyze.buffer.com` (see `Edge: subdomain-cookie-gap`). For each channel: `$B goto /<service>/overview/<channelId>` → click the "Last week" preset button directly (the picker is already expanded inline — NO popover; see `Edge: analyze-picker-already-inline`) → `sleep 4` → extract `<li>` metrics where each `<li>` has text shape `"Label\nValue\nDelta%"` (see `Edge: analyze-hashed-classes`).

**Step 4b — Per-channel Channel ROI (5 sec).** Two paths now (added 2026-05-19, task #377):

- **Impressions path** (Analyze-covered: LinkedIn pages, IG business, FB pages where impressions exist). Compute `channel_roi_score = (avg_imps_per_post * eng_rate_decimal * 100) / (sent_count + 1)`, bucket 🟢 ≥100 / 🟡 10-100 / 🔴 <10 & followers<50 / ⚪ <10 & followers≥50, emit `engagement_denominator: "impressions"`. For Instagram, compute eng rate as `(likes + comments) / impressions × 100` (Buffer doesn't expose it directly).
- **Posts-only fallback path** (Insights-covered but no impressions: LinkedIn personal, Threads × 2). Compute `channel_roi_score = (reactions + 2 * comments) / posts` from the Insights row, bucket via the posts-only rubric in Phase 2c, emit `engagement_denominator: "posts"`. Comments are weighted 2× because they're a stronger intent signal than reactions and roughly 10× rarer at our volume.

See `Edge: facebook-impressions-unavailable` (no path applies — `channel_roi_score: null`, `bucket: "data_unavailable"`). See **Phase 2c** below for the full rubric, posts-only verdict variants, and worked examples.

**Step 5 — Compose snapshot JSON (5 sec).** Build the object with `engagement_tracked_channels` (any channel with engagement data via Insights OR Analyze — typically all 6) AND `posting_channels` (full set) as separate counts — never collapse them (see `Edge: engagement-vs-posting-channel-conflation`). Channels with no engagement data at all (rare — usually only happens when both surfaces fail) land in `channels_engagement_unavailable[]`. Include a `channel_roi[]` array with `{channel, sent, posts, reactions, comments, avg_imps_per_post, eng_rate_pct, channel_roi_score, engagement_denominator, bucket, verdict}` per channel — `null` for channels with missing inputs. `engagement_denominator` is `"impressions"` for the Analyze path and `"posts"` for the Insights fallback path.

**Step 5.5 — JOIN tagIds → format_tag, emit `format_engagement` (5 sec).** For every post in `top_posts[]` and every sent post enumerated in Step 2, read `tags { id name }` from the MCP payload and resolve against `_shared/buffer-post-prep/tag-ids.local.json` (inverted: tagId → `format:<name>`). Write `top_posts[].format_tag` (the **real** tag) alongside the existing `format_tag_guess` (kept as fallback). Roll up into a top-level `format_engagement` object keyed by `format:<name>` with `{posts, reactions, impressions, eng_rate_pct, channels{}}`. See **Phase 3.5** below. Edge cases: `Edge: tag-ids-missing` (no lookup file, every post resolves to `null`), `Edge: post-has-no-format-tag` (older or manual posts — fall back to `format_tag_guess` for display, exclude from aggregate).

**Step 6 — Week-over-week deltas (2 sec).** Find newest snapshot in `cache/` older than `delta_window_days` (7). Compute Δfollowers / Δengagement_rate / Δqueue_depth / Δsent_count per channel. First run renders `—` (see `Edge: delta-bootstrap`).

**Step 7 — Format-performance attribution (10 sec).** For each sent post in window, read its `format:<name>` tag via `mcp__buffer__get_post`. Aggregate by `(channel, format)` → avg impressions, avg engagement, eng rate. Surface a verdict per channel.

**Step 8 — Skill recommendations (5 sec).** From the format-performance table, generate suggestions for promote-*, carousel-newsletter, etc. with data citations. Suggestions only — user accepts/rejects manually.

**Step 9 — Render + write (5 sec).** Print the fixed-structure markdown report (Channels table → Format performance → Skill recommendations → Top posts → Flags). Write `cache/snapshot-YYYY-MM-DD.json` and `.md` unless `--no-cache`.

### Edge labels (jump to these only when you hit the matching failure signal)

| Label | Symptom |
|---|---|
| `Edge: multi-org-selection` | Account has >1 Buffer org and no `organization_name` saved |
| `Edge: subdomain-cookie-gap` | `analyze.buffer.com` redirects to login despite `buffer.com` cookies |
| `Edge: no-last-7-days-preset` | Buffer Analyze date picker has no "Last 7 days" option |
| `Edge: analyze-picker-already-inline` | Date picker shows This/Last month + This/Last week buttons inline — no popover; click target directly |
| `Edge: analyze-hashed-classes` | Class selectors return nothing; text-pattern extractors needed |
| `Edge: instagram-not-linked` | IG channel shows "Unlock Instagram Analytics" banner, no engagement data |
| `Edge: instagram-no-eng-rate-field` | IG doesn't report "Engagement Rate" — compute from `(likes + comments) / impressions × 100` |
| `Edge: facebook-impressions-unavailable` | Facebook Pages Impressions field missing entirely |
| `Edge: engagement-data-lag` | New channel / freshly published post engagement reads `null` 24-48h |
| `Edge: custom-date-range-unimplemented` | `--days` value other than 7 or ~30 |
| `Edge: delta-bootstrap` | First run — no prior snapshot to diff against |
| `Edge: engagement-vs-posting-channel-conflation` | total_followers reads tiny because LinkedIn-personal/Threads omitted |
| `Edge: tag-ids-missing` | `_shared/buffer-post-prep/tag-ids.local.json` not present — `format_tag` resolves to `null` for every post |
| `Edge: post-has-no-format-tag` | Pre-convention or manual post — no `format:*` tag, falls back to `format_tag_guess` for display only |
| `Edge: insights-row-extraction-failed` | Per-channel rows in `publish.buffer.com/insights` don't parse — selectors drifted, all 3 non-Analyze channels read null |

Each label corresponds to an entry in **Known issues / robustness notes** below.

## Config

The skill reads config from (in priority order):
1. `~/dev/claude-social-media-skills/buffer-stats/config.local.json` (gitignored — personal org/channel overrides)
2. `~/dev/claude-social-media-skills/buffer-stats/config.json` (committed defaults)

Fields:
- `organization_name` — if your account has >1 Buffer organization, set this to auto-pick. `null` = prompt on first run, then save choice.
- `channels_include` — array of channel names to report on. `null` = all connected channels.
- `channels_exclude` — array of channel names to skip (e.g. `["X"]` if you've stopped posting there).
- `default_window_days` — default reporting window (7).
- `top_posts_limit` — how many top posts to show (10).
- `delta_window_days` — compare against the newest snapshot at least this old (7).
- `stale_snapshot_days` — flag snapshots older than this when `/flywheel` consumes them (14).
- `analyze_base_url` / `publish_base_url` — Buffer surface URLs (unlikely to change).
- `insights_base_url` — `https://publish.buffer.com/insights` (the Beta cross-channel summary view; richer than analyze.buffer.com for most "how have my posts done" questions).

Load config at the start of every run:

```bash
CONFIG_DIR=~/dev/claude-social-media-skills/buffer-stats
if [ -f "$CONFIG_DIR/config.local.json" ]; then CONFIG_FILE="$CONFIG_DIR/config.local.json"; else CONFIG_FILE="$CONFIG_DIR/config.json"; fi
DAYS=$(jq -r .default_window_days "$CONFIG_FILE")
TOP_N=$(jq -r .top_posts_limit "$CONFIG_FILE")
ORG_NAME=$(jq -r .organization_name "$CONFIG_FILE")
```

## Process

### Phase 1 — Operational data (via Buffer MCP)

The Buffer MCP is always available in this project — no browser needed, no cookies required.

1. **Resolve organization**
   - Call `mcp__buffer__get_account` → read `organizations[]`.
   - If `config.organization_name` is set, find by name. Else if there's exactly one org, use it. Else prompt the user to pick and offer to save the choice to `config.local.json`.

2. **List channels**
   - Call `mcp__buffer__list_channels` with `organizationId`.
   - Filter out `isDisconnected: true`.
   - Apply `channels_include` / `channels_exclude` filters from config.

3. **Per-channel operational metrics**
   - For each channel, call `mcp__buffer__get_channel` to grab:
     - `postingGoal` — `{ goal, sentCount, scheduledCount, status, periodStart, periodEnd }` (may be `null` if no goal set)
     - `postingSchedule` — weekly cadence array
     - `isQueuePaused`
     - `service` + `type` (e.g. `linkedin/profile`, `instagram/business`)
   - For each channel, call `mcp__buffer__list_posts` with:
     - `filter.channelIds: [channelId]`, `filter.status: [scheduled]`, `first: 100` → queue depth
     - `filter.channelIds: [channelId]`, `filter.status: [sent]`, `filter.createdAt: { start: <N days ago> }`, `first: 100` → sent count in window, plus tag distribution

4. **Organization-level flags**
   - Paused queues, posting goals `AtRisk`, channels with empty queues, channels over weekly limit.

### Phase 2 — Engagement data

Skip this phase entirely when invoked as `/buffer-stats operational`.

**Buffer exposes engagement data on TWO surfaces.** The skill scrapes both for complementary views (confirmed 2026-04-27):

1. **`https://publish.buffer.com/insights`** (Beta) — **cross-channel aggregated.** Covers ALL connected channels (LinkedIn personal, both Threads accounts, etc.) that analyze.buffer.com doesn't surface. Top posts ranked by reactions/comments. Best single view for "how have my posts done overall."
2. **`https://analyze.buffer.com`** — per-channel deep dive. Covers FB pages, IG business, LinkedIn pages, Twitter (NOT LinkedIn personal, NOT Threads). Top posts ranked by impressions. Best for per-channel engagement-rate breakdown.

Run Insights first (faster, better cross-channel summary), then Analyze for the channels Insights doesn't fully break out per-platform engagement-rate for.

#### Phase 2a — publish.buffer.com/insights (cross-channel summary)

```bash
B=~/.claude/skills/gstack/browse/dist/browse
$B goto https://publish.buffer.com/insights
sleep 4
$B js "
const main = document.querySelector('main, [role=main]') || document.body;
const text = main.innerText;
// Top section: Posts / Followers / Reactions / Comments with deltas
const summary = {};
const summaryRegex = /(Posts|Followers|Reactions|Comments)\\s+(\\S+)\\s+(Up|Down)\\s+([\\d.]+%|\\+\\d+)/g;
let m;
while ((m = summaryRegex.exec(text))) {
  summary[m[1]] = { value: m[2], direction: m[3].toLowerCase(), delta: m[4] };
}
({ summary, mainTextSnippet: text.slice(0, 600) })
"
```

Date presets at publish.buffer.com/insights: **Last 7 days, Last 30 days, Month to date, Last month, Custom**. Default is "Last 7 days" — change via the picker button (text matches the current selection, e.g. "Last 7 days" → click → option list appears).

Top-5-posts extraction from Insights:
```bash
$B js "
const text = (document.querySelector('main, [role=main]') || document.body).innerText;
// Posts appear as '#N Post' / 'X Reactions' / time / text
const postBlockRegex = /#(\\d+) Post\\s+(\\d+) Reactions\\s+(\\d{1,2}:\\d{2}\\s*[AP]M)\\s+([\\s\\S]+?)(?=View comments|Duplicate post|#\\d+ Post|$)/g;
const posts = [];
let m;
while ((m = postBlockRegex.exec(text))) {
  posts.push({ rank: parseInt(m[1]), reactions: parseInt(m[2]), time: m[3], snippet: m[4].trim().slice(0, 200) });
}
posts
"
```

The Insights surface is in **Beta** (banner: "Looking for more metrics and reports? Go to Analyze"). Treat it as the primary cross-channel view but expect occasional UI changes.

##### Phase 2a-bis — Per-channel aggregate rows from Insights (added 2026-05-19, task #377)

`publish.buffer.com/insights` renders a per-channel breakdown below the summary tiles — **one row per connected channel, all 6 visible**, regardless of whether Buffer Analyze covers that surface. This is the only place we can read aggregate engagement for LinkedIn personal + both Threads accounts (Analyze doesn't expose them per-channel).

Each row's text shape is `"<Channel display name>\n<service>\n<N> Posts\n<R> Reactions\n<C> Comments"` (no impressions field — Insights aggregates reactions/comments only, which is exactly why these channels fall back to the posts-only ROI path).

```bash
$B js "
const main = document.querySelector('main, [role=main]') || document.body;
const text = main.innerText;
// Per-channel rows: name on one line, then Posts/Reactions/Comments triplets nearby.
// Capture is greedy on the name then anchors on the integer-Posts/Reactions/Comments triple.
const rowRegex = /([^\\n]+?)\\n(linkedin|instagram|facebook|threads|twitter)\\s*\\n(\\d+)\\s+Posts?\\s+(\\d+)\\s+Reactions?\\s+(\\d+)\\s+Comments?/gi;
const rows = [];
let m;
while ((m = rowRegex.exec(text))) {
  rows.push({
    display_name: m[1].trim(),
    service: m[2].toLowerCase(),
    posts: parseInt(m[3]),
    reactions: parseInt(m[4]),
    comments: parseInt(m[5])
  });
}
rows
" > /tmp/bf-insights-rows.json
```

Cross-reference each row against `mcp__buffer__list_channels` output by display name to attach the `channelId` + `type` (profile vs page vs business). If the row regex returns fewer rows than `posting_channels`, that's `Edge: insights-row-extraction-failed` — re-run discovery (see "Re-discovery" section).

The default range at `publish.buffer.com/insights` is "Last 7 days" but the per-channel rows respect whatever the picker is set to. For the weekly snapshot, leave it at "Last 7 days". For a 30-day reconciliation (when comparing against the user's manual eyeball of Insights), click "Last 30 days" first.

#### Phase 2b — analyze.buffer.com (per-channel deep dive)

**Buffer Analyze lives at `https://analyze.buffer.com`** (separate subdomain from `publish.buffer.com`). The dashboard is client-rendered React; selectors below are confirmed as of 2026-04-20.

#### Step 2a — Initialize browser and verify login

```bash
B=~/.claude/skills/gstack/browse/dist/browse
if [ ! -x "$B" ]; then echo "gstack browse not installed"; exit 1; fi

ANALYZE_URL=$(jq -r .analyze_base_url "$CONFIG_FILE")
$B goto "$ANALYZE_URL"
sleep 3
$B js "(() => ({ url: location.href, needLogin: !!document.querySelector('input[type=email], input[type=password]') }))();" > /tmp/bf-login-check.txt
if grep -q '"needLogin":true' /tmp/bf-login-check.txt; then
  echo "Not logged in — attempting cookie import..."
  $B cookie-import-browser chrome buffer.com
  # Cookie picker UI opens; user selects buffer.com, closes picker
  $B goto "$ANALYZE_URL"
  sleep 3
  $B js "(() => !!document.querySelector('input[type=password]'))();" > /tmp/bf-login-check.txt
  if grep -q 'true' /tmp/bf-login-check.txt; then
    # KNOWN ISSUE: cookies for `buffer.com` do not cover the `analyze.` subdomain.
    # A fresh login is required on first run.
    $B handoff "Please log in to Buffer Analyze at $ANALYZE_URL — reply 'done' once you see the dashboard."
    # user runs $B resume
  fi
fi
```

#### Step 2b — Discover channel URLs from the home page

Each channel has a dedicated URL. Extract them from the home-page links:

```bash
$B goto "$ANALYZE_URL"
sleep 2
$B js "
  (() => {
    const links = [...document.querySelectorAll('a[href]')].filter(a => /analyze\\.buffer\\.com\\/(facebook|instagram|twitter|linkedin|threads)\\/overview\\//.test(a.href));
    return links.map(a => {
      const m = a.href.match(/\\/(facebook|instagram|twitter|linkedin|threads)\\/overview\\/([a-f0-9]+)/);
      return { service: m[1], channelId: m[2], name: a.innerText.trim(), url: a.href };
    });
  })();
" > /tmp/bf-channels.json
```

Cross-reference with the MCP's `list_channels` output (matches on `serviceId` → Buffer's channel ID).

#### Step 2c — Set the date range

Buffer Analyze has preset ranges only: **This month, Last month, This week, Last week, Custom**. There's no "Last 7 days" preset — **use "Last week"** as the closest match for a 7-day window (it's Monday-to-Sunday of the previous week, a complete 7 days of data).

**Why not "This week":** it starts at Monday-to-now; if the skill runs on a Monday, all metrics read 0.

```bash
# Open the date picker and pick "Last week"
$B js "
  (() => {
    const btn = [...document.querySelectorAll('button')].find(b => /This month|Last month|This week|Last week|Custom|[A-Z][a-z]+ \\d+(\\s*-\\s*\\d+)?,?\\s*\\d{4}/.test(b.innerText));
    btn?.click();
    return btn ? 'opened' : 'no_btn';
  })();
"
sleep 1
$B js "
  (() => {
    const opt = [...document.querySelectorAll('button')].find(b => b.innerText?.trim() === 'Last week');
    opt?.click();
    return opt ? 'selected' : 'no_opt';
  })();
"
sleep 3
```

For `--days 30`, use "Last month" (full calendar previous month). For arbitrary windows, use "Custom" — see Known Issues section.

#### Step 2d — Extract channel Overview metrics

Each metric on a channel's Overview page is a `<li>` whose `innerText` matches `"<Label>\n<Value>\n<Delta>%"` (delta is optional). The class names are hashed (styled-components), so extract by text pattern rather than class selector:

```bash
# For each channel URL discovered in Step 2b:
$B goto "$CHANNEL_URL"
sleep 3
# Re-set date range if needed (the picker persists across navigations)
$B js "
  (() => {
    const lis = [...document.querySelectorAll('li')];
    const metrics = {};
    for (const li of lis) {
      const lines = (li.innerText||'').split('\\n').map(s => s.trim()).filter(Boolean);
      if (lines.length < 2 || lines.length > 3) continue;
      const [label, value, delta] = lines;
      if (!/^[A-Za-z][A-Za-z ]+\$/.test(label)) continue;
      if (!/^[\\d.,]+%?\$/.test(value)) continue;
      metrics[label] = { value, delta: delta || null };
    }
    return metrics;
  })();
"
```

Expected labels per service (confirmed for LinkedIn, assumed similar for others):
- **Performance section:** `Followers`, `New Followers`, `Posts`, `Impressions`, `Clicks`, `Engagement Rate`
- **Average performance section:** `Average Impressions Per Post`, `Average Clicks Per Post`, `Average Engagement Rate Per Post`

**Service-specific gaps:**
- Facebook Pages: `Impressions` shows a "not available" banner on some plans — parse as `null`.
- Instagram: requires linking to a Facebook Business Page (banner: "Unlock Instagram Analytics"). If the banner is present, skip engagement extraction and flag in output.

#### Step 2e — Extract per-post metrics from the Posts tab

Navigate to `https://analyze.buffer.com/<service>/posts/<channelId>`. Posts are in an `<ol>` containing `<li>` elements with the semantic class token `post-item` (safer than the hashed prefix):

```bash
$B goto "https://analyze.buffer.com/$SERVICE/posts/$CHANNEL_ID"
sleep 3
$B js "
  (() => {
    const posts = [...document.querySelectorAll('li.post-item, li[class*=post-item]')];
    return posts.map(post => {
      const lines = (post.innerText||'').split('\\n').map(s => s.trim()).filter(Boolean);
      const dateIdx = lines.findIndex(l => /^[A-Z][a-z]+ \\d+, \\d{4}/.test(l));
      const metricsStart = lines.findIndex(l => l === 'Impressions');
      const date = dateIdx >= 0 ? lines[dateIdx] : null;
      const username = dateIdx >= 0 ? lines[dateIdx + 1] : null;
      const postText = dateIdx >= 0 && metricsStart > dateIdx ? lines.slice(dateIdx + 2, metricsStart).join(' ') : null;
      const metrics = {};
      for (let i = metricsStart; i < lines.length - 1; i += 2) {
        if (/^[A-Za-z][A-Za-z. ]+\$/.test(lines[i]) && /^[\\d.,]+%?\$/.test(lines[i+1])) {
          metrics[lines[i]] = lines[i+1];
        }
      }
      return { date, username, postText: postText?.slice(0, 300), metrics };
    });
  })();
" > /tmp/bf-posts-$SERVICE.json
```

Per-post metrics observed for LinkedIn: `Impressions`, `Likes`, `Comments`, `Clicks`, `Eng. Rate`. Facebook and Threads likely use `Reactions` or `Shares` in place of `Likes` — the extractor is label-agnostic and will preserve whatever Buffer shows.

#### Step 2f — Cross-channel aggregation

Combine Step 2d (per-channel overview) + Step 2e (per-channel posts). Compute:
- `channels[].engagement` = overview metrics (followers, engagement rate, impressions)
- `top_posts[]` = flatten per-service posts, sort by `Impressions` desc, take top N from `config.top_posts_limit`

Normalize metric names across services (e.g. LinkedIn's "Eng. Rate" and Facebook's potentially different label — keep originals in raw JSON but expose a unified `engagement_rate` field).

#### Phase 2c — Channel ROI score (run after Phase 2a-bis + Phase 2b for every channel with engagement data)

For each Buffer-connected channel, compute a `channel_roi_score` and bucket it. This surfaces deprioritization candidates so the user can stop fanning out content to channels that don't earn back the queue cost. **Two paths** depending on whether impressions data exists:

##### Path A: impressions available (LinkedIn pages, IG business, FB pages with impressions)

```
channel_roi_score = (avg_imps_per_post * eng_rate_decimal * 100) / (sent_count_in_window + 1)
engagement_denominator = "impressions"
```

Where `eng_rate_decimal` is the engagement rate expressed as a decimal (e.g. 12.42% → `0.1242`). The `* 100` keeps the score in a readable 0-1000 range; the `+ 1` in the denominator is a smoothing term so a channel with `sent=0` doesn't divide-by-zero.

**Bucketing rubric (Path A):**

| Score | Followers | Bucket | Action |
|---|---|---|---|
| ≥ 100 | any | 🟢 **High ROI** | Keep current cadence, consider increasing |
| 10 - 100 | any | 🟡 **Mid ROI** | Current cadence is fine |
| < 10 | < 50 | 🔴 **Below threshold** | Drop from fan-out (recommend `min_followers_to_promote = 100`) |
| < 10 | ≥ 50 | ⚪ **Diminishing returns** | Reduce fan-out volume; consider `tease-newsletter` over `promote-newsletter` |

##### Path B: posts-only fallback (LinkedIn personal, Threads × 2 — Insights rows from Phase 2a-bis)

When Buffer doesn't surface impressions for a channel (true for LinkedIn personal and both Threads accounts on Insights), score on the reaction+comment volume per post instead. Comments weighted 2× because they're rarer and a stronger intent signal at our volume (~10× rarer than reactions on the channels we measure).

```
channel_roi_score = (reactions + 2 * comments) / posts
engagement_denominator = "posts"
```

For `posts = 0`, emit `channel_roi_score: null` and bucket as `data_unavailable` — we have no signal at all.

**Bucketing rubric (Path B):**

| Score (engagement per post) | Bucket | Action |
|---|---|---|
| ≥ 1.0 | 🟢 **High eng/post** | Keep current cadence — every post is earning a reaction or comment |
| 0.3 - 1.0 | 🟡 **Mid eng/post** | Current cadence fine; watch for downward drift |
| 0.05 - 0.3 | ⚪ **Low eng/post** | Reduce fan-out frequency; consider routing through a different format |
| 0 - 0.05 | 🔴 **Dead channel candidate** | Investigate (was the channel ever live? auth broken? algorithm shadow-band?). Pair with `audit-buffer-queue`. |
| exactly 0 over ≥10 posts | 🔴 **Dead channel** | Pause queue; open investigation task (see #379 for the enterprisevibecode Threads case) |

**Verdict variants (Path B):**

| Bucket | Verdict template |
|---|---|
| 🟢 High eng/post | `"High engagement per post (X.X reactions+2comments/post across N posts) — keep cadence"` |
| 🟡 Mid eng/post | `"Mid engagement per post (X.X/post) — current cadence is fine"` |
| ⚪ Low eng/post | `"Low engagement per post (X.X/post across N posts) — consider reducing fan-out"` |
| 🔴 Dead candidate | `"Near-dead channel: X total reactions across N posts (X.XX/post) — investigate"` |
| 🔴 Dead channel | `"Dead channel: 0 reactions across N posts — pause and investigate (task #379-style)"` |
| ⚪ data_unavailable | `"No posts in window — no signal to compute ROI"` |

**Worked examples** (confirmed against real data — impressions path 2026-05-18, posts-only path 2026-05-19 from Apr 19-May 19 Insights view):

Path A (impressions):

| Channel | Sent | Avg imps/post | Eng rate | Score | Bucket |
|---|---:|---:|---:|---:|:---|
| LinkedIn page (EVC) | 5 | 13 | 12.42% | `(13 * 0.1242 * 100) / 6 = 26.9` | 🟡 Mid (saved from 🔴 by strong eng rate on tiny audience) |
| Instagram (EVC) | 6 | 157.2 | 1.06%* | `(157.2 * 0.0106 * 100) / 7 = 23.8` | 🟡 Mid |
| Facebook (EVC) | 1 | n/a | n/a | n/a | ⚪ Data unavailable (see `Edge: facebook-impressions-unavailable`) |

\* IG eng rate computed via the fallback below (Buffer doesn't expose a single "Engagement Rate" field for IG).

Path B (posts-only, from Insights 30-day window):

| Channel | Posts | Reactions | Comments | Score `(R + 2C) / P` | Bucket |
|---|---:|---:|---:|---:|:---|
| LinkedIn personal (mikelady) | 60 | 47 | 22 | `(47 + 44) / 60 = 1.52` | 🟢 High eng/post — engagement king |
| Threads (mikelady) | 53 | 11 | 2 | `(11 + 4) / 53 = 0.28` | ⚪ Low eng/post |
| Threads (enterprisevibecode) | 64 | 0 | 0 | `0 / 64 = 0` | 🔴 Dead channel (task #379) |

Cross-method sanity check: LinkedIn personal scoring 1.52 on Path B vs LinkedIn page scoring 26.9 on Path A is **expected** — the two scales are not comparable. Path A is `imps * eng_rate / sent` (impressions × rate); Path B is raw engagement per post. The buckets normalize that gap; the raw scores do not.

**Instagram engagement-rate fallback** (`Edge: instagram-no-eng-rate-field`):

Buffer Analyze's IG overview returns `Posts / Impressions / Reach / Likes / Comments / Daily average impressions / Average likes per post / Average comments per post` — no aggregated "Engagement Rate" line. Compute it:

```
eng_rate_pct = ((likes + comments) / impressions) * 100
```

Use impressions (not reach) as the denominator — keeps the rate comparable to LinkedIn's `engagement_rate` field which is `(reactions + comments + reposts) / impressions`.

**Facebook impressions gap** (`Edge: facebook-impressions-unavailable`): Buffer shows reactions and new fans but the Impressions field is missing entirely. Without impressions you cannot compute either `avg_imps_per_post` or `eng_rate`. Set `channel_roi_score: null`, `bucket: "data_unavailable"`, and note in the verdict.

**Output shape** to include in the snapshot under `channel_roi[]`. Every record carries `engagement_denominator` so downstream consumers know which formula produced the score and which scale to compare against:

```json
"channel_roi": [
  {
    "channel": "linkedin/page (EVC)",
    "followers": 28,
    "sent": 5,
    "avg_imps_per_post": 13,
    "eng_rate_pct": 12.42,
    "channel_roi_score": 26.9,
    "engagement_denominator": "impressions",
    "bucket": "yellow_mid",
    "verdict": "Mid ROI — current cadence fine, but absolute reach is tiny (13 imps/post)"
  },
  {
    "channel": "linkedin/profile (mikelady)",
    "posts": 60,
    "reactions": 47,
    "comments": 22,
    "channel_roi_score": 1.52,
    "engagement_denominator": "posts",
    "engagement_source": "publish.buffer.com/insights per-channel row (30d window)",
    "bucket": "green_high_eng_per_post",
    "verdict": "High engagement per post (1.52 reactions+2comments/post across 60 posts) — keep cadence"
  },
  {
    "channel": "threads/profile (mikelady)",
    "posts": 53,
    "reactions": 11,
    "comments": 2,
    "channel_roi_score": 0.28,
    "engagement_denominator": "posts",
    "engagement_source": "publish.buffer.com/insights per-channel row (30d window)",
    "bucket": "white_low_eng_per_post",
    "verdict": "Low engagement per post (0.28/post across 53 posts) — consider reducing fan-out"
  },
  {
    "channel": "threads/profile (enterprisevibecode)",
    "posts": 64,
    "reactions": 0,
    "comments": 0,
    "channel_roi_score": 0,
    "engagement_denominator": "posts",
    "engagement_source": "publish.buffer.com/insights per-channel row (30d window)",
    "bucket": "red_dead_channel",
    "verdict": "Dead channel: 0 reactions across 64 posts — pause and investigate (task #379)"
  },
  {
    "channel": "facebook/page (EVC)",
    "sent": 1,
    "channel_roi_score": null,
    "engagement_denominator": null,
    "bucket": "data_unavailable",
    "verdict": "FB Impressions field unavailable; manual eyeball only"
  }
]
```

### Phase 3 — Compose snapshot

Combine operational + engagement data into a single JSON object. **CRITICAL schema rule (added 2026-05-03 after the flywheel reported total_followers=26 when the actual was ~2,200):** distinguish "channels we have engagement data for" (the engagement-tracked subset — typically all 6 since Insights covers everything Analyze doesn't) from "channels we post to" (the full posting set). Conflating the two gives downstream consumers like `/flywheel` a false total.

**Updated 2026-05-19 (task #377):** the `channels_engagement_unavailable[]` list is for channels with **zero** engagement coverage across BOTH Insights and Analyze. The previous framing — "Buffer does not cover Threads / LinkedIn personal" — was wrong. The correct framing is: Buffer Analyze does not cover those per-post, but Buffer Insights covers their aggregate (posts + reactions + comments per channel, 7d/30d window). Channels that only have Insights aggregate (no per-post breakdown) should still appear in `channel_roi[]` via the posts-only fallback path; they should NOT appear in `channels_engagement_unavailable[]`.

```json
{
  "fetched_at": "2026-04-20T17:00:00Z",
  "window_days": 7,
  "organization": { "id": "...", "name": "..." },
  "engagement_tracked_channels": 6,    // count of channels with engagement data via Insights aggregate OR Analyze per-post (typically all 6 after task #377)
  "posting_channels": 6,                // count of channels we actually post to (full set)
  "channels_engagement_unavailable": [], // only channels with NO engagement data on either surface (rare — empty in steady state)
  "channels_impressions_unavailable": [ // channels using the posts-only ROI fallback (Insights aggregate, no impressions)
    "linkedin/profile (mikelady) — Insights aggregate only; Analyze does not cover LinkedIn personal per-post",
    "threads/profile (mikelady) — Insights aggregate only; Analyze does not cover Threads per-post",
    "threads/profile (enterprisevibecode) — Insights aggregate only; Analyze does not cover Threads per-post"
  ],
  "channels": [
    {
      "id": "...",
      "service": "linkedin",
      "type": "profile",
      "name": "Mike Lady",
      "operational": {
        "queued": 5,
        "sent_in_window": 3,
        "is_paused": false,
        "posting_goal": { "goal": 4, "sent": 3, "scheduled": 5, "status": "OnTrack" },
        "top_tags": [{"name": "newsletter", "count": 2}]
      },
      "engagement": {
        "followers": 1234,
        "followers_delta": 7,
        "engagement_rate": 0.042,
        "total_impressions": 18500,
        "total_reach": 12300
      }
    }
  ],
  "top_posts": [
    {
      "id": "...",
      "channel_id": "...",
      "service": "linkedin",
      "text_snippet": "Kai Greene was never Mr. Olympia...",
      "sent_at": "2026-04-19T16:00:00Z",
      "impressions": 4210,
      "likes": 112,
      "comments": 18,
      "shares": 9,
      "clicks": 64,
      "engagement": 139,
      "engagement_rate": 0.033
    }
  ],
  "flags": {
    "disconnected_channels": [],
    "paused_queues": [],
    "goals_at_risk": [],
    "empty_queues": [],
    "stale_engagement": []
  }
}
```

Write both outputs:
- `cache/snapshot-YYYY-MM-DD.json` — flywheel-ingest shape (stable fields for diffability)
- `cache/snapshot-YYYY-MM-DD.md` — human-readable report (regenerated from the JSON)

### Phase 3.5 — Resolve real `format_tag` from Buffer tagIds (JOIN)

This is the **real** closed-loop key. Phase 2's engagement scrape produces `top_posts[]` with text snippets but no tag attribution; Phase 1's MCP calls return each post's `tagIds` array but no impressions/reactions. This phase JOINs the two so downstream consumers can answer "which formats actually work" instead of guessing from snippet text.

**Why this exists:** snapshots prior to 2026-05-19 carry a `format_tag_guess` field that was inferred by eyeballing each post's snippet (`"this looks like a verbatim_quote because…"`). That guess is unreliable and not machine-comparable across weeks. The promote-* and carousel-newsletter skills already set `tagIds` at compose time via `_shared/buffer-post-prep` — this phase reads them back.

**GraphQL field shape** (confirmed against `_shared/buffer-post-prep/README.md` setup query): each `Post` node exposes `tags { id name }` where `id` is the 24-char hex Tag ID and `name` is the human label (e.g. `"format:verbatim-quote"`). The MCP's `mcp__buffer__get_post(id)` returns the same `tags` field; `list_posts` also includes it on each edge.

**Lookup file:** `_shared/buffer-post-prep/tag-ids.local.json` (gitignored, per-organization). Shape is `{ "<format_key>": "<24-hex-tagId>" }` — keys match `_shared/format_tags.json` keys (e.g. `verbatim_quote`, `teaser`, `carousel`, `link_share`, `batch_summary`). Invert it on load to get `tagId → format_key`.

#### Step 3.5a — Load the tagId → format_key lookup

```bash
REPO_ROOT=$(git -C ~/dev/claude-social-media-skills rev-parse --show-toplevel)
TAG_IDS_FILE="$REPO_ROOT/_shared/buffer-post-prep/tag-ids.local.json"
if [ ! -f "$TAG_IDS_FILE" ]; then
  echo "buffer-stats: WARN $TAG_IDS_FILE missing — falling back to format_tag_guess for all posts (see Edge: tag-ids-missing)"
  TAG_LOOKUP="{}"
else
  TAG_LOOKUP=$(jq 'with_entries(.value as $id | .value = .key | .key = $id) | with_entries(.value = "format:" + (.value | gsub("_"; "-")))' "$TAG_IDS_FILE")
fi
```

The `jq` invert flips `{ "verbatim_quote": "69ef..." }` into `{ "69ef...": "format:verbatim-quote" }` so per-post resolution is an O(1) hash lookup.

#### Step 3.5b — Resolve `format_tag` for every post in `top_posts[]` and for every sent post in the window

For each post discovered in Phase 2 (`top_posts[]`) AND every sent post enumerated in Phase 1 step 3 (the `list_posts` calls with `status: [sent]`):

1. Pull `tagIds` from the MCP payload — `list_posts` already returns `tags { id name }` on each edge, so usually no extra round-trip is needed. If the engagement scrape produced a post that wasn't in the MCP set (rare — only happens when the post is outside the MCP `createdAt` window), call `mcp__buffer__get_post(id)` for that single post.
2. For each tagId on the post, look it up in `TAG_LOOKUP`. Stop at the first match (a post should only have one `format:*` tag, but other tags like `via:network` may coexist — ignore those).
3. Write the resolved name to `format_tag`. If no `format:*` tag is present on the post, fall back to `format_tag_guess` (existing snippet-based inference). Always emit both fields when both are available; downstream consumers prefer `format_tag` over `format_tag_guess`.

Example resolution:

```js
const formatTag = (post.tags || [])
  .map(t => TAG_LOOKUP[t.id])
  .find(name => name && name.startsWith("format:"));
post.format_tag = formatTag || null;
// keep post.format_tag_guess for fallback display only
```

#### Step 3.5c — Emit `format_engagement` aggregate

Roll the per-post `format_tag` up into a top-level `format_engagement` object in the snapshot. Posts whose `format_tag` is `null` (no real tag, only a guess) are **excluded** from this aggregate — the goal is high-confidence attribution, and `format_tag_guess` belongs in a separate exploratory table if anywhere.

Schema:

```json
"format_engagement": {
  "format:verbatim-quote": {
    "posts": 7,
    "reactions": 14,
    "impressions": 312,
    "eng_rate_pct": 4.49,
    "channels": {
      "linkedin/page": { "posts": 3, "reactions": 6, "impressions": 39 },
      "linkedin/profile": { "posts": 4, "reactions": 8, "impressions": 273 }
    }
  },
  "format:teaser": { "posts": 2, "reactions": 1, "impressions": 188, "eng_rate_pct": 0.53, "channels": { "...": { "..." : 0 } } },
  "format:carousel": { "posts": 1, "reactions": 39, "impressions": 4293, "eng_rate_pct": 0.91, "channels": { "instagram/business": { "posts": 1, "reactions": 39, "impressions": 4293 } } }
}
```

Per-format `eng_rate_pct` = `(reactions / impressions) * 100`, rounded to two decimals. When `impressions` is `null` for any contributing post (e.g. Facebook — see `Edge: facebook-impressions-unavailable`), drop that post from the impressions/eng_rate roll-up but keep it in `posts` and `reactions` so the count still reflects reality; note the partial coverage in the snapshot's `flags.format_engagement_partial[]` array.

Channel keys use the same `service/type` shape as `channels[].service` + `channels[].type` (e.g. `linkedin/page`, `instagram/business`) for cross-referencing. Do **not** collapse across channels — Phase 5's recommendation engine needs the per-`(format, channel)` cell because a format that works on LinkedIn personal may flop on a LinkedIn page.

#### Edge: tag-ids-missing

If `_shared/buffer-post-prep/tag-ids.local.json` doesn't exist, every post's `format_tag` resolves to `null` and `format_engagement` is `{}`. The skill must still produce a valid snapshot — the engagement scrape and operational data are independent of this JOIN. Surface as `flags.tag_ids_missing: true` and print a one-line notice in the markdown report: "Closed-loop attribution disabled — no `_shared/buffer-post-prep/tag-ids.local.json`. Run setup per `_shared/buffer-post-prep/README.md` to enable per-format engagement attribution."

#### Edge: post-has-no-format-tag

Posts created before the format-tagging convention shipped (anything before 2026-04-27 for most channels) and posts created outside the promote-*/carousel-newsletter skills (e.g. manual Buffer web-UI posts, `via:network` auto-cross-posts) carry no `format:*` tag. `format_tag` stays `null`; `format_tag_guess` may still be populated by the snippet-inference path for display purposes only. The `format_engagement` aggregate excludes these posts — counting them as "untagged" rather than guessing inflates whichever format the inference happens to lean toward.

Track untagged volume in `flags.untagged_posts`:

```json
"flags": {
  "untagged_posts": { "count": 4, "channels": ["linkedin/profile", "threads/profile"] }
}
```

This is the same signal `audit-buffer-queue` surfaces but computed from sent-post data here.

### Phase 4 — Week-over-week deltas

```bash
CACHE_DIR=~/dev/claude-social-media-skills/buffer-stats/cache
mkdir -p "$CACHE_DIR"

# Find the newest snapshot older than `delta_window_days` days
TARGET_AGE=$(jq -r .delta_window_days "$CONFIG_FILE")
CUTOFF=$(date -v-${TARGET_AGE}d -u +%Y-%m-%d 2>/dev/null || date -d "$TARGET_AGE days ago" -u +%Y-%m-%d)
PRIOR_SNAP=$(ls -1 "$CACHE_DIR"/snapshot-*.json 2>/dev/null | awk -v c="snapshot-$CUTOFF" '$0 <= c' | tail -1)
```

For each channel in the current snapshot that also exists in `$PRIOR_SNAP`:
- Δfollowers
- Δengagement_rate
- Δqueue_depth
- Δsent_count

Bootstrap case: first run has no prior snapshot → deltas render as `—`.

### Phase 5 — Format-performance analysis (closed-loop input)

This is the **measurement** half of the closed loop. The promote-* skills tag every post they create with a `format:<name>` tag (`format:verbatim-quote`, `format:teaser`, `format:carousel`, `format:link-share`, `format:long-form-pulse`, `format:batch-summary`). Group sent posts by tag, compute per-format engagement, and surface which formats are working on which channels.

**Phase 3.5 already did the heavy lifting** — it resolved every post's `format_tag` from its `tagIds` and produced the top-level `format_engagement` aggregate. Phase 5 consumes that aggregate to produce the rendered table and the verdicts; it should **not** re-walk the post list or re-call `get_post`. If `format_engagement` is `{}` (the lookup file was missing), Phase 5 renders "Closed-loop attribution disabled — see `flags.tag_ids_missing`" and skips the table.

For each sent post in the window, the per-post `format_tag` set in Phase 3.5 already feeds the aggregate. For each `format:<name>` key in `format_engagement`, surface:
- count of posts
- sum of impressions
- sum of engagements (reactions + comments + reposts)
- average impressions per post
- average engagement rate per post

Group by `(channel, format)` so the output answers "do verbatim quotes work on LinkedIn?" not just "what's the average engagement on LinkedIn?"

Produce a table:

| Channel | Format | Posts | Avg imps | Avg eng | Eng rate |
|---|---|---:|---:|---:|---:|
| LinkedIn personal | verbatim-quote | 9 | 23 | 0.4 | 1.7% |
| LinkedIn personal | teaser | 5 | 188 | 1.2 | 0.6% |
| LinkedIn personal | long-form-pulse | 1 | 412 | 8 | 1.9% |
| Instagram | carousel | 1 | 4,293 | 39 | 0.91% |
| ... | | | | | |

**Surface a verdict** under each `(channel, top-2-formats)` pairing — "long-form-pulse outperforms verbatim-quote 18× on impressions on LinkedIn personal." This is the seed data for Phase 5b's adapt step.

### Phase 5b — Recommend skill changes (closed-loop output)

Based on the format-performance table, generate a **recommendations block** in the snapshot output:

```
## Skill recommendations (auto-generated from this week's data)

- promote-newsletter: max_posts_per_channel_per_article currently = 3. Verbatim-quote engagement on LinkedIn personal averaged 0.4 reactions vs teaser at 1.2. **Recommend lowering to 2** for LinkedIn channels OR routing LinkedIn through tease-newsletter exclusively.
- carousel-newsletter: avg IG impressions 4,293 (1 post) vs IG verbatim avg 269 (9 posts). **Recommend running /carousel-newsletter on every newsletter** (current usage is opportunistic).
- promote-* skills (all): EVC LinkedIn page sent 8 posts past 7d, total 23 impressions across all of them, +0 followers. **Recommend raising min_followers_to_promote from 50 to 100** (the page is at 28; not worth fan-out at any volume).
```

Each recommendation should include the data citation (numerator/denominator + source format + channel) so the user can verify before adopting. The recommendations are **suggestions, not auto-applied changes** — the user reviews and either accepts (which triggers a SKILL.md edit + commit) or rejects (which gets logged as a "data didn't move us" note for next week).

This phase is what makes the system a closed loop: posts go out tagged → engagement gets attributed by tag → recommendations get generated from the attribution → user accepts/rejects → defaults shift → next batch of posts is better-targeted.

### Phase 5c — Render report

Print markdown to stdout. Fixed structure for diff-ability across weeks:

```markdown
# Buffer Stats — Weekly Snapshot (YYYY-MM-DD)

**Window:** last N days · **Organization:** <name> · **Channels:** M active

## Channels

| Channel | Queued | Sent (Nd) | Followers | Δ7d | Eng rate | Goal status |
|---|---:|---:|---:|---:|---:|:---:|
| LinkedIn (Mike Lady) | 5 | 3 | 1,234 | +7 | 4.2% | 🟢 OnTrack |
| Instagram (@evc) | 2 | 1 | 512 | +3 | 2.1% | 🟡 AtRisk |
| …

## Format performance

(See Phase 5 — per-(channel, format) engagement aggregation table.)

## Skill recommendations

(See Phase 5b — auto-generated suggestions based on this week's format-performance data.)

## Top posts (last N days)

| Rank | Channel | Impressions | Engagement | Rate | Sent | Snippet |
|---:|---|---:|---:|---:|---|---|
| 1 | LinkedIn | 4,210 | 139 | 3.3% | Apr 19 | Kai Greene was never Mr. Olympia... |
| 2 | Instagram | 2,830 | 88 | 3.1% | Apr 17 | … |
| …

## Flags

- 🔴 Disconnected: <none> | <list>
- 🟡 Paused queues: <list>
- 🟡 Goals at risk: <list>
- ⚪ Empty queues: <list> (queue depth <3)
- ⚪ Stale engagement: <channels with no data for N days>
- ⚪ Untagged posts: <count> (posts sent without a `format:` tag — closed-loop attribution gap)

## Raw numbers
<collapsed JSON path>
```

### Phase 6 — Write snapshot (unless `--no-cache`)

```bash
SNAP_DATE=$(date -u +%Y-%m-%d)
SNAP_JSON="$CACHE_DIR/snapshot-$SNAP_DATE.json"
SNAP_MD="$CACHE_DIR/snapshot-$SNAP_DATE.md"
# write both files
```

The `cache/` directory is gitignored — all snapshots stay local.

## Re-discovery (when selectors break)

Phase 2 selectors were confirmed 2026-04-20. When Buffer re-skins Analyze and the skill returns garbage, re-run discovery:

1. `$B goto https://analyze.buffer.com` and log in (handoff — subdomain cookies don't carry over).
2. For a channel page: `$B js "[...document.querySelectorAll('li')].slice(0,30).map(li => li.innerText).filter(t => t.split('\n').length >= 2)"` — look for metric-card text patterns `"Label\nValue\nDelta%"`.
3. For the Posts tab: `$B js "[...document.querySelectorAll('[class*=post-item]')].length"` — confirms the `post-item` class token still exists. If not, `$B js "[...document.querySelectorAll('button, a')].filter(el => el.innerText.trim() === 'View Post').map(el => el.closest('li')?.className)"` finds the current post-card class.
4. Update the `$B js` extraction blocks in Phase 2 and commit with an updated "as of YYYY-MM-DD" marker.

Text-pattern extractors (finding `<li>` by `innerText` shape) are more resilient than class-name selectors because Buffer's styled-components produce hash-based class names that change across deploys. Only use class selectors when Buffer adds a semantic token like `post-item` alongside the hash.

## Known issues / robustness notes

- **Subdomain cookie gap.** *Label: `Edge: subdomain-cookie-gap`* ~~Cookies imported for `buffer.com` do NOT automatically authenticate `analyze.buffer.com`.~~ **Updated 2026-04-27:** cookie import for `buffer.com` DOES carry to `analyze.buffer.com` in current Buffer setup — confirmed working via `$B cookie-import-browser chrome buffer.com` then nav to `analyze.buffer.com` succeeds without redirect-to-login. Try cookie import first; only fall back to manual handoff login if the cookie path actually fails.
- **No "Last 7 days" preset.** *Label: `Edge: no-last-7-days-preset`* Buffer Analyze only offers This/Last month, This/Last week, and Custom. The skill uses **Last week** for a 7-day window (complete Monday-Sunday). "This week" is broken on Mondays (0 days of data). For arbitrary windows, the skill falls back to Custom — implementation note below.
- **Date picker is inline, not a popover.** *Label: `Edge: analyze-picker-already-inline`* Confirmed 2026-05-18: on per-channel overview pages, all four preset buttons ("This month", "Last month", "This week", "Last week") are rendered inline in the page header — there's no "open the picker" click first. Just `$B click` the target preset directly. Earlier skill text suggested expanding a popover; that's a stale assumption.
- **Instagram has no Engagement Rate field.** *Label: `Edge: instagram-no-eng-rate-field`* Confirmed 2026-05-18: Buffer Analyze IG overview returns Posts/Impressions/Reach/Likes/Comments/Daily-avg-impressions/Avg-likes-per-post/Avg-comments-per-post — no aggregated `Engagement Rate` line. **Fallback:** compute it as `((likes + comments) / impressions) * 100`. Use impressions (not reach) as the denominator so the result is comparable to LinkedIn's engagement_rate field which is `(reactions + comments + reposts) / impressions`. Worked example: IG 943 imps, 10 likes, 0 comments → 1.06% eng rate.
- **Instagram requires Facebook Business link.** *Label: `Edge: instagram-not-linked`* If the IG channel isn't linked to a Facebook Business Page, Buffer shows an "Unlock Instagram Analytics" banner and no engagement data. The skill detects the banner and flags `engagement: { unavailable: true, reason: "ig_not_linked" }` in the JSON — doesn't fail the snapshot.
- **Facebook Pages impressions.** *Label: `Edge: facebook-impressions-unavailable`* Banner: "Learn why impressions are not available for Facebook Pages." For affected channels, the Impressions field is missing entirely. Parse as `null`, not `0`.
- **Buffer Analyze DOM uses hashed class names.** *Label: `Edge: analyze-hashed-classes`* Text-pattern `<li>` extractors are the primary pattern. When selectors break, see "Re-discovery" below.
- **Multi-organization accounts.** *Label: `Edge: multi-org-selection`* The skill prompts on first run and saves the choice to `config.local.json`. Re-pick by editing the file.
- **Engagement data lag.** *Label: `Edge: engagement-data-lag`* Buffer Analyze sometimes lags 24-48h for new channels or freshly published posts. Fields render as `null` with a footnote rather than `0`.
- **MCP permission prompts.** The Buffer MCP's read-only tools (`get_account`, `list_channels`, `list_posts`, `get_channel`, `get_post`) are used heavily. Consider adding them to `~/.claude/settings.json` allowlist if prompts get annoying. (`get_account` and `list_channels` are already globally allowed.)
- **Custom date ranges.** *Label: `Edge: custom-date-range-unimplemented`* For `--days` values other than 7 or ~30, the skill must open the Custom picker and select start/end dates programmatically. Not yet implemented — falls back to "Last week" with a warning. Add the custom-picker handling when needed.
- **Delta bootstrap.** *Label: `Edge: delta-bootstrap`* First run has no prior snapshot → deltas show `—`. After one week, numbers mean something.
- **Engagement-tracked vs posting channel conflation.** *Label: `Edge: engagement-vs-posting-channel-conflation`* Buffer Analyze can only scrape per-post engagement for FB pages, IG business, and LinkedIn pages — NOT LinkedIn personal or Threads. **However (updated 2026-05-19, task #377):** Buffer Insights covers all 6 channels in aggregate (posts + reactions + comments per channel). The skill now extracts Insights per-channel rows AND falls back to a posts-only ROI formula for channels without impressions — so `engagement_tracked_channels` is typically 6, not 3. `channels_engagement_unavailable[]` is reserved for channels with zero coverage on either surface (rare). Channels using the posts-only path are listed separately under `channels_impressions_unavailable[]`. If the snapshot collapses any of these into one count, downstream consumers like `/flywheel` report a false total (caught 2026-05-03 when flywheel reported total_followers=26 against an actual ~2,200).
- **Insights per-channel row extraction.** *Label: `Edge: insights-row-extraction-failed`* The `publish.buffer.com/insights` per-channel rows (Phase 2a-bis) are the only source for LinkedIn personal + Threads aggregate engagement. If the row regex returns fewer rows than `posting_channels`, Buffer changed the Insights DOM. Symptoms: 3 of the 6 channels read `posts: null, reactions: null` in `channel_roi[]`; the rest (Analyze-covered) still populate normally. Fix: open Insights manually, inspect the channel-row block in DevTools, update the regex in the Phase 2a-bis `$B js` snippet to match the new shape. The summary tiles (top-of-page Posts/Followers/Reactions/Comments) and top-posts extraction live in separate selectors and are not affected by this edge.
- **`operational` fast-path.** For mid-week queue checks without engagement scraping (much faster, no browser), use `/buffer-stats operational`.

## Feeds into /flywheel

`/flywheel` reads the most recent Buffer snapshot from `cache/snapshot-*.json` without re-running this skill every time. The JSON shape is stable; `/flywheel` uses:
- `channels[].engagement.followers` summed across channels → cross-channel follower total
- `channels[].engagement.engagement_rate` → per-channel engagement score
- `top_posts[]` → top-3 cross-channel performers

If the newest Buffer snapshot is older than `stale_snapshot_days` (14), flywheel flags it the same way it flags stale LinkedIn snapshots.
