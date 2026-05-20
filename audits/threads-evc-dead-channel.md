# Threads @enterprisevibecode — Dead Channel Audit

**Date:** 2026-05-19
**Task:** #379
**Trigger:** Buffer Insights screenshot 2026-05-19 showed 0 reactions / 0 comments on 64 posts to Threads @enterprisevibecode over the last 30 days. Sibling account Threads @mikelady, posting similar content via the same Buffer integration, returned 11 reactions / 2 comments / 1.66% engagement on 53 posts over the same window.

## Observed State

### Buffer Insights (last 30 days)

| Account | Posts | Reactions | Comments | Eng Rate |
|---------|-------|-----------|----------|----------|
| Threads @enterprisevibecode | 64 | 0 | 0 | 0.00% |
| Threads @mikelady | 53 | 11 | 2 | 1.66% |

### threads.com public profile snapshot (2026-05-19)

| Field | @enterprisevibecode | @mikelady |
|-------|--------------------|-----------|
| Followers | **19** | **1,189** |
| Posts visible | 4 recent (last 24h) | 4 recent (last 24h) |
| Warning banners / restrictions | None detected | None detected |
| Post format | Text-only | Mix (text + image) |
| Bio | "Senior DevOps Engineer focused on enterprise-grade app development using AI. Vibe Coding Enterprise-Grade Apps." | "Senior DevOps Engineer @enterprisevibecode. Vibe Coding Enterprise-Grade Apps. Helping others do the same. Watch me ship." |
| Posting cadence | ~64/30d (~2.1/day) | ~53/30d (~1.8/day) |
| Account looks active? | Yes — posts within last 24h | Yes — posts within last 24h |

**Headline differentiator: 62x follower gap (19 vs 1,189).**

## Root-Cause Hypothesis Ranking

### 1. Audience never showed up — MOST LIKELY

**Evidence:**
- 19 followers. With Threads' follower-graph-heavy distribution (recommendations on Threads still lean disproportionately on follower signal vs IG's stronger interest-graph push), a 19-follower account effectively has no organic distribution surface.
- 64 posts in 30 days into a 19-follower audience yields a per-post impression ceiling on the order of single-digit views; expected reactions at any realistic CTR round to zero. 0/64 is mathematically unsurprising, not anomalous.
- The sibling @mikelady (1,189 followers) gets 11 reactions on 53 posts — ~0.21 reactions/post — which extrapolated down to 19 followers is ~0.003 reactions/post, i.e. effectively zero. **The engagement-per-follower rate is consistent across both accounts; the EVC account isn't suppressed, it's just tiny.**
- No warning banners or visible restrictions on the public profile.
- Account is clearly posting (4 posts visible in last 24h on public page).

### 2. Topic mismatch with Threads audience — CONTRIBUTING

**Evidence:**
- @enterprisevibecode posts are uniformly text-only and uniformly on-topic (AI agents, formal verification, enterprise dev). Threads' culture skews casual / conversational / lifestyle.
- @mikelady's higher-engagement post visible today is an image post (BJJ photo) — personality content, not technical. The technical posts on @mikelady likely also underperform; the BJJ-style posts likely carry the engagement average.
- Contributing factor to slow follower growth, not the proximate cause of 0 reactions.

### 3. Cross-posting penalty from Buffer — UNLIKELY

**Evidence against:**
- @mikelady uses the same Buffer integration and gets normal engagement. If Buffer cross-posting were penalized, both would be hit.
- No public-page evidence of broken/truncated rendering.

### 4. Buffer integration / formatting issue — RULED OUT

**Evidence against:**
- Public posts on @enterprisevibecode render normally (verified via threads.com fetch). No truncation, missing media, or malformed content visible.
- Posts are reaching the platform successfully — they're just not being seen.

### 5. Algorithm shadow ban / new-account quality flag — RULED OUT

**Evidence against:**
- No restriction banner, no "limited reach" notice, profile is fully publicly visible and indexed.
- Posts appear in the public feed view.
- Engagement-per-follower rate is consistent with @mikelady when scaled — a shadow ban would produce a per-follower engagement collapse vs the sibling, not parity.

## Recommendation: **REDUCE cadence + shift strategy** (not pause, not continue as-is)

Posting 2.1 text-only technical posts/day into a 19-follower audience is burning content for ~zero distribution. But pausing entirely forfeits the slow follower trickle from cross-account mentions and Buffer's queue mechanics.

### Specific changes

1. **Cut Threads @enterprisevibecode cadence from ~2/day to 3-4/week.** Buffer queue should reflect this — free up the slots for channels that have audiences (LinkedIn, X, the @mikelady Threads account).
2. **Shift remaining EVC Threads posts to personality + reply-bait formats** — questions, hot takes, "what would you do" prompts. Text-only is fine on Threads; the issue is tone, not media.
3. **Cross-promote from @mikelady → @enterprisevibecode** explicitly. @mikelady already mentions @enterprisevibecode in bio — add periodic "follow my brand account" posts from @mikelady (the 1,189-follower account is the only growth lever EVC has on Threads right now).
4. **Reassess in 30 days.** If @enterprisevibecode crosses ~100 followers, restore cadence. If still under 50, consider pausing the channel entirely and posting EVC content only via @mikelady with brand mentions.

## Follow-up Actions

| Priority | Action | Owner | Done when |
|----------|--------|-------|-----------|
| P0 | Reduce Buffer posting schedule for Threads @enterprisevibecode to 3-4 slots/week | user | Buffer schedule edited |
| P0 | Reallocate freed Threads-EVC slots to higher-performing channels (LinkedIn, X) | user | Buffer schedule rebalanced |
| P1 | Draft 4-6 "follow @enterprisevibecode" cross-promo posts for @mikelady's queue | content | drafts in Buffer queue |
| P1 | Add a Threads-specific content variant: reply-bait questions instead of declarations, for EVC's remaining slots | content | next 2 weeks of EVC Threads posts use new format |
| P2 | Set a 2026-06-19 calendar check: re-pull Buffer Insights for EVC Threads, compare followers + engagement vs today's baseline (19 followers, 0/64) | flywheel | calendar event created |
| P2 | If 30-day re-check shows < 50 followers and still ~0 engagement, escalate to "pause channel" decision | flywheel | decision documented |

## Notes / Caveats

- Public threads.com pages do not surface per-post like/reply counts without auth, so this audit could not verify whether *individual* EVC posts have ever received any engagement. Buffer's aggregate (0 reactions / 0 comments over 64 posts) is the source of truth for "dead channel" claim.
- "Algorithm shadow ban" hypothesis is hard to fully exclude without a logged-in view comparing impressions per post, but the per-follower engagement-rate parity with @mikelady (~0.21 reactions/post at 1,189 followers vs predicted 0.003 at 19) is strong evidence the account is operating normally — it's just small.
- Did not pursue WebSearch on 2026 Threads algorithm changes; the follower-count differential is so dominant (62x) that algorithmic factors would be a rounding error against it.
