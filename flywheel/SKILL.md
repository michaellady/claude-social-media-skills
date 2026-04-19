---
name: flywheel
description: Use when user wants a weekly dashboard of the Enterprise Vibe Code growth flywheel — "flywheel report", "weekly rollup", "how's the flywheel spinning", "am I on track", "week over week growth", "priorities check". Produces a single markdown report against the 5 growth-plan priorities.
user_invocable: true
---

# flywheel

Aggregate signal from YouTube, beehiiv, LinkedIn, and the consulting log into one weekly report against the 5 priorities in `~/dev/youtube_analytics/enterprise_vibe_code_growth_priorities.md`. Answers "is the flywheel spinning this week?" with specific numbers, not vibes.

## Usage

`/flywheel` — full report for the last 7 days, saves snapshot
`/flywheel --days 30` — custom window
`/flywheel --no-save` — produce the report but don't overwrite today's snapshot
`/flywheel --compare 2026-04-12` — diff against a specific older snapshot

## Data sources

| Source | How | What it gives |
|---|---|---|
| YouTube | `go run . analyze --since <date>` in `~/dev/youtube_analytics` | streams/week, long-form count, views, revenue, subs |
| beehiiv list | `mcp__beehiiv__beehiiv_stats` | current subscriber count |
| beehiiv attribution | `mcp__beehiiv__beehiiv_attribution` | source mix (YouTube vs LinkedIn vs direct) |
| LinkedIn | `~/dev/claude-social-media-skills/linkedin-stats/cache/snapshot-<latest>.json` | newsletter subs, profile + page followers |
| Consulting | `(cd ~/dev/consulting-log && ./cl json)` | pipeline stages, realized revenue, content gaps |

If any source fails or is stale, note it in the report — don't silently drop the row.

## Process

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

**Priority 1 check** (streams 3-4×/week):
- actual_lives / DAYS * 7 = streams_per_week
- target: 3-4
- status: on_track if ≥3, behind otherwise

**Priority 4 check** (long-form volume):
- target: 2-3 long-form videos per week
- actual_long_form / DAYS * 7 = long_form_per_week
- status: on_track if ≥2

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

### Phase 5 — Consulting pipeline

```bash
CL_JSON=$(cd ~/dev/consulting-log && ./cl json 2>/dev/null)
if [ -z "$CL_JSON" ]; then
  CL_STATUS="no deals logged"
else
  # Aggregate by stage and compute content gaps.
  TOTAL_PIPELINE=$(printf '%s' "$CL_JSON" | jq '[.[] | .estimated_value] | add // 0')
  TOTAL_REVENUE=$(printf '%s' "$CL_JSON" | jq '[.[] | .actual_revenue] | add // 0')
  GAPS=$(printf '%s' "$CL_JSON" | jq '[.[] | select((.status == "delivered" or .status == "closed") and (.content_pieces | length) == 0)] | length')
fi
```

**Priority 5 check** (every engagement → content):
- gaps == 0 means fully on track
- status: each gap is a broken flywheel link

### Phase 6 — Compose report

Build the markdown with a fixed structure so snapshots are diffable week-over-week:

```markdown
# Enterprise Vibe Code — Flywheel Snapshot

**Window:** YYYY-MM-DD → YYYY-MM-DD (N days)
**Generated:** YYYY-MM-DDTHH:MM:SSZ

## Priority 1 — Stream 3-4×/week
- Streams this window: N (target: 3-4/week)
- Pace: X/week
- Status: [🟢 on track | 🟡 under | 🔴 off]

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

## Priority 4 — Long-form ≥ shorts volume
- Long-form this window: N (target: 2-3/week)
- Shorts this window: N
- Ratio: long-form / shorts
- Status: [🟢 | 🟡 | 🔴]

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
  "consulting": { "pipeline": F, "realized_revenue": F, "content_gaps": N }
}
```

Diffing JSON is trivially reliable even if the markdown structure evolves.

## Growth-plan hook

After a few weeks of running `/flywheel` every Sunday, the snapshots directory becomes a diff-able history of the flywheel's state. A future `/flywheel --trend` mode can plot week-over-week acceleration across all five priorities.

## Known issues

- **beehiiv MCP requires Claude Code restart** after `make install`ing the server. If the attribution tool is missing, remind the user.
- **LinkedIn snapshots must be current.** If `/linkedin-stats` hasn't been run recently, Priority 3 will show stale numbers. The report should flag any snapshot older than 14 days as unreliable.
- **YouTube data.** `youtube_analytics` `analyze` reads `data/videos.json` which is only refreshed on `fetch`. If it's stale, the YouTube section will be too. Run `go run . fetch` in `~/dev/youtube_analytics` before running `/flywheel` if the numbers look off.
- **Consulting log is local-only.** No data migrates from other tools. If the user uses a CRM, they have to update the markdown files themselves.
