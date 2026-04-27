# Architecture

The skills in this repo are designed as a **closed loop**, not as independent one-shot tools. Every post the compose skills create is tagged at compose time so the analytics skills can later attribute engagement back to the format that produced it — and use that attribution to recommend skill-config changes for the next promotion cycle.

## The loop

```
Compose with format tag (promote-* skills, Phase 6)
          ↓
Adversarial review (fresh agent vs source + skill rules)
          ↓
User review + publish (Phase 5 + 6)
          ↓
Audit queue health (audit-buffer-queue, weekly)
          ↓
Measure engagement per format (buffer-stats Phase 5)
          ↓
Recommend skill changes (buffer-stats Phase 5b)
          ↓
User accepts → SKILL.md edits committed → next batch better-targeted
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
- **Phase 6 — Buffer create_post** with `tags: ["format:<name>"]` (attribution prereq)

### Measure (read side)

| Skill | Surface scraped | Output |
|---|---|---|
| [`buffer-stats`](buffer-stats/SKILL.md) | publish.buffer.com/insights (cross-channel) + analyze.buffer.com (per-channel) | Per-(channel, format) engagement table + auto-generated skill recommendations |
| [`linkedin-stats`](linkedin-stats/SKILL.md) | linkedin.com/dashboard/ + /analytics/creator/* | Followers + impressions + per-post engagement deltas vs cached snapshot |
| [`flywheel`](flywheel/SKILL.md) | Aggregates buffer-stats + linkedin-stats + YouTube + beehiiv into one weekly rollup keyed to growth priorities | Weekly priorities-keyed report with channel ROI scores |

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

## Defaults baked in 2026-04-27

These were derived from the LinkedIn /dashboard/ + Buffer Insights data collected during one heavy promotion week. They're the loop's *current state* — not eternal truths. The expectation is they shift as more data flows through the system.

| Default | Value | Reason (data citation) |
|---|---|---|
| `max_posts_per_channel_per_article` | 3 | Buffer Insights: reactions ↓52% M-o-M while posts ↑24.5% — fan-out past ~3/channel/article fatigues audiences |
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
2. Tag every Buffer post the skill creates with `tags: ["format:<name>"]` at `mcp__buffer__create_post` time
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
