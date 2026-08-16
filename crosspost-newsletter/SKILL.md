---
name: crosspost-newsletter
description: Cross-post or syndicate a Beehiiv newsletter to LinkedIn, Medium, Substack, Hacker News, and Reddit with source-faithful preparation, idempotent destination checks, browser automation, per-platform approval gates, and public-result verification. Use for requests such as "cross-post this newsletter", "publish my latest Beehiiv issue everywhere", "syndicate to Medium or Substack", "submit this issue to HN", or "share this article on Reddit".
---

# Cross-post a Beehiiv newsletter

Treat Beehiiv as the canonical source. Prepare deterministic content with `crosspost-prep`; keep platform fit, copy, authentication handoffs, and publish decisions in the browser workflow.

## Non-negotiable rules

1. Never duplicate an existing destination post. Run preflight before drafting.
2. Never click a final Publish, Send, Submit, or Post control without a fresh approval for that platform. An approval covers only the exact draft shown.
3. Re-read visible fields immediately before asking for approval. If the editor changes afterward, re-approve.
4. Never request a password, recovery secret, CAPTCHA answer, or one-time code in chat. Let the user complete those in the open browser.
5. Preserve source order and content. Do not silently omit text, headings, images, links, or populated blockquotes.
6. Reopen every public result with a cache-busting query and verify it before reporting success. A stale tab is not evidence.

## Load references progressively

Read only the references needed for selected or blocked targets:

- [LinkedIn](references/linkedin.md)
- [Medium](references/medium.md)
- [Substack](references/substack.md)
- [Hacker News](references/hacker-news.md)
- [Reddit](references/reddit.md)
- [Troubleshooting and verification](references/troubleshooting.md)

## Phase 1: Resolve and snapshot the source

1. Resolve the feed URL from the request or local configuration. For Enterprise Vibe Code, use `https://rss.beehiiv.com/feeds/9AbhG8CTgD.xml`.
2. Resolve `latest` from the feed. If the user supplies an article URL, match by normalized URL path so Beehiiv custom-domain aliases do not create a false mismatch.
   For a catch-up request, enumerate the relevant Beehiiv issues first. Do not assume only the latest issue is missing.
3. Open the canonical article in the user's browser and capture a rendered snapshot as JSON. Use this shape:

```json
{
  "url": "https://newsletter.example.com/p/slug",
  "title": "Source title",
  "subtitle": "Source subtitle",
  "published_at": "2026-06-15",
  "cover_image_url": "https://...",
  "paragraph_count": 100,
  "heading_count": 16,
  "blockquote_count": 0,
  "rendered_only_paragraphs": [
    {"index": 20, "text": "A paragraph present in the rendered page but absent from RSS"}
  ],
  "body_images": [
    {"src": "https://...", "alt": "", "caption": ""}
  ]
}
```

Extract counts from the rendered article body, not the full page. Keep the cover separate from body images. Omit empty blockquotes from `blockquote_count`. When rendered paragraphs are absent from RSS, diff normalized paragraph sequences and record only the rendered-only paragraphs with their zero-based rendered paragraph indexes. Never synthesize missing text.

4. Run the preparation helper from this skill directory for each issue selected after preflight:

```text
scripts/crosspost-prep/crosspost-prep prepare \
  --feed-url <feed-url> \
  --article latest|<article-url> \
  --rendered <browser-snapshot.json> \
  [--out <parent-directory>]
```

If the binary does not exist, build it with `go build -o crosspost-prep .` from `scripts/crosspost-prep/`. `--out` is a parent directory; every invocation still creates a unique child run directory.

5. Read `manifest.json`. Stop if source and rendered counts disagree. Resolve extraction or snapshot errors instead of lowering the expected counts.

The helper owns deterministic transport: RSS selection, semantic sanitization, tracking-parameter removal, source order, image downloads and anchors, target HTML variants, unique run directories, and manifest creation. The helper must not choose platforms, titles, copy, topics, flairs, retries, or publication actions.

## Phase 2: Idempotent preflight

Before creating any draft, inspect each selected destination in the signed-in browser. Search both published content and existing drafts where the platform exposes them. Match in this order:

1. Exact normalized canonical/source URL.
2. Exact normalized title plus author/publication.
3. Same Beehiiv slug in a destination URL, body, or metadata.

Treat a strong match as already complete even if its publication date differs from Beehiiv. Record the public URL and do not create another draft.

Verify authentication with a page that exposes account state. A visible sign-in form means blocked. Do not interpret a public homepage as authenticated.

