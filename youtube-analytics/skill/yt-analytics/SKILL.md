---
name: yt-analytics
description: Use when user asks about YouTube analytics, revenue, subscribers, watch time, retention, video performance, title/tag analysis, duplicate detection, data-quality checks, or wants to fetch/view/export channel data. Also triggers on cohort grouping/tagging, hypothesis tracking, weekly/closed-loop reviews, and per-cohort performance. Triggers on "check analytics", "fetch latest", "youtube stats", "how are my videos doing", "best titles", "recent performance", "compare to last run", "since last fetch", "weekly review", "closed loop", "how is the X cohort doing", "what did we predict last week", "grade hypotheses", "cohort report", "insights pending".
user_invocable: true
---

# yt-analytics

Fetch YouTube channel data, generate reports/dashboards, and run targeted queries via the CLI.

## Project Location

`/Users/mikelady/dev/claude-social-media-skills/youtube-analytics` (channel: Enterprise Vibe Code)

## CLI Commands

Run from the project directory:

| Command | What it does | Duration |
|---------|-------------|----------|
| `go run . fetch` | Pull video metadata from YouTube Data API + auto-snapshot | 1-2 min |
| `go run . fetch-analytics` | Pull watch time, retention, subs, revenue (OAuth2) | 2-5 min |
| `go run . fetch-analytics --all` | Above + per-day, traffic-source, sub-status breakdowns | 5-10 min |
| `go run . fetch-comments` | Pull top-level comments per video → `data/comments.json` | 2-5 min |
| `go run . analyze` | Print terminal report | instant |
| `go run . dashboard` | Generate `data/dashboard.html` and open in browser | instant |
| `go run . find-duplicates` | Detect near-duplicate uploads | instant |
| `go run . data-quality` | Missing tags, templated descriptions, etc. | instant |
| `go run . titles` | Title pattern analysis with keyword lift | instant |
| `go run . video <id>` | Deep-dive on a single video | instant |
| `go run . export` | Flat CSV/TSV export | instant |
| `go run . cohort <subcmd>` | Local cohort tagging — list/show/assign/auto/report | instant |
| `go run . insights <subcmd>` | Hypothesis ledger — list/pending/grade/new | instant |

Analytics window for `fetch-analytics` is fixed at 2025-12-01 → today in `fetch_analytics.go`. Change that file if you need a different baseline.

### fetch-analytics dimensional flags

The base `fetch-analytics` only pulls aggregate-over-window metrics. The flags below add additive dimensional data — aggregate fields stay populated, the new fields just go alongside them:

| Flag | Adds to each video | Useful for |
|------|---------------------|------------|
| `--daily` | `daily_metrics[]` — date, views, watch_min, retention, subs | Date-correlation hypotheses ("did newsletter mention spike views on 2026-04-23?") |
| `--traffic-sources` | `traffic_sources{}` — keyed by `YT_SEARCH`, `RELATED_VIDEO`, `SUBSCRIBER`, `EXT_URL`, `BROWSE`, `NOTIFICATION`, etc. | "Did the format-fatigue collapse come from SUBSCRIBER traffic drying up, or RELATED_VIDEO suppression?" |
| `--sub-status` | `sub_status_metrics{SUBSCRIBED, UNSUBSCRIBED}` | Cleaner conversion signal — `subs/1K` over `UNSUBSCRIBED.views` is the actual conversion target |
| `--all` | All three at once | Default for closed-loop weekly review |

Each flag adds one extra batched API call per fetch. Daily uses a smaller batch size (50 vs 200) because rows-per-video balloon with the day dimension.

### fetch-comments

`fetch-comments` writes top-level comments for filtered videos to `data/comments.json`, keyed by video ID. Re-running merges by ID — videos NOT in the current run keep their previously-fetched comments.

```bash
# All videos (uses the full filter set — flags must precede positional)
go run . fetch-comments --limit 20

# Just past-week long-forms in a cohort
go run . fetch-comments --since 2026-04-19 --type long-form --cohort gastown-series --limit 10

# Time-ordered (most recent first) instead of relevance
go run . fetch-comments --order time --limit 5
```

