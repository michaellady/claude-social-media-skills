---
name: audit-buffer-queue
description: Use when user wants a health check on the Buffer queue — bunching, dead channels, theme over-saturation, untagged posts that break closed-loop measurement. Triggers — "audit my buffer queue", "buffer queue health", "is my buffer queue too crowded", "check my queued posts for problems".
user_invocable: true
---

# audit-buffer-queue

Inspect the Buffer queue for health issues that aren't caught by the per-skill scheduling logic. The promote-* skills each see only their own batch; this skill sees the whole queue.

## When to Use

Use when:
- User asks "audit my buffer queue", "check my queued posts", "is my queue too crowded"
- After a heavy promotion run (e.g. `/promote-newsletter` + `/tease-newsletter` + `/carousel-newsletter` + `/crosspost-newsletter` on the same article = ~22 surfaces in one session). Worth a queue-health check.
- Weekly, as part of the closed-loop review cycle (companion to `/buffer-stats` and `/flywheel`).
- When `/buffer-stats` or `/flywheel` flags `untagged_posts > 0` or `bunched_posts > 0`.

Do NOT use for:
- Engagement analysis → use `/buffer-stats`
- Per-platform follower deltas → use `/linkedin-stats`
- Cross-platform weekly rollup → use `/flywheel`

## Process

### Phase 1 — Pull the full queue

```
mcp__buffer__get_account → org ID
mcp__buffer__list_posts (status: ["scheduled", "needs_approval", "draft"], first: 100, sort: dueAt asc)
```

If the result exceeds the tool-result size limit, the response is auto-saved to a file. Use `jq` to extract `{id, dueAt, channelId, channelService, text, tags}` for each post — you don't need the full payload.

### Phase 2 — Per-channel bunching check

For each channel, compute the time gap between consecutive posts. **Default minimum gap: 3 hours.** Anything shorter is "bunched."

```python
# Pseudocode
posts_by_channel = group_by(posts, key=lambda p: p['channelId'])
bunched = []
for channel_id, channel_posts in posts_by_channel.items():
    sorted_posts = sorted(channel_posts, key=lambda p: p['dueAt'])
    for i in range(1, len(sorted_posts)):
        gap = sorted_posts[i]['dueAt'] - sorted_posts[i-1]['dueAt']
        if gap < timedelta(hours=3):
            bunched.append({
                'channel': channel_id,
                'gap_minutes': gap.total_seconds() / 60,
                'post_a': sorted_posts[i-1],
                'post_b': sorted_posts[i],
            })
```

Surface bunched pairs with a recommendation: cancel one, OR reschedule one to a different time slot.

### Phase 3 — Theme over-saturation check

Posts about the same article/topic going out too close together = audience fatigue.

