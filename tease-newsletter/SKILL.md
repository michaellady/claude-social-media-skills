---
name: tease-newsletter
description: Use when user wants to promote a beehiiv newsletter on social media with teaser-style hooks (rather than verbatim quotes) that summarize the article without giving away the punchline — "tease this newsletter", "write teasers for the newsletter", "summarize and promote", "hook posts for the article". Each post ends with the standard "Comment newsletter…" CTA so commenters get the article DM'd to them.
user_invocable: true
---

# tease-newsletter

Sibling to `promote-newsletter`. Instead of pulling verbatim snippets from the article, this skill writes short, original teaser hooks per channel that summarize the piece in a curiosity-gap style and end with the standard CTA.

The CTA is identical to `promote-newsletter` so the same Manychat / DM automation works:

```
Comment "newsletter" to get my latest post, "<Article Title>"
```

## When to Use

Use when:
- User says "tease this newsletter", "write teasers for...", "promote with hooks", "summarize and promote", "do tease-newsletter on..."
- The article has been promoted already with verbatim snippets and the user wants a second wave with a different angle
- The article's strongest snippets are too long, too inside-baseball, or too punchline-heavy to quote raw

Do NOT use when:
- User wants verbatim quotes → use `promote-newsletter`
- User wants full-article syndication on LinkedIn/Substack/Medium/HN/Reddit → use `crosspost-newsletter`
- User wants illustrated carousels → use the carousel-newsletter skill (if present)

## Process

### Phase 1 — Fetch Newsletter Content

Same as `promote-newsletter` Phase 1:
- If URL provided: `WebFetch` the beehiiv post, extract title, subtitle, body paragraphs, image URLs.
- If "latest" or no URL: fetch the beehiiv RSS feed (URL stored in memory `reference_beehiiv_feed.md`), list recent articles, ask the user which one.

### Phase 2 — Identify the Hook Material (no quoting yet)

Read the whole article and identify:
- **The single strongest insight or claim** (the "punchline" — this gets withheld in teasers)
- **3–5 angles** the article approaches the topic from (each angle becomes a candidate teaser)
- **The emotional/intellectual payoff** the reader gets at the end (also withheld)
- **Any concrete numbers, names, or vivid metaphors** worth name-dropping in the tease

Present the article's structure to the user as a brief outline:
```
Article: "<Title>"
Punchline withheld: <one-line summary of the takeaway>
Angles available:
  A) <angle> — could tease as "..."
  B) <angle> — could tease as "..."
  C) <angle> — could tease as "..."
  D) <angle> — could tease as "..."
```

Also: **check the Buffer queue for existing posts from this article** (same procedure as `promote-newsletter` Phase 2). Annotate each angle with `✅ new` or `⚠️ overlaps existing queued post about <subject>` so the user can avoid duplicate angles.

### Phase 3 — User Picks ONE Angle (then we adapt to length)

**The workflow is: present angles → user picks ONE → fit that single message to each platform's length.** The user's job is to choose the angle. Your job is to length-adapt — never to angle-vary.

Concretely:

1. **Present** the 3–5 angles from Phase 2 to the user with a one-line preview tease for each. Use AskUserQuestion (single-select, NOT multi-select) so the UI forces a single pick.
2. **Wait** for the user to select exactly one angle.
3. **Also ask** in the same prompt:
   - Which image attaches where (default: hero image on the highest-priority channel — see priority order below)
   - Tease style preference: **curiosity-gap** ("Here's what I learned about X — and why it surprised me"), **provocation** ("Most people get X wrong. Here's why."), or **personal-story** ("I bought a $200 paperweight last week.")
4. **In Phase 4**, write that one chosen angle as the canonical message at the longest applicable length (LinkedIn/Instagram). Then for shorter platforms, derive a length-trimmed version of the SAME message — same hook, same setup, same withheld punchline — just compressed. Don't introduce new content, new angles, or new framings in the shorter versions.

**Banned in this phase:** asking the user "would you like different angles for different platforms?" That escape hatch is the source of the per-channel-variation antipattern. Only honor that request if the user volunteers it explicitly and unprompted.

**Unattended-mode fallback:** If running unattended (e.g. `/loop`, autonomous run, or no human reviewer): pick the strongest single angle from Phase 2 and use the **curiosity-gap** style as default. Apply that one message to all channels with length adaptation only — do NOT auto-vary angles per channel. Proceed without waiting.

### Phase 4 — Compose Teaser Posts

