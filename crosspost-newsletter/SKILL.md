---
name: crosspost-newsletter
description: Use when user wants to cross-post a beehiiv newsletter article on LinkedIn, Substack, Medium, Hacker News, or Reddit using browser automation — "crosspost this article", "cross-post newsletter", "publish to Medium", "post to hacker news", "submit to reddit", "share on HN", "crosspost to substack", "syndicate newsletter".
user_invocable: true
---

# crosspost-newsletter

Cross-post a beehiiv newsletter article across five platforms in two modes:
- **Full-article syndication** to LinkedIn (native article), Substack, and Medium — preserves rich formatting, headings, images, and sets canonical URL back to the beehiiv original. Uses gstack browse for LinkedIn/Substack, Claude in Chrome for Medium (which blocks headless browsers).
- **Link submission** to Hacker News and Reddit — submits the beehiiv URL with the article title. Uses Claude in Chrome for both (neither platform has a workable public posting API). For Reddit, submits to one or more subreddits chosen by the user.

If a platform offers the option to send the article as an email to subscribers, always enable it.

## Usage

`/crosspost-newsletter <beehiiv-post-url>` or `/crosspost-newsletter latest`

## Process

### Phase 1 — Fetch Full Newsletter Content

**If URL provided:**
Use `WebFetch` with the beehiiv post URL to get metadata (title, subtitle, date).

**IMPORTANT: beehiiv renders article content dynamically.** WebFetch and the RSS feed may return empty blockquotes and miss dynamically-loaded content. To get the complete article:

1. First, fetch the RSS feed via `WebFetch`: `https://rss.beehiiv.com/feeds/9AbhG8CTgD.xml` to get the article HTML body. This gives you most text, headings, and links, but blockquotes may be empty and images may use beehiiv CDN URLs.

2. Then, use the browser to extract blockquotes, cover image, and body images + captions from the rendered page. beehiiv renders content client-side in these DOM structures (current as of 2026):
   - `.dream-post-content-doc` — article body root
   - `.dream-post-content-paragraph` — paragraph text (inside a `div.j6zgbu0` wrapper)
   - `.dream-post-content-imageBlock` — image with optional caption in a `figcaption`
   - Section headings in divs whose class starts with `hynlcx`
   - Cover/hero image is NOT inside the body — it's in `<meta property="og:image">`

   ```bash
   $B goto <beehiiv-post-url>
   $B js "
     const quotes = [...document.querySelectorAll('blockquote, [class*=blockquote]')]
       .map(q => q.textContent.trim().replace(/^❝/, '').trim())
       .filter(q => q.length > 0);

     // Cover image is separate — from og:image meta tag
     const coverImageUrl = document.querySelector('meta[property=\"og:image\"]')?.content;

     // Body images with captions, in article order
     const imageBlocks = [...document.querySelectorAll('.dream-post-content-imageBlock')];
     const bodyImages = imageBlocks.map(block => {
       const img = block.querySelector('img');
       const cap = block.querySelector('figcaption, [class*=caption]');
       return { src: img?.src, caption: cap?.textContent?.trim() || '' };
     });

     JSON.stringify({ quotes, coverImageUrl, bodyImages }, null, 2);
   "
   ```

3. Merge the blockquote text into the RSS HTML body, replacing empty `<blockquote>` tags.

4. Download cover + body images locally (cover and body are separate):
   ```bash
   # Cover image (for LinkedIn cover slot, Substack/Medium cover where applicable)
   curl -sL "<coverImageUrl>" -o /tmp/bb-cover.png

   # Body images in article order (these get inserted inline)
   curl -sL "<bodyImages[0].src>" -o /tmp/bb-img1.jpg
   curl -sL "<bodyImages[1].src>" -o /tmp/bb-img2.jpg
   # ...
   ```
   Images must be uploaded separately per platform — they cannot be pasted via HTML.
   **Preserve each body image's caption** for Step 4d of each platform's inline-image flow.

**If "latest" or no URL:**
Fetch the RSS feed, list recent articles, ask the user which one, then follow the above process.

