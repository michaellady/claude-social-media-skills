# Hacker News link submission

Use the user's signed-in real browser session at `news.ycombinator.com/submit`.

## Preflight

Inspect the authenticated user's submitted page and search HN for the Beehiiv URL, slug, and intended title. Record a matching item even if its title was adapted. Do not resubmit a dead or duplicate item automatically.

## Draft

1. Use a descriptive title no longer than 80 characters. Remove birthday/listicle/clickbait framing when a source-grounded descriptive title is clearer. Use `Show HN:` only for a genuine original-work demo.
2. Fill the Beehiiv canonical URL.
3. A URL submission may also include text. Use an optional source-grounded `Author here` note under 500 characters; do not paste the article or invent claims. HN may materialize this text as the submitter's first comment.
4. Re-read all three fields from the live form after filling them.

## Approval and verification

Show the exact title, URL, and note verbatim immediately before submission. Wait for approval, then submit once.

Require an HN item URL. Reopen it and verify the title and destination URL, then check the user's submitted page/newest for visibility and ensure the item is not marked `[dead]`. If an author note was used, verify the resulting comment is present and not `[flagged]`.

Check again immediately before the final cross-post receipt. The public item page or the official Firebase item endpoint may reveal `dead: true` after an initially successful redirect. Report the final state as `dead`; rate limiting, a flagged note, or a dead item is an outcome, not permission to retry.
