# Troubleshooting and verification

## Authentication

- A public profile or homepage is not proof of authentication; use an account dashboard or editor.
- Preserve a signed-out Substack tab for in-browser login/recovery. Never ask for credentials or one-time codes in chat.
- CAPTCHA and account-recovery challenges are user handoffs.

## Source preparation

- If feed and rendered URLs use different custom domains, match normalized `/p/<slug>` paths.
- If counts differ, inspect whether the rendered snapshot included navigation/footer nodes or whether RSS contains Beehiiv boilerplate. Correct the extraction; never weaken the baseline.
- Keep the cover separate. Body image count excludes the cover.
- `crosspost-prep --out` accepts a parent directory and creates a unique run child. Use the run path printed on stdout; never share fixed `/tmp/article-*` files across runs.
- Use `article-substack.html` for a one-paste Substack body. It embeds sanitized remote image URLs in ordered figures; `article-linkedin-substack.html` retains numbered upload anchors for LinkedIn.
- Image download failure blocks full-fidelity drafting. Do not silently omit the image.

## Editor integrity

- Re-snapshot after navigation, post-type changes, modals, flair changes, or uploads. Stale references can send text into the wrong field.
- Medium: after the initial real paste, prefer real clicks/keystrokes. Direct DOM mutation can leave a draft unsavable.
- LinkedIn: confirm a fresh modal button/ref after each upload and verify the canonical public `/pulse/` state rather than trusting a redirect.
- Substack: distinguish visible editor title/subtitle fields from hidden metadata duplicates.
- Substack: never publish marker text such as `CROSSPOST_IMAGE`. Manual marker replacement can truncate the paragraph or heading adjacent to an image; replace the full body with `article-substack.html` instead.
- When browser clipboard APIs are unavailable on macOS, set the pasteboard's HTML flavor from the generated file, then use real focus/select-all/paste actions in the editor:

  ```text
  osascript -e 'use scripting additions' -e 'set h to read (POSIX file "/absolute/run/article-substack.html") as «class HTML»' -e 'set the clipboard to h'
  ```

  Treat any OS automation prompt as a permission handoff. Never type the HTML as plain text.
- Hacker News: text is optional alongside a URL; keep any note under 500 characters.
- Reddit: validate title, URL, and body after every mode/flair transition to prevent field contamination.

## Final-action safety

- The final approval must refer to the exact visible draft and consequence.
- A changed draft needs a new approval.
- A successful click is not a successful publication. Require the platform's public URL and reopen it.
- Add a unique `crosspost_verify` query value when reopening public pages. If the old body still appears, force a fresh navigation before diagnosing the publication as stale or corrupt.
- Do not auto-retry a dead HN item or removed Reddit post.

## Public verification checklist

For full articles verify exact title, first/last paragraph, ordered normalized source blocks, heading count, populated-blockquote count, cover, every body image, and source/canonical link where supported. Exclude title/subtitle from body comparison and normalize whitespace introduced around inline links. For link submissions verify target, exact submitted text, destination URL, public visibility, and moderation/dead state. Recheck HN immediately before the final receipt.
