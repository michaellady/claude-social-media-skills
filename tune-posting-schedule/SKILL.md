---
name: tune-posting-schedule
description: Use when posting times are bunching, when /audit-buffer-queue surfaces structural bunches that re-appear after rescheduling individual posts, or when /buffer-stats finds a per-hour engagement pattern that disagrees with current Buffer slots. Analyzes each channel's postingSchedule against (a) gap-spacing rules, (b) recent sent-post engagement-by-hour, and (c) audience timezone, then proposes + applies a new schedule via Buffer's GraphQL mutation. Triggers — "tune my posting schedule", "fix my buffer slots", "analyze posting times", "my queue keeps bunching", "change posting schedule".
user_invocable: true
---

# tune-posting-schedule

Analyze each Buffer channel's `postingSchedule` (the time slots Buffer uses to drop queued posts into) and propose + apply a better schedule. Pairs with `/audit-buffer-queue` (queue hygiene) — this skill fixes the **structural** cause when bunches keep re-appearing after rescheduling individual posts.

## When to Use

Use when:
- `/audit-buffer-queue` flags bunching (gap < 3h) that re-appears after rescheduling — the slots themselves are too tight
- `/buffer-stats` Phase 5 surfaces an engagement-by-hour pattern (e.g. "Threads posts at 9-10am get 3x impressions of 12-1pm posts") that disagrees with current slot allocation
- The user wants to spread out a clustered schedule (e.g. all 3 daily slots in a 3-hour morning window)
- The user wants to drop slot count to better match posting cadence (e.g. 3 slots/day → 2 slots/day on a low-velocity channel)
- After a follower-count milestone where the user wants to test a different posting cadence

Do NOT use for:
- One-off rescheduling of a single post → use `mcp__buffer__update_post` directly (or `/audit-buffer-queue` Phase 8)
- Pausing the queue → use Buffer's web UI (`isQueuePaused` toggle)
- Cancelling posts → use `/audit-buffer-queue` Phase 8

## Process

### Phase 1 — Resolve target channels

If the user names channels, use those. If not, default to **all non-locked, non-disconnected, non-startPage channels** in the org:

```
mcp__buffer__get_account → org ID
mcp__buffer__list_channels → filter where !isDisconnected && !isLocked && service != 'startPage'
```

Confirm the channel list with the user before proceeding (channel changes are visible to followers — checking first is cheap).

### Phase 2 — Pull current schedule per channel

For each target channel:

```
mcp__buffer__get_channel(channelId) → postingSchedule, timezone, postingGoal, isQueuePaused
```

The `postingSchedule` is an array of `{day: "mon"|"tue"|..., times: ["HH:MM", ...], paused: bool}` in the channel's own `timezone`. **All times are in the channel's local timezone, not UTC.**

### Phase 3 — Analyze structural bunches in current schedule

For each day's `times`, compute consecutive gaps. Flag any gap < `min_gap_hours` (default 3h).

```python
# Pseudocode
for day in schedule:
    sorted_times = sorted(day['times'])
    for i in range(1, len(sorted_times)):
        gap_min = (parse(sorted_times[i]) - parse(sorted_times[i-1])).total_seconds() / 60
        if gap_min < min_gap_hours * 60:
            structural_bunches.append((channel, day, sorted_times[i-1], sorted_times[i], gap_min))
```

These are bunches **built into the slots** — rescheduling individual posts won't fix them; the schedule itself needs editing.

### Phase 4 — (Optional) Engagement-by-hour analysis

If the user wants engagement-driven recommendations (vs just gap-fixing), pull recent sent posts and bucket by local-hour:

```
mcp__buffer__list_posts(status: ["sent"], dueAt: { start: 90_days_ago }, channelIds: [...]) 
```

For each sent post, convert `sentAt` → channel's local hour. Group by hour. Compare per-hour densities + (where available) engagement against current slot allocation.

**Limits:**
- Buffer's `list_posts` doesn't return per-post engagement directly — for that, the user must have run `/buffer-stats` recently and have the cached engagement data on disk. Check for it; if absent, skip engagement analysis and use gap-only.
- 90-day window is a default; widen for low-velocity channels (< 1 post/day), narrow for high-velocity ones.

