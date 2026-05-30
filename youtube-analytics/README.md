# YouTube Analytics CLI

A Go CLI tool that fetches your YouTube channel data and generates detailed analytics reports and interactive dashboards. Includes watch time, retention, subscriber impact, and posting pattern analysis.

## Features

- **Video metadata** via YouTube Data API v3 (views, likes, comments, duration, tags)
- **Watch time analytics** via YouTube Analytics API v2 with OAuth2 (estimated minutes watched, average view duration, audience retention, subscriber gains/losses)
- **Dimensional breakdowns** via `fetch-analytics --all` (per-day series, traffic sources, SUBSCRIBED vs UNSUBSCRIBED split)
- **Comments** via `fetch-comments` for qualitative signal (top-level, idempotent merge, quota guardrail)
- **Local cohort tagging** with auto-assignment rules — group videos into named buckets (`gastown-series`, `live-deep-dives`, etc.) and filter every command with `--cohort`
- **Hypothesis ledger** for closed-loop weekly review — propose, surface past-due, grade with cited videos' current metrics
- **Terminal report** with posting patterns, engagement trends, title analysis, watch time breakdowns, traffic-source mix, UNSUBSCRIBED conversion ranking
- **Interactive HTML dashboard** with Chart.js visualizations

## Prerequisites