**Content preparation:**
1. **Strip beehiiv boilerplate** — remove tracking pixels, analytics images, newsletter signup forms, footer, "View in browser" links, beehiiv-specific CSS classes/inline styles, UTM parameters from links
2. **Keep semantic HTML only** — h1-h6, p, strong, em, a, ul/ol/li, blockquote, pre/code (do NOT include img tags — images are uploaded separately)
3. **PRESERVE EXACT ELEMENT ORDER from source** — walk the beehiiv DOM (`.dream-post-content-doc` children) in document order and emit elements in the same order. Do NOT reorder, group, or guess positions. Blockquotes, footnotes, and mid-article callouts are easy to misplace when hand-constructing the HTML — the Sun Tzu blockquote in "3 Lessons in AI Agents from a Black Belt" belongs as the VERY LAST element after both footnotes, not before them. Use this extraction JS to get everything in correct order:
   ```bash
   $B js "
     const body = document.querySelector('.dream-post-content-doc');
     const parts = [];
     for (const wrap of body.children) {
       const child = wrap.firstElementChild;
       if (!child) continue;
       const cls = child.getAttribute('class') || '';
       const wrapCls = wrap.className || '';
       if (cls.includes('paragraph')) parts.push('<p>' + child.innerHTML + '</p>');
       else if (cls.startsWith('hynlcx')) parts.push('<h2>' + child.textContent.trim() + '</h2>');
       else if (wrapCls.includes('imageBlock')) {
         const img = wrap.querySelector('img');
         if (img) parts.push('IMG::' + img.src);
       } else if (wrapCls.includes('blockquote') || cls.includes('blockquote')) {
         parts.push('<blockquote>' + wrap.textContent.trim().replace(/^❝/, '').trim() + '</blockquote>');
       }
     }
     parts.join('\\n');
   "
   ```
   Then hand-clean the output (strip beehiiv-specific spans, fix blockquote attribution, replace `IMG::` placeholders with empty lines that document where each image goes). Do not introduce any new elements or reorder what the extractor produced.
4. **Verify element order matches source** — before saving `/tmp/article-body.html`, run a sanity check:
   - Count `<blockquote>` tags — should match beehiiv's count
   - Count `<h2>` tags — should match the number of section headings in the source
   - Count `<p>` tags — should match (±1-2 for empty/spacer paragraphs)
   - Check first and last paragraph text against the source
   - For each blockquote: confirm the paragraph immediately before it (in your HTML) matches the paragraph immediately before it in beehiiv. This catches misplaced blockquotes before they get pasted into every platform.
5. **Format blockquote attributions** — the quote author/attribution must always be on a separate line below the quote text. In the HTML, place a `<br>` before the em dash and author name inside each `<blockquote>`:
   ```html
   <blockquote>"Quote text here."<br>— Author Name</blockquote>
   ```
   This formatting must be done in the initial paste HTML for all platforms. Post-paste editing of blockquotes causes save errors on Medium, and LinkedIn's editor is similarly restrictive. Getting it right in the initial paste is the only reliable approach.
6. **Save clean HTML to a temp file** — `/tmp/article-body.html`
7. **Escape for JS embedding** — when loading from file into `$B js`, escape backticks and `${` sequences:
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

Ask the user which platforms to cross-post to (**multi-select** — any combination is valid):

**Full-article syndication (paste entire body with formatting + images):**
- **A)** LinkedIn — native long-form article (gstack browse)
- **B)** Substack — full post (gstack browse)
- **C)** Medium — full story (Claude in Chrome; bypasses Cloudflare)

**Link submission (title + URL only, no article body):**
- **D)** Hacker News — submit to `news.ycombinator.com` (Claude in Chrome)
- **E)** Reddit — submit as a Link post to one or more subreddits (Claude in Chrome)

**Wait for user input before proceeding.**

Platforms are processed one at a time, sequentially. Full-article platforms run first (they need the article body prepared in Phase 1), then link submissions.

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

