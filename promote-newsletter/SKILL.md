---
name: promote-newsletter
description: Use when user wants to promote a beehiiv newsletter post on social media, create social posts from a newsletter, or schedule newsletter promotions to Buffer channels — "promote this newsletter", "share newsletter on social", "create posts for this article".
user_invocable: true
---

# promote-newsletter

Extract the strongest snippets from a beehiiv newsletter post and schedule each selected snippet to every eligible Buffer channel. Never rewrite content — only trim to fit each channel's char budget.

## Usage

`/promote-newsletter <beehiiv-post-url>` or `/promote-newsletter latest`

## 🟢 Happy Path (read first; everything below is edge-case detail)

For a beehiiv newsletter promotion when nothing goes wrong. ~5-10 min wall-clock, mostly waiting on user review. Linear flow:

**Phase 1 — Fetch content (~1 min).**
- Refresh voice-corpus cache: `_shared/voice-corpus/voice-corpus --refresh`. Pull article body via `jq -r '.posts[] | select(.url | contains("<slug>")) | .body_text' _shared/voice-corpus/cache.json > /tmp/newsletter-body.txt`. (Prefer this over WebFetch — beehiiv URLs trip its copyright guardrail.)
- WebFetch the article URL with an image-only prompt to enumerate every image URL (hero, inline, charts).
- HEAD-check each image URL with `curl -sI ... -w "%{http_code}"`; drop any non-200 from the pool (beehiiv's CDN occasionally serves 307→403).

**Phase 2 — Pick snippets + check queue (~1 min).**
- Select 3-5 strongest snippets from the body — concrete insights, stats, or provocative claims that stand alone.
- Call `mcp__buffer__get_account` for the org ID, then `mcp__buffer__list_posts` (`status: ["scheduled", "needs_approval", "draft"]`, `first: 100`) to fetch the upcoming queue.
- Substring-match each snippet (distinctive 4-8 word phrase) and the article title against queued post text. Annotate each snippet `✅ new` or `⚠️ queued Nx (earliest YYYY-MM-DD)`.
- Present the numbered snippet list + image pool to the user, hero image flagged as the default attachment.

**Phase 3 — User snippet selection.** Single question: which snippets to approve (default: all `✅ new`) and which image to attach to each. Wait for input. Every approved snippet fans out to every eligible Buffer channel.

**Phase 4 — Compose (snippet × channel matrix).** Build a JSON manifest with one cell per (snippet × channel) — never a bash array, since snippet bodies are multi-line. For each cell: trim the verbatim excerpt to fit the channel's char budget (use `…` for truncation, never paraphrase) and append the canonical CTA `Comment "newsletter" to get my latest post, "<Article Title>"` separated by a blank line. Distribute the image pool with priority Instagram → LinkedIn → Facebook → Threads, never reusing a URL (Buffer dedups). Skip TikTok / YouTube channels (video-only).

**Phase 5 — Adversarial review.** Run `_shared/adversarial-review/adversarial-review` on the full batch with `SOURCE_LABEL: "SOURCE ARTICLE"`, the full beehiiv body, and the verbatim-excerpt + CTA rules. Must return `all_pass` before Phase 6.

**Phase 6 — User review.** Render the matrix as a single table (one row per cell: snippet, channel, chars/limit, image, body preview), followed by each unique body once. Ask "Ready to schedule all N posts to Buffer?". User can approve all, edit by row #, drop a snippet, skip a channel, or cancel.

**Phase 7 — Schedule.** For each approved cell, build args via `_shared/buffer-post-prep/buffer-post-prep` (attaches `format-tag = verbatim_quote`) and call `mcp__buffer__create_post`. Report per-channel success.

## Process

### Phase 1 — Fetch Newsletter Content

**Always try `_shared/voice-corpus` first for the body text.** The binary fetches the user's beehiiv RSS feed and caches every post's plain-text body — no rate limits, no copyright guardrails, no LLM-mediated summarization. Confirmed 2026-05-17: `WebFetch` on the user's own newsletter URL refused to return the article verbatim ("substantial reproduction of copyrighted material"), which broke the verbatim-quote rule. The RSS cache has no such problem.

```bash
_shared/voice-corpus/voice-corpus --refresh  # fetch fresh feed
# then jq to pull the article body:
jq -r '.posts[] | select(.url | contains("<slug>")) | .body_text' \
  _shared/voice-corpus/cache.json > /tmp/newsletter-body.txt
```

**WebFetch is still needed for image URLs**, since voice-corpus strips HTML. Make a SECOND WebFetch call to the article URL with a tightly-scoped prompt:

```
List EVERY image URL that appears in the article — hero, inline images,
graphs/charts, screenshots. For each one tell me: (a) full URL, (b) what
it depicts (one short phrase), (c) approximately where it appears. Do not
summarize the article. Just enumerate the image URLs.
```

**Pre-validate every image URL with a HEAD request** before adding it to the image pool. beehiiv's `media.beehiiv.com/cdn-cgi/image/...` URLs occasionally return a 307 redirect to a 403, and Buffer's image-dimensions fetcher rejects them at `create_post` time with "Failed to fetch image dimensions: Not Found". Confirmed 2026-05-17 on the Magic-section image of "The Fix-Forward Solutions to AI Coding Problems."

```bash
for url in "${IMAGE_URLS[@]}"; do
  code=$(curl -sI "$url" -o /dev/null -w "%{http_code}")
  if [[ "$code" == "200" ]]; then echo "OK  $url"
  else echo "DROP ($code) $url"
  fi
done
```

Drop any non-200 URL from the image pool. Note the drop count in Phase 2 so the user sees how many images were filtered.

**If "latest" or no URL:**
Refresh the voice-corpus cache and list the recent articles with titles and dates. Ask the user which one to promote, then proceed with that article's URL.

**If the article isn't on beehiiv** (rare — this skill is beehiiv-first): fall back to the full WebFetch flow. Be aware its content-policy guardrails may refuse full-body extraction.

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

**Prep manifest format.** Snippet bodies are multi-line; bash arrays + `read -r IFS='|'` truncate at the first newline. Use a JSON manifest instead — one object per cell, then iterate in Python:

```json
[
  {"id": "01", "channel_id": "692438d9...", "service": "linkedin",
   "text": "We as an industry...\n\n…\n\nWhat can you add to tokens that someone else can't? Your perspective.\n\nComment \"newsletter\" to get my latest post, \"<Article Title>\"",
   "image_url": null, "image_alt": null},
  {"id": "02", "channel_id": "6934230229...", "service": "linkedin",
   "text": "We as an industry...", "image_url": "https://media.beehiiv.com/...",
   "image_alt": "<Article Title>"}
]
```

A Python loop calling `buffer-post-prep` once per cell handles multi-line bodies safely. Confirmed 2026-05-17: a bash array-based prep silently truncated all 6 long-form bodies to their first line.

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

Render the snippet × channel matrix as a single table — at 4 snippets × 6 channels = 24 posts, the per-channel narrative format used to drown in scroll. Show one row per cell with the cell's character count, image assignment, and a body preview:

```
Snippet × Channel matrix (24 cells)

# | snippet | channel        | chars/limit | image                  | body preview
01 | A      | LinkedIn (Mike)| 613/3000    | text-only              | We as an industry need…
02 | A      | LinkedIn (EVC) | 613/3000    | Open Secret (#2)       | We as an industry need…
03 | A      | Threads (mike) | 307/500     | text-only              | The key question for…
...
```

Follow the table with the full body of each unique snippet variant (long-form vs short-form per snippet — typically 2 variants × N snippets = 6–10 unique bodies, not 24), so the user can scan once per snippet rather than once per cell.

Ask: **"Ready to schedule all N posts to Buffer?"**

The user can:
- Approve all
- Edit individual cells (by row #)
- Drop a snippet entirely (removes all of its row entries)
- Skip a specific channel column
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
