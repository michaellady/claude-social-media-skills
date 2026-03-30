---
name: crosspost-newsletter
description: Use when user wants to cross-post a beehiiv newsletter article as a full long-form post on LinkedIn, Substack, or Medium using browser automation — "crosspost this article", "cross-post newsletter", "publish to Medium", "post full article to LinkedIn", "syndicate newsletter", "crosspost to substack".
user_invocable: true
---

# crosspost-newsletter

Cross-post a full beehiiv newsletter article to LinkedIn (native article), Substack, and/or Medium using browser automation via gstack browse. Preserves rich formatting, headings, images, and sets canonical URL back to the original beehiiv post.

## Usage

`/crosspost-newsletter <beehiiv-post-url>` or `/crosspost-newsletter latest`

## Process

### Phase 1 — Fetch Full Newsletter Content

**If URL provided:**
Use `WebFetch` with the beehiiv post URL. Extract:
- Article title (exact text)
- Article subtitle/description (if present)
- Full body HTML — preserve all formatting: headings (h2, h3), bold, italic, links, blockquotes, lists, code blocks, images with alt text
- All image URLs (hero image, inline images)
- The canonical URL (the beehiiv post URL itself)

**If "latest" or no URL:**
Fetch the RSS feed via `WebFetch`: `https://rss.beehiiv.com/feeds/9AbhG8CTgD.xml`
List recent articles with titles and dates. Ask the user which one to cross-post, then fetch that article's URL.

**Content preparation:**
After fetching, prepare the body:
1. **Strip beehiiv boilerplate** — remove tracking pixels, analytics images, newsletter signup forms, footer, "View in browser" links, beehiiv-specific CSS classes and inline styles
2. **Keep semantic HTML only** — h1-h6, p, strong, em, a, ul/ol/li, blockquote, pre/code, img (with src and alt)
3. **Prepare a plain-text Markdown fallback** — for platforms where HTML injection fails

Present to the user:
```
Article: "<Title>"
Published: <date>
Length: ~<word count> words, <image count> images
URL: <beehiiv URL>
```

### Phase 2 — Platform Selection

Ask the user which platforms to cross-post to:
- **A)** LinkedIn (native long-form article)
- **B)** Substack
- **C)** Medium
- **D)** All three

**Wait for user input before proceeding.**

Platforms are processed one at a time, sequentially. Each gets its own browser tab and user review gate.

### Phase 3 — Browser Setup & Authentication

#### 3a. Initialize the browse binary

```bash
_ROOT=$(git rev-parse --show-toplevel 2>/dev/null)
B=""
[ -n "$_ROOT" ] && [ -x "$_ROOT/.claude/skills/gstack/browse/dist/browse" ] && B="$_ROOT/.claude/skills/gstack/browse/dist/browse"
[ -z "$B" ] && B=~/.claude/skills/gstack/browse/dist/browse
if [ -x "$B" ]; then
  echo "READY: $B"
else
  echo "NEEDS_SETUP"
fi
```

If `NEEDS_SETUP`: tell the user gstack browse needs a one-time build, then run setup.

#### 3b. Verify authentication for each selected platform

Navigate to a page that reveals login state and take a snapshot:

**LinkedIn:**
```bash
$B goto https://www.linkedin.com/feed/
$B snapshot -i
```
Logged in = feed visible. Not logged in = login/signup page.

**Substack:**
```bash
$B goto https://substack.com/account/settings
$B snapshot -i
```
Logged in = account settings visible. Not logged in = login page.

**Medium:**
```bash
$B goto https://medium.com/me/stories
$B snapshot -i
```
Logged in = stories dashboard visible. Not logged in = login/signup page.

#### 3c. Handle authentication failures

For any platform where the user is NOT logged in:

1. Attempt cookie import:
   ```bash
   $B cookie-import-browser --domain linkedin.com
   ```
   (Replace domain per platform: `linkedin.com`, `substack.com`, `medium.com`)

2. Re-navigate to the check page and snapshot again.

3. If still not logged in, handoff to the user:
   ```bash
   $B handoff "Please log in to <platform> — I'll continue once you're done."
   ```
   Ask the user via AskUserQuestion: "I've opened Chrome at the login page. Please log in and let me know when you're done."

4. After user confirms, `$B resume` and verify login. If still not logged in, skip this platform and report in the summary.

### Phase 4 — Cross-Post to Each Platform

Complete one platform fully (through publish) before starting the next.

---

#### Platform: LinkedIn (Native Article)

**Step 1 — Navigate to article editor:**
```bash
$B goto https://www.linkedin.com/article/new/
$B snapshot -i
```