**IMPORTANT — heading conversion:** LinkedIn's editor converts pasted `<h2>` tags to `<h3>`. When subsequent steps query headings (to anchor figure positioning), use `h3` (or the wildcard `h1,h2,h3,h4,h5,h6`) — do not assume the tag matches your source HTML.

**Step 4 — Set cover image + upload and position body images:**

The reliable pattern is: (a) set the cover image first, (b) batch-upload all body images to wherever LinkedIn drops them, (c) move each `<figure>` to its target heading via DOM, (d) set captions.

**The image toolbar ignores cursor position.** Setting `range.setStartAfter(heading)` before clicking the toolbar has no effect — images land at a cached/default position regardless. Don't bother setting the cursor; fix placement by moving figures after upload.

**Step 4a — Set the cover image (separate from body images):**

The cover image is the article's hero image from beehiiv's `og:image` meta (downloaded as `/tmp/bb-cover.png` in Phase 1). The LinkedIn cover slot is the `.article-editor-cover-media` element at the top of the editor.

Scroll to top and check cover state:
```bash
$B js "
  window.scrollTo(0, 0);
  const cover = document.querySelector('.article-editor-cover-media');
  ({
    hasImg: !!cover?.querySelector('img'),
    placeholderVisible: !!cover?.querySelector('.article-editor-cover-media__placeholder')
  });
"
```

- If placeholder is visible: click the "Upload from computer" button inside the placeholder (from the initial snapshot, typically `[button] "Upload from computer"`).
- If the cover already has the wrong image (e.g. you want to replace it): click the "Delete" button inside the cover (aria-label contains "remove the cover image"). Then the placeholder returns — click "Upload from computer".

Upload:
```bash
$B upload "#media-editor-file-selector__file-input" /tmp/bb-cover.png
```

The cover modal overlay appears (`[button] "Dismiss"`, `[button] "Edit"`, `[button] "Delete"`). **Handoff to user** to click the article body to dismiss it — the modal cannot be escaped programmatically.

**Step 4b — Identify the body image toolbar button (once):**
```bash
$B js "
  const buttons = [...document.querySelectorAll('.scaffold-formatted-text-editor-icon-button')];
  buttons.map((b, i) => {
    const svg = b.querySelector('svg use');
    return { index: i, href: svg?.getAttribute('href') };
  }).filter(x => x.href && x.href.includes('image'));
"
```
The image button has `href: '#image-medium'` (typically index 9).

**Step 4c — Batch upload all body images in article order:**
For each body image (in the order they appear in the article):
```bash
# Click the image toolbar button via JS (cursor position is ignored — don't try to set it)
$B js "
  const buttons = [...document.querySelectorAll('.scaffold-formatted-text-editor-icon-button')];
  buttons[9].click();
  'clicked';
"

# Upload the image
$B upload "#media-editor-file-selector__file-input" /tmp/bb-imgN.jpg

# Handoff to user to dismiss the image overlay by clicking the article body
$B handoff "Image N uploaded. Please click the article body to dismiss the overlay."
# (resume after user confirms)
```

After all body images are uploaded, they will all be clumped together in the wrong spot. That's fine — the next step fixes it.

**Step 4d — Move each figure to its correct heading via DOM:**

Walk the editor, find each heading, find each figure (in upload order), and use `parentNode.insertBefore` to place each figure right after its target heading:

```bash
$B js "
  const editor = document.querySelector('[aria-label=\"Article editor content\"]');
  const figures = [...editor.querySelectorAll('figure')];
  const headings = [...editor.querySelectorAll('h3')];

  // targets: pair each upload-order figure with its target heading text
  // (populate from Phase 1 bodyImages + section anchors you identified)
  const targets = [
    { headingText: 'The Black Belt', figIdx: 0 },
    { headingText: 'The Dream',      figIdx: 1 },
    { headingText: 'The PhD',        figIdx: 2 },
    { headingText: 'The Business License', figIdx: 3 }
  ];

  for (const { headingText, figIdx } of targets) {
    const h = headings.find(h => h.textContent.trim() === headingText);
    const fig = figures[figIdx];
    if (h && fig) h.parentNode.insertBefore(fig, h.nextSibling);
  }
  editor.dispatchEvent(new Event('input', { bubbles: true }));
  'MOVED ' + targets.length + ' figures';
"
```