- [Go 1.21+](https://go.dev/dl/)
- A [Google Cloud](https://console.cloud.google.com) project with:
  - **YouTube Data API v3** enabled
  - **YouTube Analytics API** enabled (for watch time data)

## Setup

### 1. Get a YouTube Data API key

1. Go to [Google Cloud Console](https://console.cloud.google.com) > APIs & Services > Credentials
2. Click **Create Credentials** > **API Key**
3. (Recommended) Restrict the key to YouTube Data API v3

### 2. Create a `.env` file in the project root

```
YOUTUBE_API_KEY=your-api-key-here
CHANNEL_HANDLE=@YourChannelHandle
```

### 3. (Optional) Set up OAuth2 for watch time data

The `fetch-analytics` command requires OAuth2 authentication as the channel owner.

1. In Google Cloud Console > APIs & Services > Credentials, click **Create Credentials** > **OAuth client ID**
2. Application type: **Desktop app**
3. Download the JSON and save it as `client_secret.json` in the project root
4. Go to **OAuth consent screen** and add your Google account email as a **test user**
5. Under the client's settings, add `http://localhost:8089/callback` as an **Authorized redirect URI**

## Usage

```bash
# Install dependencies
go mod download

# Fetch video metadata (uses API key)
go run . fetch

# Fetch watch time analytics (opens browser for OAuth2)
go run . fetch-analytics

# Print terminal report
go run . analyze

# Generate interactive HTML dashboard
go run . dashboard
```

### Commands

| Command | Description |
|---|---|
| `fetch` | Fetch all video metadata from YouTube Data API v3. Saves to `data/videos.json` and snapshots to `data/snapshots/` (keeps last 30) |
| `fetch-analytics` | Fetch watch time data via OAuth2. Merges into `data/videos.json`. Flags: `--daily`, `--traffic-sources`, `--sub-status`, `--all` |
| `fetch-comments` | Fetch top-level comments per video into `data/comments.json`. Flags: `--limit`, `--order`, `--max-age`, `--force`, `--dry-run` |
| `fetch-peers` | Pull top recent videos from peer channels in `data/peer-channels.yaml` into `data/peer-videos.json`. Used by `/yt-ab` for title grounding. Flags: `--window` (default 90d), `--top` (default 15), `--dry-run` |
| `analyze` | Print a detailed terminal report. Supports `--diff <snapshot>` and `--compare-periods A..B C..D` modes |
| `dashboard` | Generate an interactive HTML dashboard at `data/dashboard.html` |
| `find-duplicates` | Detect near-duplicate uploads (same title, similar duration, close timestamps) |
| `data-quality` | Report videos missing tags, with short or templated descriptions, or missing thumbnails |
| `titles` | Title pattern analysis with keyword lift for top performers |
| `video <id>` | Deep-dive on a single video with all analytics fields, traffic sources, sub-status, daily trajectory, cohorts, and top comments |
| `export` | Flatten video data to CSV or TSV |
| `cohort <subcmd>` | Local cohort tagging: `list`, `show <id>`, `assign <id> <video-id>...`, `unassign`, `auto`, `report` |
| `insights <subcmd>` | Hypothesis ledger: `list`, `pending`, `grade --verdict <v> --outcome "<text>" <id>`, `new [YYYY-MM-DD]` |

### Filter flags

`analyze`, `dashboard`, `find-duplicates`, `data-quality`, `titles`, `export`, `suggest-tags`, `update-tags`, `cohort show`, `cohort report`, and `fetch-comments` all accept:

| Flag | Description |
|---|---|
| `--since YYYY-MM-DD` | Videos published on or after this date |
| `--until YYYY-MM-DD` | Videos published on or before this date |
| `--type <short\|long-form\|live>` | Filter by video type (repeatable) |
| `--duration-min N` | Minimum duration in seconds |
| `--duration-max N` | Maximum duration in seconds |
| `--exclude <ID>` | Exclude a specific video ID (repeatable) |
| `--cohort <id>` | Filter to a cohort id from `data/cohorts.yaml` (repeatable) |

> **Go stdlib flag note:** for commands that take both filter flags and a positional argument (`cohort show <id>`, `insights grade ... <id>`, `video <id>`), flags must precede the positional. `cohort show --since 2026-04-19 gastown-series` works; `cohort show gastown-series --since 2026-04-19` silently ignores the flag.

Examples:

```bash
# Recent 3-10 min long-form only
go run . analyze --since 2026-04-01 --type long-form --duration-min 180 --duration-max 600

# Title pattern for top live streams
go run . titles --top 10 --by views --type live

# Export recent videos to CSV
go run . export --format csv --since 2026-04-01 --output recent.csv

# Delta since a snapshot
go run . analyze --diff data/snapshots/videos-20260401T000000Z.json

# Compare two months side-by-side (filter flags propagate into both ranges)
go run . analyze --cohort gastown-series --compare-periods 2026-01-01..2026-01-31 2026-03-01..2026-03-31
```

## Closed-loop weekly review

Cohorts + insights ledger turn the CLI from a one-shot dashboard into a system that tracks what you predicted, grades it next week, and informs the next round of hypotheses. The split is deliberate: Go does transport (deterministic rule application, file IO, dimensional rollups); the model does cognition (verdicts, hypothesis generation). See [`docs/primitive-test.md`](docs/primitive-test.md) for the framework.

```bash
# 1. Refresh data (transport)
go run . fetch
go run . fetch-analytics --all          # aggregate + per-day + traffic + sub-status
go run . cohort auto                    # re-derive cohort assignments from rules
go run . fetch-comments --since <last-monday>   # qualitative signal, with quota guard

# 2. Pull the numbers (transport)
go run . analyze --compare-periods <prev-week> <this-week>
go run . cohort report --since <last-monday>
go run . insights pending               # past-due hypotheses + cited videos' current metrics

# 3. Grade past hypotheses (cognition — flags before positional)
go run . insights grade --verdict <confirm|refute|inconclusive> --outcome "<text>" <id>

# 4. Propose new hypotheses for next week
go run . insights new <this-monday>
# ... edit data/insights/<this-monday>.md to add hypothesis frontmatter
```

Cohort definitions live in `data/cohorts.yaml`. Each has optional auto-match rules (regex on title/description, video type, duration, publish date). Cohorts with no rules are manual-only — managed via `cohort assign/unassign`.

Hypotheses live in `data/insights/<YYYY-MM-DD>.md` with YAML frontmatter (id, cohort, prediction, evidence_video_ids, metric, direction, evaluate_after, outcome, verdict).

## Development

```bash
make hooks-install         # one-time per clone — activates the pre-commit hook
make check                 # go vet + go test ./...
```

The pre-commit hook in `.githooks/pre-commit` runs `go vet` and `go test` whenever `.go/.mod/.sum` files are staged. Bypass with `git commit --no-verify` if you really need to (rarely).

### New-video launch + A/B testing (slash commands)

Two user-invocable Claude skills cover the launch playbook:

- **`/yt-launch <video-url-or-id>`** — sets up monitoring for a freshly published video: tags it into a cohort, writes a launch hypothesis to the insights ledger, installs the daily launch-watch LaunchAgent. Pure orchestration of existing CLI primitives.
- **`/yt-ab <video-url-or-id>`** — generates 3 candidate titles grounded in (a) the video transcript, (b) Mike's own top-retention long-forms, and (c) top recent videos from peer channels in the AI-coding niche (refreshed weekly by `fetch-peers`). Drives YouTube Studio's native *Test and Compare* via `mcp__claude-in-chrome` to kick off the test. Records the experiment to `data/launch-experiments/<id>.yaml`.

Both skills live in `skill/yt-launch/SKILL.md` and `skill/yt-ab/SKILL.md`, symlinked into `~/.claude/skills/`. The Sunday weekly review surfaces any running A/B experiments.

For `/yt-ab`, install the transcript helper once: `pipx install youtube-transcript-api`. Or pass `--transcript <path>` with a manually-pasted transcript.

**Cadence — important truth:** YT's native Test and Compare serves variants *concurrently* over ~2 weeks and picks a winner by **watch time** (not CTR). There is no "every hour vs 4 hours vs half-day" cadence to pick — that question only applies to *manual sequential rotation*, which is appropriate only for descriptions (where YT has no native test). For descriptions, rotate every 24 hours at midnight Pacific, starting 2-3 weeks after publish.

### Scheduled weekly review (macOS)

Install a LaunchAgent that runs the closed-loop review every Sunday at 9 AM local:

```bash
make schedule-install      # symlinks plist to ~/Library/LaunchAgents/ and loads it
make schedule-test         # fires the script immediately, end-to-end
make schedule-uninstall    # removes the LaunchAgent
```

The script (`scripts/weekly-review.sh`) does the full Pattern B loop:

1. `git pull` (in case any cloud-side scaffold landed during the week)
2. Refreshes data: `fetch`, `fetch-analytics --all`, `cohort auto`, `fetch-comments --since <last-monday>`
3. Invokes `claude -p` headlessly with a self-contained prompt that grades any past-due hypotheses, proposes 1–3 new ones, writes the report to `data/reviews/<date>.md`, commits the changed insights files, and pushes to main
4. Posts a macOS notification when finished

Logs land in `~/Library/Logs/yt-weekly-review/<date>.log`. Reports land in `data/reviews/<date>.md` (gitignored).

Requirements: `claude` CLI installed and authenticated (which it is if you're reading this); `.env`, `client_secret.json`, and a valid `data/token.json` for the OAuth fetch. The script will fail fast if any are missing.

## What You Get

### Terminal Report (`analyze`)

- Video count breakdown (shorts, long-form, live)
- Shorts bulk posting impact on long-form performance
- Monthly performance trends by content type
- Top/bottom videos by views and engagement
- Title analysis (top vs bottom quartile)
- Watch time overview, by content type, and monthly trends
- Top videos by watch time and retention

### HTML Dashboard (`dashboard`)

- Stat cards (total videos, shorts, long-form, live, watch time, retention, subscribers)
- Video performance timeline (scatter chart)
- Monthly trends by content type (bar + engagement line charts)
- Shorts posting frequency vs long-form performance (dual-axis)
- Monthly watch time and retention (dual-axis)
- Watch time by content type (doughnut)
- Subscriber impact by type (grouped bar)
- Sortable tables for top videos

All watch time sections gracefully degrade if `fetch-analytics` hasn't been run.

## Project Structure

```
.
├── main.go                       # CLI entry, flag.FlagSet dispatch, shared filter flags
├── models.go                     # Data types (Video, ChannelData, VideoFilter, analysis types)
├── fetch.go                      # YouTube Data API v3 video fetcher
├── oauth.go                      # OAuth2 flow (local callback server, token management)
├── fetch_analytics.go            # YouTube Analytics API v2 queries (aggregate + dimensional)
├── fetch_analytics_merge.go      # Pure-function row mergers for daily/categorical passes
├── snapshot.go                   # Per-fetch snapshots of videos.json
├── analyze.go                    # Analysis logic + terminal report (incl. traffic / UNSUB conversion)
├── dashboard.go                  # HTML dashboard generation (Chart.js)
├── diff.go                       # analyze --diff + --compare-periods (filter-aware)
├── duplicates.go                 # find-duplicates subcommand
├── data_quality.go               # data-quality subcommand
├── titles.go                     # titles subcommand
├── video.go                      # video <id> subcommand (deep-dive with all dimensions)
├── export.go                     # export subcommand
├── suggest_tags.go               # suggest-tags subcommand
├── update_tags.go                # update-tags subcommand
├── cohorts.go                    # Cohort store, auto-assignment rule engine
├── cmd_cohort.go                 # `cohort` subcommand router
├── insights.go                   # Hypothesis ledger, frontmatter parser
├── cmd_insights.go               # `insights` subcommand router
├── cmd_comments.go               # `fetch-comments` (Data API CommentThreads)
├── docs/primitive-test.md        # Decision framework: SDK vs consumer layer
├── skill/yt-analytics/SKILL.md   # Claude skill that drives the closed-loop workflow
├── .githooks/pre-commit          # `go vet` + `go test` gate (enable with `make hooks-install`)
├── Makefile                      # hooks-install, test, vet, check targets
├── go.mod
├── go.sum
├── .env                          # API key and channel handle (not committed)
├── client_secret.json            # OAuth2 credentials (not committed)
└── data/
    ├── cohorts.yaml              # Tracked: cohort definitions + rules
    ├── insights/<date>.md        # Tracked: hypothesis ledger
    ├── comments.json             # Tracked: qualitative signal per video
    ├── videos.json               # Ignored: fetched video + analytics data
    ├── cohort_assignments.json   # Ignored: regenerated by `cohort auto`
    ├── token.json                # Ignored: saved OAuth2 token
    ├── dashboard.html            # Ignored: generated dashboard
    └── snapshots/                # Ignored: auto-saved videos.json snapshots (last 30)
```
