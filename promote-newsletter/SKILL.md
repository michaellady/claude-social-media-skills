---
name: promote-newsletter
description: Use when user wants to promote a beehiiv newsletter post on social media, create social posts from a newsletter, or schedule newsletter promotions to Buffer channels — "promote this newsletter", "share newsletter on social", "create posts for this article".
user_invocable: true
---

# promote-newsletter

Extract the strongest snippets from a beehiiv newsletter post and create platform-specific social media posts via Buffer MCP. Never rewrite content — only trim to fit.

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
- Which snippet(s) to use
- Whether to use the same snippet for all platforms or assign different snippets to short-form vs long-form platforms
- Which image to attach (default: hero image)

**Wait for user input before proceeding.**

### Phase 4 — Compose Platform-Specific Posts

For each connected Buffer channel, compose a post from the selected snippet.

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

For short-form platforms (Twitter, Bluesky, Pinterest): pick the most concise snippet or trim aggressively.
For long-form platforms (LinkedIn, Instagram): the full snippet can be used, potentially with multiple paragraphs.

**Media attachment:**
- **IMPORTANT — Buffer duplicate detection:** Buffer will reject posts that share the same image URL, treating them as duplicate content. When scheduling multiple posts from the same article, each post must use a **different image** or go text-only. Ask the user which post should get which image.
- If the newsletter has fewer images than posts, only attach images to the user's chosen posts and send the rest as text-only.
- Instagram requires an image — only schedule to Instagram on posts that have an image attached. Skip Instagram for text-only posts.
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

Apply the **[Buffer create_post pattern](../PATTERNS.md#pattern-buffer-create_post-with-channel-filter--caps)** for the transport layer (channel filter, min_followers, max_posts cap, format tag attachment, platform metadata). The skill provides:

- `format-tag = verbatim_quote` (always — per [Per-skill format tag table](../PATTERNS.md#pattern-per-skill-format-tag))
- For each approved post, build args via `_shared/buffer-post-prep/buffer-post-prep`, then call `mcp__buffer__create_post` with the resulting JSON.

The skill's cognition for this phase is choosing WHICH approved posts go to which channels in what order, which images to attach, and how many snippets to drop if the user picked more than `max_posts_per_channel_per_article` (default 3). The transport layer enforces:

- Skips channels with `isDisconnected: true`, `isLocked: true`, or `service: "startPage"`
- Skips channels below `min_followers_to_promote = 50` (verified via `mcp__buffer__get_channel`)
- Validates the post text against the platform char limit
- Attaches `tags: ["format:verbatim-quote"]`
- Sets platform-specific metadata correctly

**Rate limiting:** Buffer's API enforces rate limits (HTTP 429). When scheduling many posts (e.g. multiple articles in one session), you will hit this after ~40-50 rapid `create_post` calls. When rate limited:
- Stop immediately — do not retry in a loop.
- Save all remaining posts (snippet text, CTA, channel IDs, image URLs, Twitter-trimmed variants) to `remaining-posts.md` in the project directory so they can be scheduled in a later session.
- Report to the user which posts succeeded and which are saved for later.

Report results per channel: success or error message.