**This DOM manipulation is safe on LinkedIn** — `<figure>` moves do not break the editor's save state. (This is different from Medium, where post-paste DOM edits break save.)

Verify with a follow-up query that walks the editor and reports heading+image order.

**Step 4e — Set image captions (from Phase 1 extraction):**

Each LinkedIn figure has a `<textarea class="article-editor-figure-caption">` for captions. React ignores direct `textarea.value = "..."` assignment — use the native setter pattern:

```bash
$B js "
  const figures = [...document.querySelectorAll('[aria-label=\"Article editor content\"] figure')];
  // captions in upload order; empty string = no caption
  const captions = [
    '',  // e.g. Black Belt ceremony — no caption in beehiiv
    'First (?) White Belt Tournament, Circa Early 2013',
    'Blue Belt, Circa Late 2013',
    'This is just an excuse for me to post all of my Jiu-Jitsu pictures where I look cool'
  ];
  let filled = 0;
  figures.forEach((fig, i) => {
    if (!captions[i]) return;
    const ta = fig.querySelector('textarea.article-editor-figure-caption');
    if (!ta) return;
    const setter = Object.getOwnPropertyDescriptor(window.HTMLTextAreaElement.prototype, 'value').set;
    setter.call(ta, captions[i]);
    ta.dispatchEvent(new Event('input', { bubbles: true }));
    ta.dispatchEvent(new Event('change', { bubbles: true }));
    filled++;
  });
  'FILLED ' + filled + ' captions';
"
```

Verify captions persist with:
```bash
$B js "[...document.querySelectorAll('[aria-label=\"Article editor content\"] figure')].map(f => f.querySelector('textarea.article-editor-figure-caption')?.value)"
```

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

#### Platform: Hacker News (Link Submission)

HN has no workable public posting API (the official Firebase API at `github.com/HackerNews/API` is entirely read-only). Use Claude in Chrome browser automation. The user must be logged in to `news.ycombinator.com` in their real Chrome browser.

**Content pattern:** title + URL only. HN link submissions do NOT support a text body — if URL is filled, the text field must be empty.

**Step 1 — Navigate to submit page:**
```
mcp__claude-in-chrome__navigate (url: "https://news.ycombinator.com/submit", tabId: <tabId>)
```

**Step 2 — Verify login:**
```
mcp__claude-in-chrome__read_page (tabId: <tabId>, filter: "interactive")
```
Logged in = page shows `textbox` elements named `title`, `url`, `text` and a `submit` button. Not logged in = "please log in" message with a login form. If not logged in, handoff to user:
```
mcp__claude-in-chrome__computer (action: "screenshot", tabId: <tabId>)
```
Ask the user to log in, then wait for confirmation and re-check.

**Step 3 — Prepare HN-appropriate title:**
Before submitting, adapt the title per HN culture:
- Trim to ≤80 characters (HN enforces this)
- Strip clickbait prefixes ("How to", "You won't believe", "N things that...")
- Remove editorial framing — HN prefers descriptive titles
- **Do NOT add "Show HN:" prefix** unless the article is a genuine demo of original work (misusing this gets the submission killed)

Present the adapted title to the user for approval before proceeding.

**Step 4 — Fill title and URL:**
```
mcp__claude-in-chrome__find (tabId: <tabId>, query: "title textbox")
mcp__claude-in-chrome__computer (action: "left_click", ref: <title ref>)
mcp__claude-in-chrome__computer (action: "type", text: "<HN-adapted title>")

mcp__claude-in-chrome__find (tabId: <tabId>, query: "url textbox")
mcp__claude-in-chrome__computer (action: "left_click", ref: <url ref>)
mcp__claude-in-chrome__computer (action: "type", text: "<beehiiv URL>")
```
Leave the **text** field empty — HN rejects submissions that have both a URL and text.

**Step 5 — Screenshot for review:**
```
mcp__claude-in-chrome__computer (action: "screenshot", tabId: <tabId>)
```
Show screenshot to user (Phase 5).

