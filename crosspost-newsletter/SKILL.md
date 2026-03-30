---
name: crosspost-newsletter
description: Use when user wants to cross-post a beehiiv newsletter article as a full long-form post on LinkedIn, Substack, or Medium using browser automation — "crosspost this article", "cross-post newsletter", "publish to Medium", "post full article to LinkedIn", "syndicate newsletter", "crosspost to substack".
user_invocable: true
---

# crosspost-newsletter

Cross-post a full beehiiv newsletter article to LinkedIn (native article), Substack, and/or Medium. Uses gstack browse for LinkedIn and Substack, and Claude in Chrome (mcp__claude-in-chrome__*) for Medium (which blocks headless browsers via Cloudflare). Preserves rich formatting, headings, images, and sets canonical URL back to the original beehiiv post. If a platform offers the option to send the article as an email to subscribers, always enable it.

## Usage

`/crosspost-newsletter <beehiiv-post-url>` or `/crosspost-newsletter latest`

## Process

### Phase 1 — Fetch Full Newsletter Content

**If URL provided:**
Use `WebFetch` with the beehiiv post URL to get metadata (title, subtitle, date).

**IMPORTANT: beehiiv renders article content dynamically.** WebFetch and the RSS feed may return empty blockquotes and miss dynamically-loaded content. To get the complete article:

1. First, fetch the RSS feed via `WebFetch`: `https://rss.beehiiv.com/feeds/9AbhG8CTgD.xml` to get the article HTML body. This gives you most text, headings, and links, but blockquotes may be empty and images may use beehiiv CDN URLs.

2. Then, use the browser to extract blockquotes and verify images from the rendered page:
   ```bash
   $B goto <beehiiv-post-url>
   $B js "
     const quotes = [...document.querySelectorAll('blockquote, .blockquote, [class*=blockquote]')]
       .map(q => q.textContent.trim()).filter(q => q.length > 0);
     const imgs = [...document.querySelectorAll('img')]
       .filter(i => i.src.includes('beehiiv') && i.naturalWidth > 200)
       .map(i => ({ src: i.src, alt: i.alt }));
     JSON.stringify({ quotes, imgs }, null, 2);
   "
   ```

3. Merge the blockquote text into the RSS HTML body, replacing empty `<blockquote>` tags.

4. Download all article images locally for upload:
   ```bash
   curl -sL "<image-url>" -o /tmp/image-name.jpg
   ```
   Images must be uploaded separately per platform — they cannot be pasted via HTML.

**If "latest" or no URL:**
Fetch the RSS feed, list recent articles, ask the user which one, then follow the above process.

**Content preparation:**
1. **Strip beehiiv boilerplate** — remove tracking pixels, analytics images, newsletter signup forms, footer, "View in browser" links, beehiiv-specific CSS classes/inline styles, UTM parameters from links
2. **Keep semantic HTML only** — h1-h6, p, strong, em, a, ul/ol/li, blockquote, pre/code (do NOT include img tags — images are uploaded separately)
3. **Save clean HTML to a temp file** — `/tmp/article-body.html`
4. **Escape for JS embedding** — when loading from file into `$B js`, escape backticks and `${` sequences:
   ```bash
   ARTICLE_HTML=$(cat /tmp/article-body.html | sed "s/\`/\\\\\`/g" | sed 's/\$/\\$/g')
   ```

Present to the user:
```
Article: "<Title>"
Subtitle: "<subtitle>"
Published: <date>
Length: ~<word count> words, <image count> images, <quote count> blockquotes
URL: <beehiiv URL>
```

### Phase 2 — Platform Selection

Ask the user which platforms to cross-post to:
- **A)** LinkedIn (native long-form article)
- **B)** Substack
- **C)** Medium (note: may be blocked by Cloudflare)
- **D)** All three

**Wait for user input before proceeding.**

Platforms are processed one at a time, sequentially.

### Phase 3 — Browser Setup & Authentication

#### 3a. Initialize the browse binary

