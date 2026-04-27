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

### Phase 3 — Compose snapshot

Combine operational + engagement data into a single JSON object:

```json
{
  "fetched_at": "2026-04-20T17:00:00Z",
  "window_days": 7,
  "organization": { "id": "...", "name": "..." },
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

### Phase 5 — Render report

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

- **Subdomain cookie gap.** ~~Cookies imported for `buffer.com` do NOT automatically authenticate `analyze.buffer.com`.~~ **Updated 2026-04-27:** cookie import for `buffer.com` DOES carry to `analyze.buffer.com` in current Buffer setup — confirmed working via `$B cookie-import-browser chrome buffer.com` then nav to `analyze.buffer.com` succeeds without redirect-to-login. Try cookie import first; only fall back to manual handoff login if the cookie path actually fails.
- **No "Last 7 days" preset.** Buffer Analyze only offers This/Last month, This/Last week, and Custom. The skill uses **Last week** for a 7-day window (complete Monday-Sunday). "This week" is broken on Mondays (0 days of data). For arbitrary windows, the skill falls back to Custom — implementation note below.
- **Instagram requires Facebook Business link.** If the IG channel isn't linked to a Facebook Business Page, Buffer shows an "Unlock Instagram Analytics" banner and no engagement data. The skill detects the banner and flags `engagement: { unavailable: true, reason: "ig_not_linked" }` in the JSON — doesn't fail the snapshot.
- **Facebook Pages impressions.** Banner: "Learn why impressions are not available for Facebook Pages." For affected channels, the Impressions field is missing entirely. Parse as `null`, not `0`.
- **Buffer Analyze DOM uses hashed class names.** Text-pattern `<li>` extractors are the primary pattern. When selectors break, see "Re-discovery" below.
- **Multi-organization accounts.** The skill prompts on first run and saves the choice to `config.local.json`. Re-pick by editing the file.
- **Engagement data lag.** Buffer Analyze sometimes lags 24-48h for new channels or freshly published posts. Fields render as `null` with a footnote rather than `0`.
- **MCP permission prompts.** The Buffer MCP's read-only tools (`get_account`, `list_channels`, `list_posts`, `get_channel`, `get_post`) are used heavily. Consider adding them to `~/.claude/settings.json` allowlist if prompts get annoying. (`get_account` and `list_channels` are already globally allowed.)
- **Custom date ranges.** For `--days` values other than 7 or ~30, the skill must open the Custom picker and select start/end dates programmatically. Not yet implemented — falls back to "Last week" with a warning. Add the custom-picker handling when needed.
- **Delta bootstrap.** First run has no prior snapshot → deltas show `—`. After one week, numbers mean something.
- **`operational` fast-path.** For mid-week queue checks without engagement scraping (much faster, no browser), use `/buffer-stats operational`.

## Feeds into /flywheel

`/flywheel` reads the most recent Buffer snapshot from `cache/snapshot-*.json` without re-running this skill every time. The JSON shape is stable; `/flywheel` uses:
- `channels[].engagement.followers` summed across channels → cross-channel follower total
- `channels[].engagement.engagement_rate` → per-channel engagement score
- `top_posts[]` → top-3 cross-channel performers

If the newest Buffer snapshot is older than `stale_snapshot_days` (14), flywheel flags it the same way it flags stale LinkedIn snapshots.