**Step 6 — Submit:**
```
mcp__claude-in-chrome__find (tabId: <tabId>, query: "submit button")
mcp__claude-in-chrome__computer (action: "left_click", ref: <submit ref>)
```

**Step 7 — Capture result:**
After submit, the URL changes to `news.ycombinator.com/item?id=<N>`. If HN redirects to a rate-limit warning ("You're posting too fast") or a validation error, capture that instead and report to the user. Also watch for the "[dead]" state — HN silently kills flagged submissions, which appear successful but never show on `/newest`.

---

#### Platform: Reddit (Link Submission)

Reddit has an OAuth2 write API, but it requires creating a Reddit app and managing tokens. Browser automation via Claude in Chrome is simpler and consistent with the rest of the skill. The user must be logged in to `reddit.com` in their real Chrome browser.

**Step 1 — Select subreddits:**

Present the default subreddit list and let the user multi-select (with an option to add custom subs):

Default list:
- `r/programming`
- `r/webdev`
- `r/ClaudeAI`
- `r/ArtificialIntelligence`
- `r/artificial`
- `r/LocalLLaMA`
- `r/SaaS`
- `r/Entrepreneur`
- `r/devops`

If the user selects more than 5 subs, warn them about Reddit's 9:1 self-promotion rule and confirm.

**Step 2 — For each selected subreddit, repeat Steps 3–8:**

**Step 3 — Navigate to submit page:**
```
mcp__claude-in-chrome__navigate (url: "https://www.reddit.com/r/<subreddit>/submit", tabId: <tabId>)
```

**Step 4 — Verify login:**
```
mcp__claude-in-chrome__read_page (tabId: <tabId>, filter: "interactive")
```
Logged in = page shows "Post", "Images & Video", "Link", "Poll" tabs and a title textbox. Not logged in = login prompt. Handoff if not logged in.

**Step 5 — Select "Link" post type:**
Reddit's new UI defaults to text "Post". Click the "Link" tab:
```
mcp__claude-in-chrome__find (tabId: <tabId>, query: "Link tab in post type selector")
mcp__claude-in-chrome__computer (action: "left_click", ref: <Link tab ref>)
```

**Step 6 — Fill title and URL:**
```
mcp__claude-in-chrome__find (tabId: <tabId>, query: "Title textbox")
mcp__claude-in-chrome__computer (action: "left_click", ref: <title ref>)
mcp__claude-in-chrome__computer (action: "type", text: "<article title>")

mcp__claude-in-chrome__find (tabId: <tabId>, query: "Link URL textbox")
mcp__claude-in-chrome__computer (action: "left_click", ref: <url ref>)
mcp__claude-in-chrome__computer (action: "type", text: "<beehiiv URL>")
```

**Step 7 — Handle subreddit-specific required fields:**

- **Flair:** Check the page for a "Flair" or "Add flair" button. If required, present available flairs to the user and let them pick. Some subs won't allow Post without a flair.
- **Rules acknowledgment:** Some subs show a modal with rules that must be accepted.
- **Community questions:** Some subs (especially r/SaaS, r/Entrepreneur) ask additional questions. Handoff if detected.

Use `read_page` to detect these states and handle each as needed. If anything requires user judgment, handoff.

**Step 8 — Screenshot, review, and submit:**
```
mcp__claude-in-chrome__computer (action: "screenshot", tabId: <tabId>)
```
Show to user (Phase 5). On approval:
```
mcp__claude-in-chrome__find (tabId: <tabId>, query: "Post button")
mcp__claude-in-chrome__computer (action: "left_click", ref: <post ref>)
```

**Step 9 — CAPTCHA check:**
After clicking Post, screenshot the page. If a CAPTCHA appears, handoff:
```
mcp__claude-in-chrome__computer (action: "screenshot", tabId: <tabId>)
```
Ask the user to solve the CAPTCHA, then wait for them to click Post manually.

**Step 10 — Capture result URL and rate-limit pause:**
After successful submission, the URL changes to `reddit.com/r/<sub>/comments/<id>/<slug>/`. Capture it.