Comments are qualitative signal. Use them when grading hypotheses: read the model the top-3 comments per cited video and let it spot themes (confusion, requests, push-back). Don't try to classify sentiment in Go — that's cognition, belongs in the prompt.

## Filter Flags

**Available on `analyze`, `dashboard`, `find-duplicates`, `data-quality`, `titles`, `export`:**

| Flag | Purpose |
|---|---|
| `--since YYYY-MM-DD` | Published on/after |
| `--until YYYY-MM-DD` | Published on/before |
| `--type <short\|long-form\|live>` | Video type (repeatable) |
| `--duration-min N` / `--duration-max N` | Duration bounds in seconds |
| `--exclude <ID>` | Skip specific video (repeatable) |
| `--cohort <id>` | Filter to a cohort id from `data/cohorts.yaml` (repeatable) |

> **Go stdlib flag note:** when a command takes both filter flags and a positional argument (e.g. `cohort show <id>`, `insights grade <id>`, `video <id>`), the flags must come *before* the positional. `cohort show --since 2026-04-19 gastown-series` works; `cohort show gastown-series --since 2026-04-19` silently ignores the `--since` flag.

## Typical Workflows

**Full refresh + report** ("fetch latest", "run analytics again", "since we last ran it"):
```bash
cd /Users/mikelady/dev/claude-social-media-skills/youtube-analytics
go run . fetch
go run . fetch-analytics --all   # aggregate + per-day + traffic-sources + sub-status
go run . cohort auto             # refresh cohort assignments from rules
go run . analyze > data/analytics_report.md
```

**Report only** (cached data): `go run . analyze`

**Dashboard**: `go run . dashboard` (add filter flags for a scoped view)

## Sister skills

When a NEW video drops:
- **`/yt-launch <id>`** — sets up monitoring (cohort + hypothesis + daily LaunchAgent). Run this first.
- **`/yt-ab <id>`** — A/B tests title via YouTube Studio's native Test and Compare. Run after `/yt-launch`. Drives Studio in your logged-in Chrome via `mcp__claude-in-chrome`.

These are user-invocable slash commands that orchestrate this skill's primitives plus browser automation. Mention them when the user asks "should I track this launch?" or "how do I A/B test this title?".

## Closed-Loop Weekly Review (cohorts + hypotheses)

The weekly review is a **skill workflow**, not a single command. The CLI handles transport (data, file IO, deterministic rules); the model handles cognition (grading, hypothesis generation). This split is deliberate — see `docs/primitive-test.md`.

Cohorts are named groups of videos defined in `data/cohorts.yaml` with optional auto-match rules (regex on title/description, video_type, duration, publish date). Run `go run . cohort auto` to (re)derive assignments after `fetch`. Manual-only cohorts (no rules) are managed via `cohort assign/unassign`.

Insights live in `data/insights/<YYYY-MM-DD>.md` with YAML frontmatter:
```yaml
---
date: 2026-04-26
hypotheses:
  - id: h-2026-04-26-1
    cohort: gastown-series
    prediction: Title variation will lift retention on next 3 long-forms
    evidence_video_ids: [abc, def, ghi]
    metric: retention
    direction: up
    evaluate_after: 2026-05-10
    outcome: ""
    verdict: ""
---
```

### Weekly review workflow

1. **Refresh data** (transport):
   ```bash
   go run . fetch && go run . fetch-analytics --all
   go run . cohort auto
   # Optional: fetch fresh comments for any cohort under active hypothesis
   go run . fetch-comments --since <last-monday> --limit 10
   ```
   Use `--all` so `insights pending` can surface the per-day trajectory, traffic sources, and sub-status split for cited videos — without those dimensions you're stuck with the same aggregate the prior cycle had.

2. **Pull the numbers the model needs** (transport):
   ```bash
   go run . analyze --compare-periods <prev-week> <this-week>
   go run . cohort report --since <last-monday>
   go run . insights pending          # past-due hypotheses + current metrics
   ```