**Step 2 — Fill the title:**
Find the title input or contenteditable element in the snapshot (typically labeled "Title" or "Headline").
```bash
$B click @eN
$B type_text "<article title>"
```

**Step 3 — Insert the article body:**
Click into the body editor area, then inject content.

**Strategy A — Clipboard paste (preferred):**
```bash
$B js "
  const html = `<ESCAPED_HTML>`;
  const dt = new DataTransfer();
  dt.setData('text/html', html);
  dt.setData('text/plain', html.replace(/<[^>]*>/g, ''));
  const editor = document.querySelector('[role=\"textbox\"][contenteditable=\"true\"], .ql-editor, [data-testid=\"editor-content\"], .article-editor__content [contenteditable]');
  if (editor) {
    editor.focus();
    const evt = new ClipboardEvent('paste', { clipboardData: dt, bubbles: true, cancelable: true });
    editor.dispatchEvent(evt);
    'PASTE_OK';
  } else {
    'EDITOR_NOT_FOUND';
  }
"
```

If `EDITOR_NOT_FOUND`: take a fresh `$B snapshot -i`, find the editor, adjust selector, retry.

If `PASTE_OK`: verify content appeared:
```bash
$B snapshot -c -d 3
$B screenshot /tmp/linkedin-article-preview.png
```

**Strategy B — Direct innerHTML (fallback):**
```bash
$B js "
  const editor = document.querySelector('[role=\"textbox\"][contenteditable=\"true\"], .ql-editor');
  if (editor) {
    editor.innerHTML = `<ESCAPED_HTML>`;
    editor.dispatchEvent(new Event('input', { bubbles: true }));
    'SET_OK';
  } else {
    'EDITOR_NOT_FOUND';
  }
"
```

**Strategy C — User handoff (last resort):**
Copy HTML to clipboard via JS, then `$B handoff "Please paste the article body (Cmd+V) into the editor."` Wait for user, then `$B resume`.

**Step 4 — Set canonical URL:**
Look for a settings gear/menu or "Publishing settings" button in the snapshot. Click it and look for a "Canonical URL" or "Original article" field:
```bash
$B snapshot -i
$B fill @eN "<beehiiv post URL>"
```
If no canonical URL field is found, note this in the summary.

**Step 5 — Screenshot for review:**
```bash
$B screenshot /tmp/linkedin-article-review.png
```
Show screenshot to user (Phase 5).

---

#### Platform: Substack

**Step 1 — Determine Substack subdomain:**
```bash
$B goto https://substack.com/account/settings
$B text
```
Look for the publication name/subdomain. If multiple publications, ask the user which one.

**Step 2 — Navigate to post editor:**
```bash
$B goto https://<subdomain>.substack.com/publish/post
$B snapshot -i
```

**Step 3 — Fill the title and subtitle:**
Find title and subtitle fields in the snapshot:
```bash
$B click @eN
$B type_text "<article title>"
$B click @eM
$B type_text "<article subtitle>"
```

**Step 4 — Insert the article body:**
Substack uses ProseMirror. Apply the same clipboard paste strategy as LinkedIn:
```bash
$B js "
  const html = `<ESCAPED_HTML>`;
  const dt = new DataTransfer();
  dt.setData('text/html', html);
  dt.setData('text/plain', html.replace(/<[^>]*>/g, ''));
  const editor = document.querySelector('.ProseMirror[contenteditable=\"true\"], [role=\"textbox\"][contenteditable=\"true\"]');
  if (editor) {
    editor.focus();
    const evt = new ClipboardEvent('paste', { clipboardData: dt, bubbles: true, cancelable: true });
    editor.dispatchEvent(evt);
    'PASTE_OK';
  } else {
    'EDITOR_NOT_FOUND';
  }
"
```
Same fallback cascade (innerHTML, then handoff).

**Step 5 — Set canonical URL:**
Look for a "Settings" button or gear icon in the snapshot. Click it and look for "Canonical URL" field:
```bash
$B snapshot -i
$B fill @eN "<beehiiv post URL>"
```

**Step 6 — Screenshot for review:**
```bash
$B screenshot /tmp/substack-article-review.png
```

---

#### Platform: Medium

**Step 1 — Navigate to story editor:**
```bash
$B goto https://medium.com/new-story
$B snapshot -i
```

**Step 2 — Fill the title:**
Find the title placeholder in the snapshot. Click and type:
```bash
$B click @eN
$B type_text "<article title>"
$B press Enter
```

