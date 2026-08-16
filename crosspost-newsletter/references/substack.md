# Substack post

Use the user's signed-in real browser session.

## Authentication and preflight

Use `/sign-in` redirect behavior or the authenticated dashboard as the login signal. If signed out, keep the tab open and ask the user to log in or recover the account in the browser. Do not request credentials or codes in chat.

After login, discover the publication from the dashboard rather than guessing a subdomain. Search its published posts and drafts for the canonical URL, Beehiiv slug, or exact title. Record a published match; resume an equivalent draft after validation; never duplicate it.

For a catch-up request, compare every relevant Beehiiv feed item with both Substack Published and Drafts. Produce an issue-level matrix with date, title, source URL, Substack match, and intended delivery. Let the user confirm the exact batch before final actions.

## Draft

1. Open a new publication post only when preflight says missing.
2. Enter the source title and subtitle in the visible editor fields.
3. Paste `article-substack.html` into the empty body with one real rich-text paste. It contains remote body-image figures in source order. Do not paste marker text or manually interleave image uploads when this artifact exists; that workflow can swallow the blocks adjacent to an image.
4. Wait for Substack to ingest/rehost the remote images and for autosave to reach `Saved`.
5. Enable the email header/footer. Use the cover for the social preview/thumbnail or a separate hero only when the publication layout requires it; do not count it as a body image.
6. Verify semantic source-block order, headings, populated blockquotes, body images, and first/last paragraph text. Search the editor text for `CROSSPOST_IMAGE`; any match blocks publication.
7. In publish settings keep `Send via email and the Substack app` selected only when the approved delivery includes it. Choose `Publish without buttons` when prompted because email header/footer is already enabled.

## Catch-up delivery safety

- Recommend web-only delivery for historical backfill issues unless the user explicitly requests email/app delivery for them.
- Show the exact historical titles and dates with `Send via email and the Substack app` off. Show the exact issue that will send, subscriber count when visible, and app consequence separately.
- One approval may cover the exact batch. Publish sequentially and verify each public result before advancing.
- Re-read the delivery checkbox before every final button. Historical posts use `Publish on web only`; the approved current issue may use `Send to everyone now`.
- If verification exposes corruption, replace the entire body from `article-substack.html`; do not patch around individual images. A repair may reuse the original approval only when it restores the exact approved package and leaves delivery off. Any copy, package, or delivery change needs a new approval.

## Approval and verification

The approval prompt must say: **Publishing will email subscribers and send through the Substack app.** Show the intended publication, title/subtitle, counts, delivery selection, and no-inline-buttons choice. Do not click `Send to everyone now` before approval.

After approval, send once. Require the public publication `/p/<slug>` URL, then reopen it with `?crosspost_verify=<timestamp>` and verify title, cover/hero state, body-image count, and source link if present.

Compare normalized source blocks in order, excluding title/subtitle from the body comparison. Normalize Unicode quotes, whitespace, and spaces introduced at inline-link boundaries. Substack may split one source paragraph into multiple public `<p>` elements; a raw paragraph-count difference is acceptable only when recombining adjacent public fragments reproduces every source paragraph in order. Heading, populated-blockquote, and body-image counts must still match exactly.