3. **GRADE pending hypotheses** (cognition — model judgment, NOT Go):
   For each hypothesis emitted by `insights pending`:
   - Read its prediction + the cited videos' current metrics (already in the output)
   - Decide verdict: `confirm` / `refute` / `inconclusive` — explain reasoning
   - Persist (flags must precede the hypothesis-id positional argument):
     ```bash
     go run . insights grade --verdict <v> --outcome "<what actually happened>" <id>
     ```

4. **Report to the user** with:
   - Cohort movement first (lead here, not raw totals)
   - WoW deltas
   - Verdicts on hypotheses graded this cycle
   - 1-3 NEW hypotheses for next week

5. **Persist the new hypotheses** (cognition writing transport-readable files):
   ```bash
   go run . insights new <this-monday>     # creates the file
   ```
   Then edit the file directly to add hypothesis entries with frontmatter (id, cohort, prediction, evidence_video_ids, metric, direction, evaluate_after).

### Why this isn't a single `review` command

Mechanical "did metric X move by N?" grading and auto-generated hypothesis stubs would fail the Bitter Lesson — a smarter model produces better verdicts and better hypotheses from the prompt than from a hardcoded comparator. Go shuttles the data; the model brings the judgment.

## "Since we last ran it" Pattern

`fetch` auto-snapshots to `data/snapshots/videos-<UTC timestamp>.json` and keeps the last 30.

To answer "what changed since last run":
```bash
# Most recent snapshot
ls -t data/snapshots/videos-*.json | head -1

# Delta since then
go run . analyze --diff data/snapshots/videos-<timestamp>.json
```

The snapshots directory is the history — no manual bookkeeping needed.

## OAuth

`fetch-analytics` uses OAuth2. Token cached at `data/token.json`. Expired token → browser opens on `localhost:8089/callback`. Scopes: `yt-analytics.readonly` + `yt-analytics-monetary.readonly`.

## Data Schema (`data/videos.json`)

`ChannelData` has `channel_id`, `channel_title`, `fetched_at`, `has_analytics`, `analytics_fetched_at`, `videos[]`. Each video:

| Field | Type | Notes |
|---|---|---|
| `id` | string | YouTube video ID |
| `title`, `description`, `tags` | string/[]string | `tags` often null |
| `published_at` | RFC3339 string | |
| `duration_seconds` | int | short: ≤60, long-form: >60 non-live, live: hasLiveBroadcast |
| `video_type` | string | `"short"` \| `"long-form"` \| `"live"` |
| `view_count`, `like_count`, `comment_count` | int | |
| `estimated_minutes_watched` | float | requires `fetch-analytics` |
| `average_view_duration` | float (seconds) | |
| `average_view_percentage` | float (0-100) | retention |
| `subscribers_gained`, `subscribers_lost` | int | |
| `estimated_revenue` | float (USD) | monetization scope |
| `cpm`, `ad_impressions`, `monetized_playbacks` | | |

## Common Recipes

**Recent 3-10 min long-form performance:**
```bash
go run . analyze --since 2026-04-01 --type long-form --duration-min 180 --duration-max 600
```

**Best live-stream titles:**
```bash
go run . titles --top 10 --by views --type live
# Or by conversion: --by subs / --by retention / --by revenue
```

**Data quality audit on a slice:**
```bash
go run . data-quality --since 2026-04-01 --type long-form
```

**Compare two months side-by-side:**
```bash
go run . analyze --compare-periods 2026-01-01..2026-01-31 2026-03-01..2026-03-31
```

**CSV export for spreadsheet work:**
```bash
go run . export --format csv --since 2026-04-01 --output recent.csv
```

**One-video deep-dive:**
```bash
go run . video Rn0Oc9U0AOE
```

**Did newsletter mention drive views on a specific day?** (requires `fetch-analytics --daily` first):
```bash
# Filter daily_metrics[] for the newsletter send date
jq '.videos[] | select(.video_type=="long-form") | {id, title, day: (.daily_metrics[]? | select(.date == "2026-04-23"))}' data/videos.json | head -40
```

