---
name: promote-newsletter
description: Use when user wants to promote a beehiiv newsletter post on social media, create social posts from a newsletter, or schedule newsletter promotions to Buffer channels — "promote this newsletter", "share newsletter on social", "create posts for this article".
user_invocable: true
---

# promote-newsletter

Extract the strongest snippets from a beehiiv newsletter post and schedule each selected snippet to every eligible Buffer channel. Never rewrite content — only trim to fit each channel's char budget.

## Usage

`/promote-newsletter <beehiiv-post-url>` or `/promote-newsletter latest`

## Process

### Phase 1 — Fetch Newsletter Content

**If URL provided:**
Use `WebFetch` with the beehiiv post URL. Extract:
- Article title and subtitle
- All body paragraphs (verbatim text, preserve exact wording)
- All image URLs (hero image, inline images, any visuals in the article)

**If "latest" or no URL:**
Fetch the beehiiv RSS feed via `WebFetch` (URL of the form `https://rss.beehiiv.com/feeds/<feed-id>.xml` — the feed ID is specific to the user's publication and will be provided in their settings or first message).
List recent articles with titles and dates. Ask the user which one to promote, then fetch that article's URL.

### Phase 2 — Identify Candidate Snippets & Media

Select **3-5 strongest snippets** from the article. Criteria:
- Contains a concrete insight, statistic, or provocative claim
- Stands alone without needing surrounding context
- Quotable and compelling for social media

Present each snippet as a numbered list:
```
1. [key stat] "73% of developers now use AI daily..." (142 chars)
2. [bold claim] "The real shift isn't automation..." (203 chars)
3. [insight] "What most people miss about..." (178 chars)
```

Also present extracted images with URLs. Recommend the hero/header image as the default media attachment.

**Before presenting the list, check the Buffer queue for existing posts from this article.**

Users often kick off `/promote-newsletter` when earlier promotion runs (crosspost-newsletter, carousel-newsletter, a previous invocation of this skill) have already seeded the queue with related posts. Flag overlapping snippets so the user doesn't unknowingly re-promote the same lines.

1. Call `mcp__buffer__get_account` for the organization ID. If Buffer MCP is unreachable, skip this check and warn the user at the top of the snippet list: *"Buffer queue check skipped — unable to reach Buffer MCP. Existing queue may overlap."* Then proceed without annotations.
2. Call `mcp__buffer__list_posts` with:
   - `organizationId`: from step 1
   - `status`: `["scheduled", "needs_approval", "draft"]`
   - `first: 100`
   - `sort: [{field: "dueAt", direction: "asc"}]`
3. If the response exceeds the tool-result size limit (saves to a file automatically), use `jq` to extract `{dueAt, channelService, text}` per post. You're scanning post text, not rendering the full payload.
4. For each candidate snippet (and the article title), match against queued post text (case-insensitive substring) using:
   - A **distinctive 4-8 word phrase** from the snippet (e.g., `"personal SaaS-apocalypse"`, `"blender of what has existed"`) — not a common word.
   - The **article title** or a distinctive title phrase.
   - Be specific — don't match on common words like "AI" or "shipped" alone.
5. Annotate each snippet in the Phase 2 presentation with a status tag:
   - `✅ new` — zero matching queued posts
   - `⚠️ queued Nx (earliest YYYY-MM-DD)` — N matching posts; show the soonest `dueAt`
   Mention the dup count in the Phase 3 question as well so the user's snippet selection is informed. Default recommendation: prefer `✅ new` snippets unless the user wants deliberate repetition.

### Phase 3 — User Snippet Selection

Ask the user:
- Which snippet(s) to approve for posting (default: all `✅ new` snippets)
- Which image to attach to each snippet (default: hero image for the first, then walk through the article's other images in order; remaining snippets go text-only)

**Default fan-out behavior:** every approved snippet posts to every eligible Buffer channel — one post per (snippet × channel) pair. Do not ask the user to assign different snippets to different platforms; the user picks which snippets are good, and every approved snippet ships to every channel.

**Wait for user input before proceeding.**

### Phase 4 — Compose Posts (full fan-out: every approved snippet × every eligible channel)

For each approved snippet, compose one post per eligible Buffer channel — same snippet text, trimmed (or expanded) to fit that channel's char budget. The matrix is **snippets × channels**, not "one snippet per platform."

**CRITICAL RULE — No Rewriting:**
Never rewrite, paraphrase, or rephrase the author's words. You may only:
- Remove words or sentences to shorten (use ellipsis `...` where text was removed)
- Add line breaks for readability
- Truncate from the end, preserving complete sentences

The post body (excluding CTA) must be a direct excerpt from the article.

**Every post must end with this CTA** (separated by a blank line), using the article title:

```
Comment "newsletter" to get my latest post, "<Article Title>"
```

**Character limits** (CTA is ~70 chars depending on title length, leave ~5 char safety margin):

| Platform   | Limit | Snippet budget              |
|------------|-------|-----------------------------|
| Twitter/X  | 280   | 280 - CTA length - 7 margin |
| Bluesky    | 300   | 300 - CTA length - 7 margin |
| Pinterest  | 300   | 300 - CTA length - 7 margin |
| Threads    | 500   | 500 - CTA length - 7 margin |
| Facebook   | 500   | 500 - CTA length - 7 margin |
| Mastodon   | 500   | 500 - CTA length - 7 margin |
| Instagram  | 2,200 | 2,200 - CTA length - 7 margin |
| TikTok     | 2,200 | 2,200 - CTA length - 7 margin |
| LinkedIn   | 3,000 | 3,000 - CTA length - 7 margin |

Calculate the actual CTA length using the article title to determine snippet budgets. The CTA format is `Comment "newsletter" to get my latest post, "<title>"` — count the full string including the title.

For short-form platforms (Twitter, Bluesky, Pinterest): when a snippet exceeds the budget, trim from the end with `…` while preserving complete sentences. If even the first sentence won't fit, drop that snippet for that channel and surface the skip — never paraphrase to fit.
For long-form platforms (LinkedIn, Instagram): the full snippet can be used, potentially with multiple paragraphs.

**Media attachment (snippet × channel fan-out has a tight image budget):**
- **IMPORTANT — Buffer duplicate detection:** Buffer will reject posts that share the same image URL, treating them as duplicate content. Each (snippet × channel) cell needs either a **unique** image URL or to be text-only across the whole batch.
- **Default distribution:** the article's inline images are a shared pool. Distribute them across the snippet × channel matrix with this priority order, never reusing a URL: (1) **Instagram cells first** — Instagram requires an image, so every snippet's Instagram cell must get one before other channels; (2) **LinkedIn cells next** — visuals lift engagement most here; (3) **Facebook cells**; (4) **Threads cells** — Threads is conversational, text-only is fine. Cells that don't receive an image go text-only.
- **Out of images:** if the pool runs dry, remaining cells go text-only. If a snippet has no image left for its Instagram cell, **skip Instagram for that snippet** rather than sending text-only (Buffer will reject) or reusing a URL (Buffer dedups).
- Attach images via `assets.images` with `metadata.altText` set to the article title.

**Skip TikTok and YouTube channels** — they require video assets, not images.

### Adversarial review (REQUIRED before user review)

Apply the **[Adversarial Review pattern](../PATTERNS.md#pattern-adversarial-review)** with these per-skill specifics:

- **SOURCE_LABEL:** "SOURCE ARTICLE"
- **SOURCE_CONTENT:** the full beehiiv article body, verbatim
- **SKILL_NAME:** `promote-newsletter`
- **ARTIFACT_NAME:** "post"
- **RULES_LIST:**
  - Each post body (excluding CTA) must be a DIRECT EXCERPT from the source. No rewriting, no paraphrasing.
  - Allowed edits: remove words/sentences (use `…`), add line breaks, truncate from the end.
  - Every post MUST end with the canonical CTA from `_shared/cta.sh "<Article Title>"`.
  - No emoji unless explicitly requested.
  - No unverifiable third-party claims ("every leader I respect…", "everyone in [industry] knows…").
- **ISSUE_GUIDANCE:** "If a snippet IS verbatim, confirm by saying 'Verbatim match: lines N-M of source.' Do not give a PASS without citing the verbatim location."

This is what makes "no fabrication" a property of the system rather than a hope.

### Phase 5 — Review Before Publishing

Present all drafted posts for approval:

```
Channel: @handle (Twitter)
Post (247/280 chars):
---
[snippet text]

Comment "newsletter" to get my latest post, "<Article Title>"
---
Image: [image URL]
```

Repeat for each channel. Ask: **"Ready to schedule these to Buffer?"**

The user can:
- Approve all
- Edit individual posts
- Skip specific channels
- Cancel entirely

### Phase 6 — Schedule to Buffer

**🔒 HARD GATE — adversarial review must have returned `summary == "all_pass"` before this phase.**

If the prior `_shared/adversarial-review/adversarial-review` run returned `some_fail` for any draft in this batch, you MUST NOT call `mcp__buffer__create_post` for that draft. Iterate fixes per the [round cap](../PATTERNS.md#round-cap-5-iterations-max), then re-run. After 5 rounds, surface remaining FAILs as deadlocks to the user; only proceed once the user explicitly accepts each deadlocked claim. Spot-checking claims manually does NOT substitute — the 4-way independent review catches issues a careful manual review misses (today's run caught "30+ PRs" inflated metric vs actual 30, voice drift, "on every public method" overstatement — all caught by reviewers and missed by the composer's manual check).

Apply the **[Buffer create_post pattern](../PATTERNS.md#pattern-buffer-create_post-with-channel-filter--caps)** for the transport layer (channel filter, min_followers, max_posts cap, format tag attachment, platform metadata). The skill provides:

- `format-tag = verbatim_quote` (always — per [Per-skill format tag table](../PATTERNS.md#pattern-per-skill-format-tag))
- For each approved post, build args via `_shared/buffer-post-prep/buffer-post-prep`, then call `mcp__buffer__create_post` with the resulting JSON.

The skill's cognition for this phase is **image assignment** (which snippet × channel cell gets which image, since Buffer dedups image URLs) and **scheduling order** (typically snippet-by-snippet across all channels, so each snippet's fan-out goes out together). Fan-out level is no longer a per-run question — the default is "every approved snippet → every eligible channel." The user controls saturation by approving more or fewer snippets in Phase 3. The transport layer enforces:

- Skips channels with `isDisconnected: true`, `isLocked: true`, or `service: "startPage"`
- Skips channels below `min_followers_to_promote = 50` (verified via `mcp__buffer__get_channel`)
- Validates the post text against the platform char limit
- Attaches `tagIds: [<format:verbatim-quote Tag ID>]` from `_shared/buffer-post-prep/tag-ids.local.json` (Buffer's `CreatePostInput` schema requires Tag IDs, not tag-name strings — `tags: [...]` is silently dropped). One-time setup in [`_shared/buffer-post-prep/README.md`](../_shared/buffer-post-prep/README.md). If the local config is missing, the post still ships (untagged) and the binary warns on stderr.
- Sets platform-specific metadata correctly

**Rate limiting:** Buffer's API enforces rate limits (HTTP 429). When scheduling many posts (e.g. multiple articles in one session), you will hit this after ~40-50 rapid `create_post` calls. When rate limited:
- Stop immediately — do not retry in a loop.
- Save all remaining posts (snippet text, CTA, channel IDs, image URLs, Twitter-trimmed variants) to `remaining-posts.md` in the project directory so they can be scheduled in a later session.
- Report to the user which posts succeeded and which are saved for later.

Report results per channel: success or error message.