**Between subreddit submissions, wait at least 90 seconds** (Reddit anti-spam). Use:
```
mcp__claude-in-chrome__computer (action: "wait", duration: 90, tabId: <tabId>)
```
or inform the user and pause via handoff.

---

### Phase 5 — User Review (per platform)

After preparing each platform, show a screenshot and ask. The template differs by content mode:

**For full-article platforms (LinkedIn, Substack, Medium):**
```
<Platform> article is ready for review.

Title: "<Article Title>"
Body: ~<word count> words injected
Images: <count> uploaded inline
Canonical URL: <set | not available on this platform>

Options:
A) Publish now
B) Let me review in browser first (handoff)
C) Skip this platform
D) Abort all remaining platforms
```

**For link-submission platforms (Hacker News, Reddit):**
```
<Platform> submission ready for review.

Target: <hacker news | r/<subreddit>>
Title: "<title being submitted>"
URL:   <beehiiv URL>
<additional field notes, e.g. "Flair: <selected flair>" for Reddit>

Options:
A) Submit now
B) Let me review in browser first (handoff)
C) Skip this <platform | subreddit>
D) Abort all remaining submissions
```

**Wait for user input.**

### Phase 6 — Publish

See platform-specific publish steps above (LinkedIn Step 6, Substack Step 8).

### Phase 7 — Summary

One row per platform, with a `Target` column for Reddit (one row per subreddit):

```
Cross-post complete!

| Platform  | Target         | Status    | URL                                         |
|-----------|----------------|-----------|---------------------------------------------|
| LinkedIn  | —              | Published | https://www.linkedin.com/pulse/...          |
| Substack  | —              | Published | https://<sub>.substack.com/p/...            |
| Medium    | —              | Published | https://medium.com/p/...                    |
| HN        | —              | Submitted | https://news.ycombinator.com/item?id=...    |
| Reddit    | r/programming  | Submitted | https://reddit.com/r/programming/comments/… |
| Reddit    | r/ClaudeAI     | Submitted | https://reddit.com/r/ClaudeAI/comments/…    |
```

If any submissions were rate-limited, silently killed, or required CAPTCHA, note it in the status column.

## Known Issues & Workarounds

### Hand-constructed HTML drifts from source order
When manually composing `/tmp/article-body.html` from beehiiv extraction output, it is easy to misplace blockquotes, footnotes, and mid-article callouts — they end up in the wrong position relative to surrounding paragraphs. The Sun Tzu blockquote in "3 Lessons in AI Agents from a Black Belt" belongs as the LAST element after both footnotes, but was initially placed before footnote [1]. **Workaround:** walk the beehiiv DOM with the extractor JS in Phase 1 Step 3, preserve document order, and verify element counts + neighbor-check each blockquote before saving the HTML file. Because the HTML is pasted verbatim into every platform, any ordering mistake propagates to all of them.

### LinkedIn image toolbar ignores cursor position
Setting a selection range (e.g. `range.setStartAfter(heading)`) before clicking the image toolbar button has no effect — images consistently land at a cached/default position, regardless of where you put the cursor. **Workaround:** batch-upload all body images in order, accept that they'll all clump together in the wrong spot, then move each `<figure>` to its target heading via `parentNode.insertBefore(figure, heading.nextSibling)` followed by dispatching an `input` event on the editor.