**Conversion target only (UNSUBSCRIBED viewers):** subs/1K is contaminated by SUBSCRIBED viewers who can't sub again. After `fetch-analytics --sub-status`:
```bash
jq -r '.videos[] | select(.sub_status_metrics.UNSUBSCRIBED.views > 100) | [
  ((.subscribers_gained // 0) / .sub_status_metrics.UNSUBSCRIBED.views * 1000),
  .sub_status_metrics.UNSUBSCRIBED.views,
  .title
] | @tsv' data/videos.json | sort -rn | head -10
```

**Where did views come from?** (after `fetch-analytics --traffic-sources`):
```bash
go run . video <id>     # video deep-dive doesn't yet surface traffic sources, so:
jq '.videos[] | select(.id=="<video-id>") | .traffic_sources' data/videos.json
```

## Escape Hatch: jq on `data/videos.json`

For one-off slices the CLI doesn't cover, jq still works:

```bash
# Aggregate over a filtered slice
jq '[.videos[] | select(.published_at >= "2026-04-01" and .video_type == "long-form")] | {count: length, avg_views: (map(.view_count) | add/length)}' data/videos.json

# Subs per 1K views ranking
jq -r '.videos[] | select(.video_type == "live" and .view_count >= 100) | [((.subscribers_gained // 0) / .view_count * 1000), .title] | @tsv' data/videos.json | sort -rn | head -10
```

Prefer the CLI when it fits — jq is for novel questions.

## Analysis Playbook

Match the user's request to the command:

| User asks | Command |
|---|---|
| "best/top titles for X" | `titles --top 10 --by views --type <X>` |
| "analyze recent [type]" | `analyze --since <date> --type <type>` |
| "since last run" | `analyze --diff <latest snapshot>` |
| "what's hurting the channel" | `data-quality` + look for low-retention+high-views in `analyze` |
| "duplicate check" | `find-duplicates` |
| "which format works" | `analyze` (look at subs/1K views by type) |
| "compare X period to Y" | `analyze --compare-periods A..B C..D` |
| "how is the X cohort doing" | `cohort report` or `analyze --cohort X` |
| "weekly review" / "closed loop" | run the Closed-Loop Weekly Review workflow above |
| "what did we predict last week" | `insights pending` then grade verdicts |
| "tell me about this video" | `video <id>` |
| "dump to spreadsheet" | `export --format csv --output path.csv` |

## Reporting Conventions

When presenting analytics to the user:
- **Lead with cohort movement** when cohorts are defined — that's the unit of intentional decision-making, not raw channel totals
- **Lead with the delta** if comparing to a previous run (new videos, new subs, new revenue)
- **Subs per 1K views** is the cleanest conversion metric — live streams dominate (~17) vs shorts (~0.7)
- **Retention + views together** — high views with low retention is a red flag, not a win
- **Flag data-quality issues proactively** (null tags, templated descriptions, duplicate uploads)
- **Don't rank by views alone** when the user is deciding what content to make — weight by subs gained and retention
- **Hypothesis grading is your judgment** — `insights pending` gives you the numbers; you give the verdict

## Common Pitfalls

- **`average_view_percentage` >100%** is possible on very short videos (rewatches). Always pair with view count and duration.
- **`video_type` classification happens at fetch time** — shorts are `duration_seconds ≤ 60`; re-run `fetch` if boundaries seem wrong.
- **OAuth token at `data/token.json`** is personal; never commit.
- **Analytics are aggregate-only** — the CLI doesn't do per-day breakdowns.
- **`--until` is inclusive** — the whole final day is included (rolls to 23:59:59).

## Skill Lives in the Repo

This skill file is symlinked from `~/.claude/skills/yt-analytics` → `/Users/mikelady/dev/claude-social-media-skills/youtube-analytics/skill/yt-analytics`. Edit in the repo; the symlink picks up changes automatically.