**CRITICAL RULES:**
1. **No contiguous run of 7+ words may be copied directly from the article.** This is the verbatim rule, applied mechanically: take any 7-word substring from your draft and grep the article for it — must return zero matches. Teasers are original copy that summarize and intrigue.
2. **Do NOT spoil the punchline.** The reader's payoff for clicking through (or commenting "newsletter") is the actual insight. If the tease gives it away, the CTA fails.
3. **Stay faithful to the article.** Don't invent claims the article doesn't make. Don't promise insights the article doesn't deliver. Hooks summarize, they don't fabricate. **Specifically banned:** unverifiable third-party claims like "every leader I respect does X", "everyone in [industry] knows Y", "successful founders all keep Z on their desk". These are appeals-to-authority that the article does not back up — they sound generic and they are factually unverifiable. Stick to first-person observations the writer can stand behind.
4. **No emoji unless the user requests it.** Match the sibling `promote-newsletter` convention.
5. **End every post with the exact CTA**, blank line above:
   ```
   Comment "newsletter" to get my latest post, "<Article Title>"
   ```

**Character budgets** (CTA is ~70–95 chars depending on title; calculate per article):

> **Definition:** "Total post budget" is the **whole-post character count, including the tease body, the blank line, and the CTA**. The 7-char margin is safety against off-by-one issues with the platform's hard limit. Stay under the budget — not just under the hard limit.

| Platform   | Hard limit | Total post budget               |
|------------|-----------:|---------------------------------|
| Twitter/X  | 280        | 280 - CTA length - 7 margin     |
| Bluesky    | 300        | 300 - CTA length - 7 margin     |
| Pinterest  | 300        | 300 - CTA length - 7 margin     |
| Threads    | 500        | 500 - CTA length - 7 margin     |
| Facebook   | 500        | 500 - CTA length - 7 margin     |
| Mastodon   | 500        | 500 - CTA length - 7 margin     |
| Instagram  | 2,200      | 2,200 - CTA length - 7 margin   |
| TikTok     | 2,200      | 2,200 - CTA length - 7 margin   |
| LinkedIn   | 3,000      | 3,000 - CTA length - 7 margin   |

**Recommended tease shapes by platform:**
- **Threads / Twitter / Bluesky:** 1–3 short lines. Lead with the hook, don't bury it. Pattern: `<curiosity statement>. <stake / why-care>. <implicit promise of payoff>.`
- **Facebook:** 2–4 lines. Conversational. Allow a personal frame ("Spent $200 on a paperweight this week...").
- **LinkedIn:** 3–6 short paragraphs (each 1–3 lines). Open with a hook, set up the topic, hint at the takeaway, withhold the conclusion. End with the CTA.
- **Instagram:** Long-form is welcome (caption can be 3–8 short paragraphs). The hero image carries the visual; the caption tells the story up to but not past the punchline.

**Media attachment:**
Same Buffer dedupe rules as `promote-newsletter` — each unique image URL can only be used on one post in the queue. Ask the user how to distribute available images across the channels.