**Step 3 — Insert the article body:**
Apply the same clipboard paste strategy:
```bash
$B js "
  const html = `<ESCAPED_HTML>`;
  const dt = new DataTransfer();
  dt.setData('text/html', html);
  dt.setData('text/plain', html.replace(/<[^>]*>/g, ''));
  const editor = document.querySelector('[role=\"textbox\"][contenteditable=\"true\"], .ProseMirror, section[contenteditable=\"true\"]');
  if (editor) {
    editor.focus();
    const evt = new ClipboardEvent('paste', { clipboardData: dt, bubbles: true, cancelable: true });
    editor.dispatchEvent(evt);
    'PASTE_OK';
  } else {
    'EDITOR_NOT_FOUND';
  }
"
```
Same fallback cascade.

**Step 4 — Set canonical URL:**
Look for "..." menu or story settings. Click it, find "Advanced settings" or "More settings", then locate the "Canonical link" field:
```bash
$B snapshot -i
$B fill @eN "<beehiiv post URL>"
```
Medium also allows setting canonical URL in the publish confirmation — check there if not found in pre-publish settings.

**Step 5 — Screenshot for review:**
```bash
$B screenshot /tmp/medium-article-review.png
```

---

### Phase 5 — User Review (per platform)

After completing content injection for each platform, show the screenshot and ask:

```
<Platform> article is ready for review.

Title: "<Article Title>"
Body: ~<word count> words injected
Images: <count> (via original beehiiv CDN URLs)
Canonical URL: <set / not found>

Options:
A) Publish now
B) Let me review in browser first (handoff)
C) Skip this platform
D) Abort all remaining platforms
```

**Wait for user input.**

If **B**: `$B handoff "Review the article and make any edits. Tell me when ready to publish."` Then `$B resume` when done.

### Phase 6 — Publish

For each approved platform, click the publish button.

**LinkedIn:**
```bash
$B snapshot -i
```
Find and click "Publish" button:
```bash
$B click @eN
$B screenshot /tmp/linkedin-published.png
$B url
```
Capture the published article URL.

**Substack:**
```bash
$B snapshot -i
```
Find and click "Publish" or "Continue". Substack may show a publish dialog.

**CRITICAL: Substack asks whether to send the post as an email to subscribers. Since this is a cross-post (not the original), UNCHECK any "Send to email subscribers" or "Email this post" option.** Only publish as a web-only post.

```bash
$B snapshot -i
```
Uncheck email option if present, then click final publish:
```bash
$B click @eN
$B screenshot /tmp/substack-published.png
$B url
```

**Medium:**
```bash
$B snapshot -i
```
Find and click "Publish". Medium shows a publish confirmation with tags and subtitle options. If canonical URL wasn't set earlier, look for it here:
```bash
$B click @eN
$B snapshot -i
$B click @eN
$B screenshot /tmp/medium-published.png
$B url
```

**After each publish, screenshot and show confirmation to the user.**

### Phase 7 — Summary

```
Cross-post complete!

| Platform  | Status    | URL                          | Canonical |
|-----------|-----------|------------------------------|-----------|
| LinkedIn  | Published | <url>                        | Set       |
| Substack  | Published | <url>                        | Set       |
| Medium    | Skipped   | —                            | —         |
```

## Important Notes

### HTML Escaping for $B js
When embedding article HTML in `$B js` expressions:
- Escape backticks (`` ` `` → `` \` ``)
- Escape `${` sequences (`${` → `\${`)
- For very long articles, write HTML to a temp file and read it in the JS expression instead of inlining

### Image Handling
- Inline images reference their original beehiiv CDN URLs — most platforms render these via hotlink when HTML is pasted
- If a platform strips external images during paste, note this in the summary
- For cover/hero images, use the platform's cover image feature if available

### Editor Detection Heuristic
If `snapshot -i` doesn't clearly identify the editor, probe with:
```bash
$B js "
  const editables = [...document.querySelectorAll('[contenteditable=\"true\"]')];
  editables.map((el, i) => ({
    index: i, tag: el.tagName, role: el.getAttribute('role'),
    classes: el.className.substring(0, 100),
    rect: el.getBoundingClientRect(), textLength: el.textContent.length
  }));
"
```
The main body editor is typically the largest contenteditable by dimensions.

### Adaptive Navigation
Do not hardcode CSS selectors. After every navigation and action, `$B snapshot -i` to discover elements by accessible names and roles. If the page layout changes, the snapshot-based approach still works.

### Graceful Degradation
If automated injection fails after all strategies:
1. Copy full HTML to clipboard via JS
2. `$B handoff` with clear paste instructions
3. `$B resume` after user completes
4. Continue with canonical URL and publish flow

Never skip a platform silently — always inform the user and offer alternatives.