**Cognition (skill judgment, not transport):**
- Which hours qualify as "audience-active" vs "engagement-dead" → judgment call (Bitter Lesson — smarter model decides better; stays in this prompt, not in a Go helper)
- How aggressively to consolidate slots (drop count, redistribute) → judgment call
- Whether a low-engagement hour is a true low or just under-sampled → judgment call

### Phase 5 — Propose new schedule per channel

Build a proposed `postingSchedule` per channel that:
- Has all gaps ≥ `min_gap_hours` (default 3h)
- Preserves the original slot **count** per day unless the user opted to change it
- Stays within the channel's `audience-active window` (default: 06:00–22:00 local; configurable)
- (If engagement data available) overweights audience-active hours, drops engagement-dead hours

**Default redistribution patterns** (3 slots/day):
- **Spread** (recommended): morning + midday + evening, e.g. `09:30 / 13:30 / 18:30`
- **AM-PM-evening cluster**: `09:00 / 14:00 / 19:00`
- **Evening-heavy** (LinkedIn personal pattern observed): `08:00 / 17:00 / 20:30`

**Default for 2 slots/day:** `09:30 / 18:30` (morning + evening).

Annotate each change with the **why**:
- "moved 12:24 → 18:30 because Mon/Tue/Wed/Thu/Fri all bunched 3 slots inside 09:30-12:30 window"
- "dropped Wed 09:40 because gap with 08:56 was 44 min"

### Phase 6 — Adversarial review (REQUIRED before user review)