### LinkedIn DOM figure moves are safe
Unlike Medium (where post-paste DOM manipulation breaks the editor's save state), LinkedIn's article editor handles moving `<figure>` elements cleanly. You can freely reorder figures with `parentNode.insertBefore` — the editor re-syncs on the `input` event and saves fine.

### LinkedIn cover image modal is persistent
After uploading an image (cover or inline), LinkedIn shows a persistent modal overlay with "Dismiss", "Edit", "Delete" buttons. Clicking "Dismiss" triggers a discard confirmation. This modal blocks all editor interaction. **Workaround:** handoff to user to click the article body below the overlay — the overlay dismisses on a body click but not programmatic focus.

### LinkedIn figure captions require React-native setter
Each figure has a `<textarea class="article-editor-figure-caption">` for captions. Direct `textarea.value = "..."` is silently reverted by React. **Workaround:** use the native setter:
```js
const setter = Object.getOwnPropertyDescriptor(window.HTMLTextAreaElement.prototype, 'value').set;
setter.call(ta, captionText);
ta.dispatchEvent(new Event('input', { bubbles: true }));
ta.dispatchEvent(new Event('change', { bubbles: true }));
```

### LinkedIn converts `<h2>` to `<h3>` on paste
LinkedIn's paste sanitizer downgrades all `<h2>` headings to `<h3>`. When anchoring operations to headings (moving figures, inserting anchors), query `h3` (or the wildcard `h1,h2,h3,h4,h5,h6`) — not the tag from your source HTML.

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

### Hacker News has no posting API
The official HN Firebase API (`github.com/HackerNews/API`) is entirely read-only — every endpoint is GET. There is no authenticated write access. **Workaround:** browser automation via Claude in Chrome against `news.ycombinator.com/submit`. User must be logged in in their real Chrome browser.

### Hacker News rate limits and shadow-killing
HN enforces strict per-user rate limits — submitting multiple stories quickly triggers "You're posting too fast." HN also silently kills ("[dead]") submissions flagged by their anti-spam filter; the submit looks successful but the story never appears on `/newest`. **Workaround:** submit one at a time, space submissions by at least 5 minutes, and check `/newest` after submitting to confirm the story is visible. If shadow-killed, the user must contact HN moderators — no automated recovery.

### Hacker News title rules
HN culture expects descriptive titles. No editorialization, no clickbait ("How to X", "You won't believe"), no "N things" listicles. Under 80 characters. "Show HN:" prefix is reserved for actual original work demos — misuse gets the submission killed. **Workaround:** adapt the beehiiv title for HN before submitting; present adapted title to the user for approval.

### Reddit has a write API but browser automation is simpler
Reddit's OAuth2 API supports submissions via PRAW but requires creating a Reddit app and managing tokens — a second auth flow. Browser automation via Claude in Chrome uses the user's existing Reddit login and is consistent with the rest of the skill. **Trade-off:** browser automation is slightly more fragile (UI changes) but needs no setup.

### Reddit self-promotion and rate limits
Reddit enforces a sitewide 9:1 rule — 9 community contributions for every 1 self-promotion. Posting the same newsletter to many subs in quick succession flags the account. **Workaround:** the skill enforces a 90-second delay between subreddit submissions. If the user selects >5 subs in a session, warn them first.

### Reddit subreddit-specific rules
Each sub has different rules: required flairs, karma minimums, self-promotion bans, mandatory community questions. The skill cannot enumerate every rule — it handles whatever the submit form presents (flair dropdowns, rule acknowledgment modals, community questions) and reports the error if a submission is rejected. **Workaround:** use `read_page` to detect required fields before submitting; handoff if community questions appear.

### Reddit CAPTCHA
Reddit occasionally shows a CAPTCHA on submit, especially for newer accounts or rapid posting. Claude in Chrome cannot solve CAPTCHAs. **Workaround:** screenshot after clicking Post, detect CAPTCHA visually, handoff to user with clear instructions, resume after they solve it.

### gstack browse command reference
- `$B goto <URL>` — navigate to URL (reports HTTP status, e.g. `Navigated to ... (200)`)
- `$B url` — print current tab's URL (useful for capturing the published article URL after publish)
- `$B type` — type text into focused element (NOT `type_text`)
- `$B fill @ref "text"` — fill a specific input
- `$B click @ref` — click an element
- `$B press Enter` — press a key
- `$B upload "selector" /path/to/file` — upload a file
- `$B js "code"` — execute JavaScript
- `$B snapshot -i` — get interactive elements with @e refs
- `$B screenshot /path.png` — capture screenshot
- `$B tab <n>` / `$B tabs` / `$B newtab` — switch/list/open browser tabs
- `$B viewport <WxH>` — resize viewport (e.g. `$B viewport 1280x900`)
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
