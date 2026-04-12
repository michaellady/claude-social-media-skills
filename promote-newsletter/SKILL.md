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

### Phase 5 — Review Before Publishing

Present all drafted posts for approval:

```
Channel: @handle (Twitter)
Post (247/280 chars):
---
[snippet text]

Comment "newsletter" to get a link to my latest post, "<Article Title>"
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

1. Call `mcp__buffer__get_account` to get the organization ID and timezone. If multiple orgs, ask the user which one.
2. Call `mcp__buffer__list_channels` with the org ID. Never guess channel IDs.
3. **Filter out disconnected/locked channels before composing or posting.** Each channel has `isDisconnected` and `isLocked` booleans — skip any where either is `true`. Also skip `service: "startPage"` (not a social channel). Silently omit them to avoid wasted API calls and dead posts.
4. For each approved post, call `mcp__buffer__create_post` with:
   - `channelId`: exact ID from `list_channels`
   - `text`: the composed post text (snippet + CTA)
   - `mode`: `"addToQueue"`
   - `schedulingType`: `"automatic"`
   - `assets`: `{ images: [{ url: "<image-url>", metadata: { altText: "<article-title>" } }] }`
   - Platform-specific `metadata`:
     - **Facebook**: `metadata.facebook.type: "post"`
     - **Instagram**: `metadata.instagram.type: "post"`, `metadata.instagram.shouldShareToFeed: true`
     - **Pinterest**: `metadata.pinterest.boardServiceId` (get from `get_channel` response, under `metadata.boards[].serviceId`)
     - **LinkedIn, Twitter, Threads, Bluesky, Mastodon**: no extra metadata required
4. **Rate limiting:** Buffer's API enforces rate limits (HTTP 429). When scheduling many posts (e.g. multiple articles in one session), you will hit this after ~40-50 rapid `create_post` calls. When rate limited:
   - Stop immediately — do not retry in a loop.
   - Save all remaining posts (snippet text, CTA, channel IDs, image URLs, Twitter-trimmed variants) to `remaining-posts.md` in the project directory so they can be scheduled in a later session.
   - Report to the user which posts succeeded and which are saved for later.
5. Report results per channel: success or error message.