Produce this matrix before drafting. For a catch-up, use one Substack row per Beehiiv issue so published posts and drafts cannot hide gaps:

| Destination | Status | Evidence | Action |
|---|---|---|---|
| LinkedIn | complete / missing / blocked | URL or auth signal | record / draft / handoff |
| Medium | complete / missing / blocked | URL or auth signal | record / draft / handoff |
| Substack | complete / missing / blocked | URL or auth signal | record / draft / handoff |
| Hacker News | complete / missing / blocked | URL or auth signal | record / draft / handoff |
| Reddit r/... | complete / missing / blocked | URL or auth signal | record / draft / handoff |

If a platform is blocked on login, leave its row blocked and continue with independent targets. For Substack, keep the sign-in tab open for user handoff.

## Phase 3: Validate the package

Validate `manifest.json` against the live baseline:

- title, subtitle, publication date, and canonical URL match;
- paragraph, heading, populated-blockquote, and body-image counts match exactly;
- cover exists separately and is not counted as a body image;
- every image has a downloaded local asset and an ordered image position;
- LinkedIn HTML has one numbered image anchor for every body image;
- `article-substack.html` contains every body image as a remote `<figure>` in source order and contains no upload anchors;
- Medium HTML contains the cover first and every body image in source order;
- no Beehiiv footer/subscribe boilerplate or tracking parameters remain.

Do not draft with an invalid package.

## Phase 4: Draft missing targets

Process full-article targets before link submissions. Read each selected platform reference before operating its editor.

### Full-article targets

- LinkedIn: if preflight found an existing native article, record it and skip all editor and accompanying-post work. Otherwise follow [LinkedIn](references/linkedin.md).
- Medium: use the source title/subtitle, cover, all body images, canonical URL, and approved topics. Follow [Medium](references/medium.md).
- Substack: discover the user's publication from the authenticated dashboard. Paste `article-substack.html` as one rich body, use email header/footer, and avoid redundant inline subscribe buttons. For catch-up requests, follow the batch and delivery rules in [Substack](references/substack.md).

### Link-submission targets

- Hacker News: adapt the title for HN and optionally add a short source-grounded author note. URL plus text is allowed. Follow [Hacker News](references/hacker-news.md).
- Reddit: create a distinct discussion-oriented link draft for each selected subreddit. Follow [Reddit](references/reddit.md).

Keep these decisions in model/browser work:

- which platforms and subreddits fit;
- title adaptation and accompanying copy;
- Medium topics;
- Reddit body, flair, and community-rule judgment;
- whether and how to retry;
- every approval and publish decision.

## Phase 5: Approval gates

Present one approval prompt per platform after the completed editor is visible and verified. Reddit drafts may be approved together only when both exact drafts are shown in the same prompt.

A Substack catch-up may use one approval for an exact enumerated batch only when the prompt shows every issue in order, each delivery setting, and which issue—if any—will email subscribers or send through the app. Any content, order, or delivery change invalidates that approval.

For a full article, show:

- destination/account;
- exact title and subtitle;
- source and rendered paragraph/heading/image counts;
- cover state;
- canonical URL state or platform limitation;
- email/app delivery state;
- exact consequence of the final button.

For a link submission, show the exact target, title, URL, note/body, and flair. For Hacker News show title, URL, and author note verbatim. For Substack say explicitly: **Publishing will email subscribers and send through the Substack app.**

Wait for approval. A request to edit invalidates the prior approval.

## Phase 6: Publish and verify

After approval, perform only the approved platform's final action. Then reopen the public result in a fresh tab with `?crosspost_verify=<timestamp>` (or `&crosspost_verify=`) and verify the platform-specific canonical success signal in its reference.

At minimum verify:

- public URL and exact title;
- canonical/source link where supported;
- source-block order and heading completeness for full articles;
- cover and all body images;
- HN item is visible and not marked dead;
- Reddit post is visible on the target subreddit/profile and not removed or filtered.

Do not retry a removed Reddit post, killed HN submission, or materially changed draft without a new approval.

Before the final receipt, recheck HN once more. An item that was initially visible but later becomes `[dead]` is `dead`, not `submitted and visible`.

Wait at least 90 seconds between Reddit submissions.

## Phase 7: Receipt

Return one row for Beehiiv, LinkedIn, Medium, Substack, HN, and every subreddit:

| Platform | Target | Status | Public URL | Verification |
|---|---|---|---|---|

Use precise statuses: `canonical source`, `already published`, `published and verified`, `submitted and visible`, `blocked on login`, `removed`, `dead`, or `skipped by user`. Never label a draft or an unverified redirect as published.