```bash
B=~/.claude/skills/gstack/browse/dist/browse
if [ -x "$B" ]; then echo "READY"; else echo "NEEDS_SETUP"; fi
```

#### 3b. Verify authentication for each selected platform

Navigate to a page that reveals login state and take a snapshot:

**LinkedIn:** `$B goto https://www.linkedin.com/feed/` — logged in = feed with search bar. Not logged in = login form with "Email or phone" field.

**Substack:** `$B goto https://substack.com/account/settings` — logged in = shows Home/Subscriptions/Dashboard buttons. Not logged in = email input and "Continue" button.

**Medium:** `$B goto https://medium.com/me/stories` — logged in = stories dashboard. Not logged in = login page. **WARNING: Medium aggressively blocks headless browsers with Cloudflare (403). Cookie import typically does not help. Expect to skip Medium or handoff for manual login.**

#### 3c. Handle authentication failures

For any platform where the user is NOT logged in:

1. Attempt cookie import (note: the syntax is `browser domain`, not `--domain`):
   ```bash
   $B cookie-import-browser chrome linkedin.com
   ```
   This opens a cookie picker UI. Tell the user to select the domain and close the picker.

2. Re-navigate to the check page and snapshot again.

3. If still not logged in, handoff:
   ```bash
   $B handoff "Please log in to <platform> — I'll continue once you're done."
   ```

4. After user confirms, `$B resume` and verify. If still not logged in, skip this platform.

### Phase 4 — Cross-Post to Each Platform

Complete one platform fully (through publish) before starting the next.

---

#### Platform: LinkedIn (Native Article)

**Step 1 — Navigate to article editor:**
```bash
$B goto https://www.linkedin.com/article/new/
$B snapshot -i
```
Look for `[textbox] "Title"` and `[textbox] "Article editor content"` in the snapshot. Also note `[button] "Upload from computer"` for the cover image.

**Step 2 — Fill the title:**
```bash
$B click @eN  # the Title textbox
$B type "<article title>"
```
Note: the command is `type`, not `type_text`.

**Step 3 — Insert the article body (text only, no images):**
Click the "Article editor content" textbox, then inject via clipboard paste:
```bash
$B click @eN  # Article editor content textbox
$B js "
  const html = `$ARTICLE_HTML`;
  const dt = new DataTransfer();
  dt.setData('text/html', html);
  dt.setData('text/plain', html.replace(/<[^>]*>/g, ''));
  const editors = [...document.querySelectorAll('[contenteditable=\"true\"]')];
  const editor = editors.find(el => el.getAttribute('aria-label')?.includes('Article'));
  if (editor) {
    editor.focus();
    const evt = new ClipboardEvent('paste', { clipboardData: dt, bubbles: true, cancelable: true });
    editor.dispatchEvent(evt);
    'PASTE_OK';
  } else { 'EDITOR_NOT_FOUND'; }
"
```
Verify with `$B screenshot`. Text, headings, links, and blockquotes should appear. Images will NOT be in the paste — they must be uploaded separately.

**Step 4 — Upload images inline:**

LinkedIn has a toolbar with icon buttons. Identify the image button:
```bash
$B js "
  const buttons = [...document.querySelectorAll('.scaffold-formatted-text-editor-icon-button')];
  buttons.map((b, i) => {
    const svg = b.querySelector('svg use');
    return { index: i, href: svg?.getAttribute('href') };
  });
"
```
The image button has `href: '#image-medium'` (typically index 9).

**IMPORTANT — Cover image behavior:** If no cover image is set, the first image upload via the toolbar button will go to the cover image slot instead of inline. The cover image creates a persistent modal overlay that blocks the editor. To handle this:

1. First, position cursor in the body where the first image should go.
2. Click the image toolbar button via JS: `buttons[9].click()`
3. Find and upload to the file input:
   ```bash
   $B upload "#media-editor-file-selector__file-input" /tmp/image.jpg
   ```
