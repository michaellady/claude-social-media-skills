# Claude Social Media Skills

Claude Code skills for promoting and syndicating your work on social media.

These skills are designed as a **closed loop** — every post is tagged at compose time so engagement can be attributed back, and the analytics skills auto-generate skill-config recommendations from each week's data. See **[ARCHITECTURE.md](ARCHITECTURE.md)** for the design philosophy and **[PATTERNS.md](PATTERNS.md)** for cross-skill cognition patterns (adversarial review, queue overlap check, CTA convention, etc).

Pure-transport helpers live in [`_shared/`](./_shared/) (Go binaries + shell scripts; no cognition). The decision framework for what belongs in code vs prompts is in [PRIMITIVE-TEST.md](PRIMITIVE-TEST.md).

## Skills

### Compose-and-publish

#### `/promote-newsletter`

Extract verbatim snippets from a [beehiiv](https://beehiiv.com) newsletter post and schedule platform-specific posts via Buffer. Preserves the author's original words — only trims to fit character limits.

```
/promote-newsletter https://www.example.com/p/my-post
/promote-newsletter latest
```

#### `/tease-newsletter`

Sibling to `/promote-newsletter`. Instead of pulling verbatim snippets, writes short original teaser hooks per channel that summarize the article without spoiling the punchline. Same `Comment "newsletter"…` CTA so the same DM automation works. Recommended default for LinkedIn channels (verbatim quotes underperform there).

```
/tease-newsletter https://www.example.com/p/my-post
/tease-newsletter latest
```

#### `/carousel-newsletter`

Promote a beehiiv newsletter as a 10-slide illustrated carousel for Instagram, LinkedIn, Facebook, and Threads. Uses Gemini 2.5 Flash Image with a brand banner as style reference. ~$0.40 per deck, ~15 min wall-clock.

```
/carousel-newsletter https://www.example.com/p/my-post
```

#### `/promote-github`

Fetch your public GitHub contributions (merged PRs, commits, releases, new repos) and compose value/impact-framed social media posts. Defaults to instant-publish.

```
/promote-github today
/promote-github this-week
/promote-github 2026-03-01..2026-03-30
/promote-github https://github.com/user/repo/pull/123
```

#### `/crosspost-newsletter`

Cross-post a beehiiv newsletter article across five platforms in two modes:

- **Full-article syndication** to LinkedIn (native article), Substack, and Medium — preserves rich formatting, headings, and images. Sets canonical URL back to the original post.
- **Link submission** to Hacker News and Reddit — submits the beehiiv URL with the article title. For Reddit, picks one or more subreddits from a configurable default list.

```
/crosspost-newsletter https://www.example.com/p/my-post
/crosspost-newsletter latest
```

### Measure (closed-loop input)

#### `/buffer-stats`

Combine Buffer's MCP (operational data: queue depth, posting goals) with a gstack scrape of Buffer Insights + Analyze (engagement: per-channel followers, impressions, top posts, format-performance). Auto-generates skill-config recommendations from this week's format-performance data.

```
/buffer-stats
/buffer-stats operational    # MCP-only fast path, no browser
/buffer-stats --days 30
```

#### `/linkedin-stats`

Scrape LinkedIn Creator analytics (`/dashboard/`, `/analytics/creator/content`, `/analytics/creator/audience`) for newsletter subs, profile followers, company-page followers, post impressions, and per-post engagement. Caches snapshots for week-over-week deltas.

```
/linkedin-stats
/linkedin-stats newsletter   # newsletter only, fast path
/linkedin-stats --since 2026-04-19
```

### Aggregate + audit (closed-loop hygiene)

#### `/flywheel`

Cross-platform weekly rollup keyed to your 5 growth priorities. Combines `buffer-stats` + `linkedin-stats` + YouTube + beehiiv into one report. Includes per-channel ROI scoring to surface deprioritization candidates.

```
/flywheel
```

#### `/audit-buffer-queue`

Inspect the Buffer queue for health issues that aren't caught by the per-skill scheduling logic — bunching (gap < 3h between posts on the same channel), theme over-saturation, untagged posts that break closed-loop measurement, dead channels, below-threshold channels.

```
/audit-buffer-queue
```

#### `/tune-posting-schedule`

Analyze each Buffer channel's `postingSchedule` (the time slots Buffer drops queued posts into) and propose + apply a better one. Pairs with `/audit-buffer-queue`: that skill cancels/reschedules individual bunched posts; this one fixes the **slots** so bunches stop recurring. Uses gap-spacing rules + (optional) engagement-by-hour data from `/buffer-stats`. Applies via Buffer's GraphQL mutation after explicit per-channel approval.

```
/tune-posting-schedule
/tune-posting-schedule threads-mikelady,facebook-evc
```

## Setup

1. Install [Claude Code](https://claude.ai/code)
2. Connect a [Buffer MCP server](https://publish.buffer.com/settings/api) with your API key
3. Install [gstack](https://github.com/nichochar/gstack) browse (for crosspost, stats, audit skills)
4. Build the Go helpers in `_shared/`:
   ```bash
   cd _shared/buffer-post-prep && go build .
   cd ../buffer-queue-check && go build .
   cd ../voice-corpus && go build .
   ```
5. Symlink each skill directory into `~/.claude/skills/`:
   ```bash
   for skill in promote-newsletter tease-newsletter carousel-newsletter \
                promote-github crosspost-newsletter \
                buffer-stats linkedin-stats flywheel \
                audit-buffer-queue tune-posting-schedule; do
     ln -s /path/to/claude-social-media-skills/$skill ~/.claude/skills/$skill
   done
   ```
6. Use the slash commands from any Claude Code session