**Default hero-image priority order** (when the user doesn't specify): LinkedIn (page) → LinkedIn (personal) → Instagram → Facebook → Threads. The hero is the article's strongest visual; it goes to the highest-priority channel with image capacity. Instagram is forced into the list because it requires an image — if no image is available for IG, skip the IG post entirely.

Skip Instagram if no image is available for it. Skip TikTok / YouTube channels (video required).

### Phase 5 — Review Before Publishing

Present all drafted teasers for approval:

```
Channel: @handle (LinkedIn page)
Post (412/3000 chars):
---
<teaser body>

Comment "newsletter" to get my latest post, "<Article Title>"
---
Image: <URL or "text-only">
```

Repeat per channel. Highlight any line that the user might consider "too close to a verbatim quote" or "too revealing of the punchline" — let them edit before scheduling.

Ask: **"Ready to schedule these to Buffer?"**

### Phase 6 — Schedule to Buffer

Identical to `promote-newsletter` Phase 6:
1. `mcp__buffer__get_account` → org ID + timezone
2. `mcp__buffer__list_channels` → exact channel IDs (filter out `isDisconnected`, `isLocked`, `service: "startPage"`)
3. Per approved post: `mcp__buffer__create_post` with `mode: "addToQueue"`, `schedulingType: "automatic"`, platform-specific metadata (`facebook.type: "post"`, `instagram.type: "post" + shouldShareToFeed: true`, etc.)
4. On HTTP 429 rate limit: stop, save remaining posts to `remaining-posts.md`, report.
5. Report per-channel success/error.

## Tease-Writing Checklist (apply to every draft before showing the user)

- [ ] Opens with a hook. **First sentence ≤ 12 words preferred.** Never opens with: "In this post", "Today I want to talk about", "I've been thinking about", "Here's what I learned", "Just shipped", or any other warmup formula.
- [ ] Names a concrete object, number, or vivid noun from the article (specificity = credibility).
- [ ] Sets up a question or tension the reader wants resolved.
- [ ] Does NOT answer that question.
- [ ] **No contiguous run of 7+ words is copied directly from the article.** (Check by sliding a 7-word window across the draft and grepping the source.)
- [ ] No emoji (unless the user requested them).
- [ ] Ends with the exact CTA and the article title in quotes.
- [ ] **Whole-post character count** (tease body + blank line + CTA) is within the platform's total post budget from the table above.
- [ ] **No unverifiable third-party claims** ("every leader I respect…", "everyone in [industry] knows…", "successful founders all do X"). First-person only, or claims the article itself supports.
- [ ] **Same core message as the other channels' posts** (length-adapted, not angle-varied) — unless the user explicitly asked for per-channel variations.

## Common Mistakes

- **Per-channel angle variations by default.** The temptation to "use the long-form room on LinkedIn for a different angle than Threads" produces a feed where each channel sounds like a different person promoting a different article. **One message, length-adapted, is the right default.** Only vary angles when the user explicitly asks for it.
- **Unverifiable third-party claims.** Lines like "every leader I respect keeps a token on their desk" sound generic and aren't backed by the article. They're appeals-to-authority that won't survive a reader thinking "no they don't." Stick to first-person observations the writer can stand behind.
- **Spoiling the punchline.** The reader has no reason to click/comment if the post already delivered the insight. Cut the conclusion sentence.
- **Vague teases ("interesting thoughts on AI").** Specificity drives clicks. Name the object, the metaphor, the concrete event.
- **Verbatim drift.** Easy to slip into quoting when paraphrasing. Apply the checklist's "no contiguous run of 7+ words copied" rule mechanically — slide a 7-word window across your draft and grep the article.
- **Confusing the budget with the hard limit.** A 459-char Facebook post is under the 500 hard limit but over a 402-char total budget. Compute the budget; don't eyeball the hard limit.
- **Forgetting the queue check.** A teaser angle that overlaps a queued snippet from the same article doubles up the feed. Use the same Buffer queue check pattern as `promote-newsletter`.
- **One-size-fits-all length.** A LinkedIn-length tease pasted onto Threads reads like a wall of text; a Threads-length tease on LinkedIn reads thin. Length-match per platform — *length only*, not angle.

## Worked Example

**Input:** `/tease-newsletter https://www.enterprisevibecode.com/p/tokens-from-our-past-and-the-great-re-why-ing`

**Phase 2 outline:**
```
Article: "Tokens From Our Past and The Great Re-Why-ing"
Punchline withheld: Disruption-induced identity loss becomes fuel when you re-derive
  your "why" around enabling others rather than personal mastery — and that same
  re-why-ing is what AI-era developers need now.
Angles available:
  A) Career-pivot artifact (the $200 Mac Pro on the desk)
  B) Cattle-not-pets / Xserve history (last forced reskill)
  C) Jiu-jitsu retirement parallel (12 years, brown belt, 2025)
  D) "Single nine" of Claude reliability (March 2026, ~90% uptime)
```

**Phase 4 — Threads (EVC), angle D, provocation style, 290 chars:**
```
Claude posted a single nine of reliability in March 2026. That is roughly 90% uptime.

The wrong reaction is to retreat to the old workflow. The right reaction has a name, and it is not "wait it out."

Comment "newsletter" to get my latest post, "Tokens From Our Past and The Great Re-Why-ing"
```

**Checklist annotation for that post:**
- ✅ Hook ≤ 12 words: "Claude posted a single nine of reliability in March 2026." (10 words)
- ✅ Concrete: "single nine", "March 2026", "90%"
- ✅ Tension: what is the right reaction? Named ("has a name") but not described.
- ✅ Withholds: the actual right reaction is the article's payoff.
- ✅ No 7-word verbatim run from source.
- ✅ No emoji.
- ✅ CTA exact, with title in quotes.
- ✅ 290 / 402 budget (Threads 500 - 91 CTA - 7 margin = 402). Within budget.

## References

- Sibling skill (verbatim version): `/Users/mikelady/dev/claude-social-media-skills/promote-newsletter/SKILL.md`
- CTA convention, Buffer dedupe rule, channel filtering: see `promote-newsletter` Phase 4 + Phase 6
- Memory: `project_channels.md` (no X), `reference_beehiiv_feed.md` (RSS feed URL)