**Transport (`_shared/buffer-queue-check`):** the deterministic substring matching + per-keyword grouping is delegated to the [Buffer queue check pattern](../PATTERNS.md#pattern-buffer-queue--recently-sent-overlap-check). Pipe the `mcp__buffer__list_posts` JSON into the helper with the candidate distinctive phrases:

```bash
echo "$LIST_POSTS_JSON" | _shared/buffer-queue-check/buffer-queue-check \
  --keywords "Tokens From Our Past,Mac Pro,paperweight,Trash Can"
```

**Cognition (skill):** the DECISIONS of which phrases count as "distinctive" + how many matches per channel within what time window count as "over-saturation" stay here.

Defaults:
- Distinctive phrase = 4-8 word substring that's unique to one article/topic (skip common words like "shipped", "skill", "AI")
- 3+ matches on the same channel within 5 days = theme over-saturation flag
- 5+ matches on the same channel for the same article title = aggressive over-saturation; recommend cancelling 2+

The helper output makes this deterministic; the thresholds above are tunable judgment.

### Phase 4 — Untagged post check + auto-backfill

Every post created by the promote-* skills should have a `format:<name>` tag (`format:verbatim-quote`, `format:teaser`, `format:carousel`, `format:link-share`, `format:batch-summary` — `format:long-form-pulse` is future-reserved and not produced by any current skill, so don't flag its absence). Posts without one of those tags break the closed-loop measurement system — `buffer-stats`'s Phase 5 format-performance analyzer can't attribute them.

```python
EXPECTED = {'format:verbatim-quote', 'format:teaser', 'format:carousel', 'format:link-share', 'format:batch-summary'}
untagged = [p for p in posts if not any(t['name'] in EXPECTED for t in p['tags'])]
```

For each untagged post, classify by text content:
- starts with `"<quote>"` and ends with `Comment "newsletter"…` CTA → `format:verbatim-quote`
- original copy + `Comment "newsletter"…` CTA → `format:teaser`
- 10-image multi-image post → `format:carousel`
- contains `github.com/` URL → `format:link-share` (or `format:batch-summary` if the post unifies multiple contributions with a theme sentence)

**Auto-backfill (since 2026-04-27, Stage A complete):** the 5 format tags exist on the org and their IDs are in `_shared/buffer-post-prep/tag-ids.local.json`. For each untagged post, you CAN auto-apply the right tag via the Buffer GraphQL `editPost` mutation — but with two important caveats:

1. **`editPost` is full-replace, NOT partial-update.** If you only pass `id + tagIds`, the post's text + assets + metadata get wiped. Always fetch the full post first via `mcp__buffer__get_post` and pass `text + assets + metadata + tagIds` together. See `/tmp/queue_audit/backfill_tags.sh` for a working reference implementation (the curl-based script used for the original 53-post backfill).
2. **Buffer enforces a 15-min sliding rate-limit window** on the OIDC token (~50-100 calls per 15 min before HTTP 429). For a backfill of more than ~25 posts, expect to hit it; build a poll-retry loop with 180s sleep between attempts (also at `/tmp/queue_audit/poll_retry.sh`).

If the user prefers manual control, surface the recommendations and let them apply individually — `mcp__buffer__edit_post` is not exposed as a domain tool, so they'd use Buffer's web UI tag picker.

### Phase 5 — Dead-channel check

A channel is "dead" if it's connected, not paused, but has had 0 sent posts in the last 14 days AND 0 scheduled posts in the queue.

```python
mcp__buffer__list_channels → all connected channels
mcp__buffer__list_posts (status: ["sent"], dueAt: { start: 14_days_ago })
mcp__buffer__list_posts (status: ["scheduled"])
# Cross-reference to find channels with 0 of both.
```

Flag dead channels — recommend either deleting them from Buffer or reactivating with a posting goal.

### Phase 6 — Below-threshold-channel check

For each channel in the queue, compare its follower count (via `mcp__buffer__get_channel`) against the `min_followers_to_promote` threshold (default 50, configurable in promote-* skills). If a channel below threshold has queued posts, those were probably scheduled before the threshold was added — recommend cancelling them.

### Phase 7 — Render report

```markdown
# Buffer Queue Audit (YYYY-MM-DD)

**Total queued posts:** N · **Channels with queue:** M · **Window:** next K days

## 🔴 Bunched posts (gap < 3 hours)

| Channel | Gap | Post A | Post B | Recommendation |
|---|---|---|---|---|
| Threads (mikelady) | 38 min | "I had to re-skill..." | "We're not going to revert..." | Cancel one |

## 🟡 Theme over-saturation

| Channel | Article / theme | Count | Recommendation |
|---|---|---:|---|
| Facebook (EVC) | Tokens From Our Past | 6 | Cancel 3 — past saturation point |

## ⚪ Untagged posts

| Channel | Post (snippet) | Suggested tag |
|---|---|---|
| LinkedIn personal | "I had to re-skill..." | format:verbatim-quote |

## 🔴 Dead channels (no sent in 14d, no queued)

| Channel | Service | Last sent |
|---|---|---|
| Mastodon (mikelady) | mastodon | 2026-03-14 |

## 🔴 Below-threshold channels with queued posts

| Channel | Followers | Queued | Recommendation |
|---|---:|---:|---|
| LinkedIn page (EVC) | 28 | 4 | Cancel — below 50-follower threshold |
```

### Phase 8 — User action

For each flagged item, present a 1-click action:
- **Cancel** the offending post → `mcp__buffer__delete_post`
- **Reschedule** with a new dueAt → `mcp__buffer__update_post` with `mode: "customScheduled"`
- **Tag** an untagged post → `mcp__buffer__update_post` adding the suggested `format:<name>` tag

Default to surfacing the recommendations and asking for batch approval ("apply all 12 recommendations? Or pick which ones?") rather than acting silently.

## Common Mistakes

- **Auto-deleting without user approval.** This skill is advisory — it surfaces problems and proposes fixes. Destructive actions need the user's OK.
- **Confusing bunching with intentional batching.** Some channels (e.g. Threads) tolerate bunches better than others (FB, LinkedIn). The 3-hour default is conservative for LinkedIn/FB; the user can override per-channel.
- **Missing the queue context.** A "bunched" pair might be a deliberate teaser-then-followup. If post A is a verbatim quote and post B is a follow-up question, that's intentional. The format tags help disambiguate — if both are `format:verbatim-quote`, it's accidental fan-out duplication.

## Closed-loop integration

This skill is the "queue hygiene" half of the post-and-improve closed loop:
- `promote-newsletter` / `tease-newsletter` / etc. CREATE posts with format tags
- `audit-buffer-queue` checks the queue for problems (this skill)
- `buffer-stats` MEASURES engagement per format tag (Phase 5 analyzer)
- `buffer-stats` Phase 5b RECOMMENDS skill-config changes
- User accepts recommendations → SKILL.md gets updated → next batch of posts is better-targeted

Run `/audit-buffer-queue` weekly between `/buffer-stats` runs to catch queue problems before they ship.