Apply the **[Adversarial Review pattern](../PATTERNS.md#pattern-adversarial-review)** with these per-skill specifics:

- **SOURCE_LABEL:** "CURRENT POSTING SCHEDULES + USER'S STATED GOALS"
- **SOURCE_CONTENT:** for each channel — current `postingSchedule`, channel timezone, `postingGoal`, plus whatever the user explicitly asked for (e.g. "spread out morning bunches", "drop to 2 slots/day on Facebook")
- **SKILL_NAME:** `tune-posting-schedule`
- **ARTIFACT_NAME:** "schedule"
- **RULES_LIST:**
  - Every gap between consecutive same-day slots MUST be ≥ `min_gap_hours` (default 3 hours).
  - Slot count per day MUST equal the original count UNLESS the user explicitly asked to change it.
  - All slot times MUST stay within the channel's audience-active window (default 06:00–22:00 local) UNLESS the user explicitly asked otherwise.
  - REQUIRED: every change has a stated **why** that points to a structural bunch, an engagement signal, or an explicit user request.
  - BANNED: silently dropping or adding slots without a cited rationale.
  - BANNED: moving a slot into a hour the channel has zero historical sent posts in (probable engagement-dead) UNLESS the user asked to test that hour.
- **ISSUE_GUIDANCE:** "For gap violations, cite the day + the two times + the computed gap. For slot-count drift, cite original vs proposed. For unjustified changes, quote the change and note that no rationale was given."

### Phase 7 — User review

Present a per-channel diff:

```
Channel: Threads (mikelady) [America/Los_Angeles, 3 slots/day, goal 21/wk]

  Mon  10:00 11:28 12:24      →  09:30 13:30 18:30
       (3 slots in 2.4hr)         (spread morning/afternoon/evening)
  Tue  10:12 11:14 12:42      →  09:30 13:30 18:30
       (3 slots in 2.5hr)         (spread)
  ...

  Why: every weekday had 3 slots inside a 2-3 hr morning window.
       Bunches in last queue: 88 min, 56 min on Mon-Fri.
```

Ask: "Apply these N schedule changes? Or pick which channels?"

User can:
- Approve all
- Approve subset (per-channel)
- Tweak proposed times
- Cancel

### Phase 8 — Apply

**Buffer's public GraphQL API does NOT expose a mutation for editing `postingSchedule`.** As of 2026-04-27, the schema only exposes `deletePost`, `createPost`, `editPost`, `createIdea`. The `postingSchedule` field on `Channel` is read-only via the API; schedule edits must be made in Buffer's web UI.

Always re-verify by calling `mcp__buffer__introspect_schema` first — Buffer may add a `updatePostingSchedule` (or similar) mutation in the future. If found, use it; if not, fall through to the manual path below.

**Manual path (current default):**

For each approved channel, surface a copy-paste-ready checklist:

```markdown
## <Channel Name> — Buffer web UI steps

1. Open https://publish.buffer.com/channels/<channelId>/settings (or Channel → Settings → Posting Schedule)
2. For each day below, replace the existing times with the proposed times:
   - Mon: 09:30, 13:30, 18:30
   - Tue: 09:30, 13:30, 18:30
   - ...
3. Save.
```

After the user confirms a channel was updated in the web UI, immediately call `mcp__buffer__get_channel(channelId)` and verify `postingSchedule` matches the proposed one. If it doesn't, surface the diff.

**If a future Buffer schema exposes the mutation:**

```
mcp__buffer__execute_mutation(
  summary: "Update <channel name> posting schedule to spread bunched morning slots",
  mutation: <the canonical mutation, e.g. updatePostingSchedule>,
  variables: { channelId: ..., schedule: [...proposed...] }
)
```

Apply per-channel, not in one batch. After each successful mutation, re-call `get_channel` and verify.

### Phase 9 — Report

Render a summary:

```markdown
# Posting schedule update (YYYY-MM-DD)

| Channel | Original slots/wk | New slots/wk | Bunches fixed | Status |
|---|---:|---:|---:|---|
| Threads (mikelady) | 21 | 21 | 7 (Mon-Fri morning bunches) | ✅ applied |
| Threads (EVC) | 21 | 21 | 8 | ✅ applied |
| LinkedIn EVC page | 21 | 21 | 3 (Mon/Sun 21:xx, Thu 13/14:00) | ⚠️  user declined |
```

## Closed-loop integration

This skill is the **structural** half of queue-hygiene; `/audit-buffer-queue` is the **per-post** half:

- `/audit-buffer-queue` → cancels/reschedules individual bunched posts (one-time fix)
- `/tune-posting-schedule` → fixes the slots themselves (permanent fix)

Run `/tune-posting-schedule` after `/audit-buffer-queue` flags bunches that recur week-over-week. If `/buffer-stats` Phase 5 starts including per-hour engagement (future work), this skill should consume that data automatically in Phase 4.

## Defaults baked in

- `min_gap_hours = 3` — minimum spacing between consecutive same-day slots. Conservative for LinkedIn/Facebook, generous for Threads (which tolerates higher cadence). User can override per-channel.
- `audience_active_window = 06:00–22:00` channel-local. Slot moves outside this window require explicit user request.
- `lookback_window_days = 90` for engagement-by-hour analysis. Widen for low-velocity channels.
- **Don't change slot counts** unless the user explicitly asks. Slot count = posting cadence = a content-strategy decision, not a hygiene decision.

## Common Mistakes

- **Auto-applying mutations without user review.** Schedule changes are visible to followers (posts shift to new times) — always show the diff and get approval.
- **Mixing slot-count changes with spread fixes in one proposal.** Keep them separate so the user can approve hygiene fixes (spread) without committing to a strategy change (cadence drop).
- **Skipping introspect.** Buffer renames GraphQL mutations periodically — guessing the mutation name leads to silent failures or wrong-input errors. Always introspect first.
- **Confusing channel-local time with UTC.** `postingSchedule.times` are in the channel's `timezone`, not UTC. Convert correctly when comparing against `sentAt` (which is UTC).
- **Treating low-engagement hours as engagement-dead without checking sample size.** A 9pm slot with 3 posts in 90 days isn't a "dead hour" — it's an undersampled hour. Flag to user, don't silently drop.

## Rationale: why this lives in the skill (cognition) vs a Go helper (transport)

Per [PRIMITIVE-TEST.md](../PRIMITIVE-TEST.md):

- **Atomicity:** schedule edits are single-channel single-mutation; Buffer's GraphQL is the source of truth — no race. ✅ no helper needed.
- **Bitter Lesson:** "which hours are audience-active" is exactly the kind of judgment a smarter model does better. ✅ stays in prompt.
- **ZFC:** the analysis IS judgment (`if morning bunched then redistribute to afternoon/evening`). ✅ stays in prompt.

The deterministic transport pieces — `mcp__buffer__list_channels`, `get_channel`, `introspect_schema`, `execute_mutation` — are already provided by Buffer's MCP server. No new `_shared/` Go binary is required for this skill.
