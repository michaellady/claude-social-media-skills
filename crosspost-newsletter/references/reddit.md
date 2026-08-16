# Reddit link posts

Use the user's signed-in real browser session. Create a separate link post for each selected subreddit.

## Preflight and fit

Inspect the user's submitted profile and each target subreddit for the Beehiiv URL, slug, or matching title. Record existing posts, including removed ones, and do not duplicate or retry them without explicit new approval.

Read the target community's current rules before drafting. Choose whether the article fits, then choose title, discussion body, post flair, and any required user flair in browser/model judgment.

## Draft safely

For each subreddit:

1. Open the bare `/r/<subreddit>/submit/` route, then select the Link post type.
2. Take a fresh interactive snapshot. Fill title, Link URL, and a distinct discussion-oriented body. Avoid “check out my article” framing; invite a community-specific discussion.
3. If flair is required, choose a fitting flair. A persistent subreddit user flair is different from post flair; hand off if the community requires one.
4. After every post-type or flair change, take a new snapshot and re-read the live values of title, URL, and body. Clear and refill any field that is empty or contaminated. Never reuse stale element references.
5. Re-check community warnings, Post-button state, target subreddit, flair, and all text immediately before approval.

## Approval and submission

Two Reddit drafts may share one approval only if the prompt shows both complete drafts verbatim: subreddit, title, URL, body, and flair. Any subsequent edit invalidates that approval.

After approval, submit the first post and verify it before proceeding. Wait at least 90 seconds, revalidate the second draft, then submit it. A CAPTCHA or required persistent user flair is a user handoff.

Reopen each public `/comments/<id>/` result. Verify title, link destination, subreddit, flair, and visible body. Check the user profile and subreddit listing for moderator/filter removal. Do not retry a removed post without new approval.

If a headless/public verifier shows “Prove your humanity,” a network-security block, or another bot challenge, mark that check inconclusive. Recheck the post, profile, and subreddit listing in the signed-in real browser; absence of removal text on a challenge page is not evidence that the post survived moderation.