4. If it goes to the cover slot (you'll see `[button] "Dismiss"`, `[button] "Edit"`, `[button] "Delete"` in snapshot), **handoff to the user** to dismiss the cover overlay by clicking on the article body. The cover modal is truly persistent and cannot be escaped programmatically.
5. After handoff/resume, with a cover now set, subsequent image uploads via the toolbar button will go inline.

For each inline image:
1. Position cursor at the correct location using JS:
   ```bash
   $B js "
     const editor = document.querySelector('[aria-label=\"Article editor content\"]');
     const paras = [...editor.querySelectorAll('p')];
     const targetP = paras.find(p => p.textContent.includes('<unique text near image>'));
     if (targetP) {
       const range = document.createRange();
       range.setStartAfter(targetP);
       range.collapse(true);
       const sel = window.getSelection();
       sel.removeAllRanges();
       sel.addRange(range);
       'CURSOR_SET';
     }
   "
   ```
2. Click the image toolbar button: `buttons[9].click()`
3. Upload: `$B upload "#media-editor-file-selector__file-input" /tmp/image.jpg`
4. An image overlay will appear — handoff to user to click the article body to dismiss it.

**Step 5 — Canonical URL:**
LinkedIn does NOT have a canonical URL field in the article editor UI. Skip this step.

**Step 6 — Publish:**
Click "Next" button to go to publish confirmation. The publish page shows:
- A text field for an accompanying post (`[textbox] "Text editor for creating content"`)
- A "Publish" button

Write a value/impact-framed accompanying post in the text field, then click Publish:
```bash
$B click @eN  # text field
$B type "<accompanying post text>"
$B click @eN  # Publish button
$B url  # capture published URL
```
The published URL will be in format: `https://www.linkedin.com/pulse/<slug>/`

---

#### Platform: Substack

**Step 1 — Determine Substack subdomain:**
```bash
$B goto https://substack.com/account/settings
$B click @eN  # Dashboard button
$B url  # URL contains subdomain, e.g. enterprisevibecode.substack.com
```

**Step 2 — Navigate to post editor:**
```bash
$B goto https://<subdomain>.substack.com/publish/post
$B snapshot -i
```
You'll see: `[textbox] "title"`, `[textbox] "Add a subtitle…"`, toolbar buttons including `[button] "Image"`, `[button] "Email header / footer"`, and `[button] "Continue"`.

**Step 3 — Fill the title and subtitle:**
**IMPORTANT: Substack has duplicate title/subtitle fields** — a sidebar pair and a main editor pair. The visible editor fields are `<textarea>` elements. Use JS to fill the correct ones:
```bash
$B js "
  const textareas = [...document.querySelectorAll('textarea')];
  // Find by placeholder text
  const title = textareas.find(t => t.placeholder === 'Title');
  const subtitle = textareas.find(t => t.placeholder.includes('subtitle'));
  if (title) { title.focus(); title.value = '<article title>'; title.dispatchEvent(new Event('input', { bubbles: true })); }
  if (subtitle) { subtitle.focus(); subtitle.value = '<article subtitle>'; subtitle.dispatchEvent(new Event('input', { bubbles: true })); }
  'FILLED';
"
```

**Step 4 — Enable email header/footer:**
Click the "Email header / footer" button to toggle it on:
```bash
$B click @eN  # "Email header / footer" button
```

**Step 5 — Insert the article body (text only, no images):**
Substack uses ProseMirror. Clipboard paste preserves text, headings, links, and blockquotes but **strips `<img>` tags**:
```bash
$B js "
  const html = `$ARTICLE_HTML`;
  const dt = new DataTransfer();
  dt.setData('text/html', html);
  dt.setData('text/plain', html.replace(/<[^>]*>/g, ''));
  const editor = document.querySelector('.ProseMirror[contenteditable=\"true\"]');
  if (editor) {
    editor.focus();
    const evt = new ClipboardEvent('paste', { clipboardData: dt, bubbles: true, cancelable: true });
    editor.dispatchEvent(evt);
    'PASTE_OK';
  } else { 'EDITOR_NOT_FOUND'; }
"
```

**Step 6 — Upload images inline:**

For each image, position cursor and use the toolbar Image button:

1. Position cursor at the correct location (same JS approach as LinkedIn).
2. Press Enter to create a new empty line: `$B press Enter`
3. Click the toolbar "Image" button (`@eN`). A dropdown menu appears with menuitems: "Image", "Gallery", "Stock photos", "Generate image".
4. Click the "Image" menuitem via JS (direct ref click may fail with "matched multiple elements"):
   ```bash
   $B js "
     const items = [...document.querySelectorAll('[role=\"menuitem\"]')];
     const imageItem = items.find(i => i.textContent.trim() === 'Image');
     if (imageItem) { imageItem.click(); 'CLICKED'; }
   "
   ```
5. This creates new file inputs. Find and upload to the one with `accept="image/*,.heic,.heif"`:
   ```bash
   $B js "
     const inputs = [...document.querySelectorAll('input[type=\"file\"]')];
     const newInput = inputs.find(i => i.accept && i.accept.includes('.heic'));
     if (newInput) { newInput.setAttribute('data-img-upload', 'true'); 'MARKED'; }
   "
   $B upload "[data-img-upload='true']" /tmp/image.jpg
   ```
6. Wait 3 seconds for upload, then verify the image is in the editor DOM:
   ```bash
   $B js "
     const editor = document.querySelector('.ProseMirror[contenteditable=\"true\"]');
     const imgs = editor ? [...editor.querySelectorAll('img')] : [];
     imgs.length;
   "
   ```
7. For subsequent images, the `data-img-upload` attribute may already exist. Use a unique attribute each time or find the latest `.heic` input.

**Step 7 — Canonical URL:**
Substack does NOT have a canonical URL field in the post settings UI. Skip this step.

**Step 8 — Publish:**
1. Click "Continue" button. A publish dialog appears with:
   - Audience: "Everyone" (checked)
   - `[checkbox] "Send via email and the Substack app"` — **KEEP CHECKED**
   - "Send to everyone now" button
2. Click "Send to everyone now".
3. A "Add subscribe buttons to your post" dialog may appear — click "Publish without buttons" or "Add subscribe buttons" per user preference.
4. Capture the published URL from the share page.

---

#### Platform: Medium

**Medium blocks headless browsers (gstack browse) via Cloudflare.** Use the Claude in Chrome extension (`mcp__claude-in-chrome__*` tools) instead, which operates through the user's real Chrome browser.

**Step 1 — Set up Claude in Chrome tab:**
```
mcp__claude-in-chrome__tabs_context_mcp (createIfEmpty: true)
mcp__claude-in-chrome__tabs_create_mcp  # if needed
```

**Step 2 — Navigate to story editor:**
```
mcp__claude-in-chrome__navigate (url: "https://medium.com/new-story", tabId: <tabId>)
```
If not logged in, the user must log in manually in Chrome — Medium sessions are in the real browser.

**Step 3 — Fill the title:**
Use `read_page` with `filter: "interactive"` to find the textbox element. Click it and type:
```
mcp__claude-in-chrome__computer (action: "left_click", ref: "<textbox ref>")
mcp__claude-in-chrome__computer (action: "type", text: "<article title>")
mcp__claude-in-chrome__computer (action: "key", text: "Enter")
```

**Step 4 — Insert the article body:**
Use `javascript_tool` to clipboard-paste HTML. Medium's editor accepts rich paste well — headings, blockquotes, links, bold, code all preserved:
```javascript
// mcp__claude-in-chrome__javascript_tool
const html = `<ARTICLE_HTML>`;
const dt = new DataTransfer();
dt.setData('text/html', html);
dt.setData('text/plain', html.replace(/<[^>]*>/g, ''));
const editors = [...document.querySelectorAll('[contenteditable="true"]')];
const editor = editors.find(el => el.getBoundingClientRect().height > 50);
if (editor) {
  editor.focus();
  const sel = window.getSelection();
  const range = document.createRange();
  range.selectNodeContents(editor);
  range.collapse(false);
  sel.removeAllRanges();
  sel.addRange(range);
  const evt = new ClipboardEvent('paste', { clipboardData: dt, bubbles: true, cancelable: true });
  editor.dispatchEvent(evt);
  'PASTE_OK';
}
```

**IMPORTANT — Quote formatting:** Medium's editor does NOT support `<br>` inside blockquotes for post-paste editing (causes "cannot save your story" errors). If you need authors on separate lines within blockquotes, either:
- Include `<br>` before the author attribution in the initial paste HTML (e.g. `<blockquote>"Quote text"<br>— Author</blockquote>`) — this works during initial paste
- Or have the user manually edit quotes after paste via handoff

Do NOT use Shift+Enter or direct DOM manipulation on blockquotes after paste — it breaks Medium's save mechanism.

**Step 5 — Upload images inline:**
For each image, position cursor after the correct paragraph, then:
1. Click the "+" button that appears on empty lines to reveal the toolbar
2. Click the image icon (first icon in the toolbar)
3. This opens a native file dialog and creates a file input. Upload via JS:
```javascript
// mcp__claude-in-chrome__javascript_tool
new Promise((resolve) => {
  const img = new Image();
  img.crossOrigin = 'anonymous';
  img.onload = () => {
    const canvas = document.createElement('canvas');
    const maxW = 800;
    const scale = Math.min(1, maxW / img.naturalWidth);
    canvas.width = img.naturalWidth * scale;
    canvas.height = img.naturalHeight * scale;
    canvas.getContext('2d').drawImage(img, 0, 0, canvas.width, canvas.height);
    canvas.toBlob((blob) => {
      const file = new File([blob], 'image.jpg', { type: 'image/jpeg' });
      const input = document.querySelector('input[type="file"]');
      const dt = new DataTransfer();
      dt.items.add(file);
      input.files = dt.files;
      input.dispatchEvent(new Event('change', { bubbles: true }));
      resolve('UPLOADED: ' + file.size + ' bytes');
    }, 'image/jpeg', 0.8);
  };
  img.onerror = () => resolve('IMG_LOAD_FAILED');
  img.src = '<IMAGE_URL>';
});
```
Note: Direct `fetch()` is CORS-blocked on Medium. The Image+canvas approach works because `crossOrigin = 'anonymous'` allows cross-origin image loading for canvas rendering. Keep images under 800px width to avoid timeouts.

**Step 6 — Set canonical URL:**
Navigate to story settings via the "..." menu → "More settings" → "Advanced Settings":
1. `mcp__claude-in-chrome__computer (action: "left_click")` on the "..." button (top-right of editor)
2. Click "More settings" in the dropdown
3. Click "Advanced Settings" in the left nav
4. Check the "This story was originally published elsewhere" checkbox via JS:
```javascript
const checkboxes = [...document.querySelectorAll('input[type="checkbox"]')];
const canonical = checkboxes.find(c => c.labels?.[0]?.textContent?.includes('originally published'));
if (canonical) { canonical.click(); 'CHECKED: ' + canonical.checked; }
```
5. Click "Edit canonical link", clear the field, type the beehiiv URL
6. Click "Save canonical link"
7. Navigate back to the editor

**Step 7 — Publish:**
1. Click "Publish" button (top-right)
2. Medium shows a publish confirmation page with:
   - Story preview (title, description, preview image)
   - Topics (optional)
   - Publication (optional — "Submit your story to connect with community")
   - `[checkbox] "Notify your N subscribers"` — **KEEP CHECKED**
   - "Publish" button and "Schedule for later" link
3. Click the final "Publish" button
4. Confirmation dialog: "Your story has been published and sent!"
5. Capture the published URL from the page

---

### Phase 5 — User Review (per platform)

After completing content injection for each platform, show a screenshot and ask:

```
<Platform> article is ready for review.

Title: "<Article Title>"
Body: ~<word count> words injected
Images: <count> uploaded inline
Canonical URL: not available on this platform

Options:
A) Publish now
B) Let me review in browser first (handoff)
C) Skip this platform
D) Abort all remaining platforms
```

**Wait for user input.**

### Phase 6 — Publish

See platform-specific publish steps above (LinkedIn Step 6, Substack Step 8).

### Phase 7 — Summary

```
Cross-post complete!

| Platform  | Status    | URL                          | Email sent |
|-----------|-----------|------------------------------|------------|
| LinkedIn  | Published | <url>                        | N/A        |
| Substack  | Published | <url>                        | Yes        |
| Medium    | Skipped   | Cloudflare blocked           | —          |
```

## Known Issues & Workarounds

### LinkedIn cover image modal
After uploading an image, LinkedIn shows a persistent modal overlay with "Dismiss", "Edit", "Delete" buttons. Clicking "Dismiss" triggers a discard confirmation. This modal blocks all editor interaction. **Workaround:** handoff to user to click the article body below the overlay.

### Substack strips images on paste
ProseMirror clipboard paste preserves text formatting but strips `<img>` tags. **Workaround:** upload images separately via the toolbar Image button after pasting text.

### Substack duplicate title fields
The editor has both sidebar metadata fields and visible textarea fields. `$B fill` may target the wrong one. **Workaround:** use JS to find textareas by placeholder text and set `.value` directly.

### Medium requires Claude in Chrome
Medium returns HTTP 403 from Cloudflare for headless Chromium browsers (gstack browse). User agent spoofing does not reliably work — may get through initially but gets blocked on subsequent page loads. **Workaround:** use Claude in Chrome extension (`mcp__claude-in-chrome__*` tools) which operates through the user's real Chrome browser and bypasses Cloudflare entirely.

### Medium blockquote line breaks
Medium's editor does NOT support post-paste editing of blockquotes with Shift+Enter or direct DOM manipulation — both cause persistent "Something is wrong and we cannot save your story" errors that prevent saving and publishing. **Workaround:** include `<br>` tags in the initial paste HTML before the author attribution. If the initial paste doesn't preserve the line breaks, have the user manually edit quotes via handoff.

### Medium image upload via JS
Direct `fetch()` is CORS-blocked on Medium's domain. **Workaround:** use `new Image()` with `crossOrigin = 'anonymous'`, draw to canvas, then create a File blob from `canvas.toBlob()`. Set the file on the `input[type="file"][name="uploadedFile"]` element and dispatch a `change` event. Keep images under 800px width to avoid JS execution timeouts.

### Medium canonical URL
Available under Story Settings → Advanced Settings → "Customize Canonical Link" → check "This story was originally published elsewhere". Use JS `click()` on the checkbox (not `form_input`) as the UI checkbox can be finicky. After checking, the "Edit canonical link" button reveals a URL input field.

### gstack browse command reference
- `$B type` — type text into focused element (NOT `type_text`)
- `$B fill @ref "text"` — fill a specific input
- `$B click @ref` — click an element
- `$B press Enter` — press a key
- `$B upload "selector" /path/to/file` — upload a file
- `$B js "code"` — execute JavaScript
- `$B snapshot -i` — get interactive elements with @e refs
- `$B screenshot /path.png` — capture screenshot
- `$B handoff "message"` — hand control to user
- `$B resume` — resume after handoff
- `$B cookie-import-browser chrome domain.com` — import cookies (syntax: browser then domain, no `--domain` flag)

### Claude in Chrome tool reference
- `mcp__claude-in-chrome__tabs_context_mcp` — get available tabs (call first)
- `mcp__claude-in-chrome__tabs_create_mcp` — create new tab
- `mcp__claude-in-chrome__navigate` — navigate to URL
- `mcp__claude-in-chrome__read_page` — get accessibility tree (use `filter: "interactive"` for buttons/inputs)
- `mcp__claude-in-chrome__find` — find elements by natural language
- `mcp__claude-in-chrome__computer` — mouse/keyboard actions and screenshots
- `mcp__claude-in-chrome__javascript_tool` — execute JS in page context (no top-level `await` — wrap in async IIFE or use Promises)
- `mcp__claude-in-chrome__form_input` — set form values by ref
