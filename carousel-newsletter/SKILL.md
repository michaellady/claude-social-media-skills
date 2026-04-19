---
name: carousel-newsletter
description: Use when user wants to promote a beehiiv newsletter as a 10-slide illustrated carousel to Buffer for Instagram, LinkedIn, Facebook, Threads — "carousel the newsletter", "swipe post", "newsletter carousel", "10-slide post". Generates on-brand EVC slides and schedules them with a "comment 'newsletter'" CTA.
user_invocable: true
---

# carousel-newsletter

Promote a beehiiv newsletter post as a **10-slide illustrated carousel** on every Buffer-connected channel (Instagram, LinkedIn, Facebook, Threads). The carousel summarizes the newsletter using **direct quotes** and drives a `comment "newsletter"` CTA that routes into the user's existing Comment-to-DM funnel.

This skill locks the visual system to the **Enterprise Vibe Code (EVC)** brand: cream bg + isometric lego/robot illustrations + navy/green/yellow/blue palette. Use the templates in `templates/` — do **not** invent new aesthetics per post.

## When to run

User says things like:
- "carousel my latest newsletter"
- "make a swipe post for the EVC article"
- "10-slide post of this newsletter"
- "post the newsletter as a carousel"

If the user wants a **single-image snippet post**, use `promote-newsletter` instead. If the user wants to **republish the full article** on LinkedIn/Medium/HN, use `crosspost-newsletter`.

## Prerequisites

- macOS with Google Chrome installed at `/Applications/Google Chrome.app` (required for headless rendering).
- `python3` on PATH (for placeholder substitution inside the renderer).
- `sips` on PATH (macOS built-in image tool; used to crop the render).
- A publicly accessible host for the rendered PNGs. **Default strategy:** commit the rendered images to this repo under `generated/<YYYY-MM-DD>-<slug>/slide-NN.png`, push to GitHub, and use `https://raw.githubusercontent.com/<owner>/<repo>/main/generated/<...>` URLs. Buffer's `create_post` requires publicly reachable image URLs — local paths will be rejected.

## Key files

- `templates/shared.css` — brand tokens (palette, typography, frame, pill, mark). Do not edit per post.
- `templates/illustrations.svg` — SVG sprite sheet (robot, minecart, rail, bricks, gears, wrench, EVC triangle mark, quote glyph, swipe chevron). Referenced inline by templates.
- `templates/01-hook.html` — cover slide. Big navy+green headline, hero scene (minecart+robot+bricks), swipe hint.
- `templates/02-section.html` — kicker + 2-line headline + body. Use for stage-setters, transitions, payoffs.
- `templates/03-quote.html` — oversized green open-quote + verbatim newsletter quote + attribution.
- `templates/04-stat.html` — giant number + short label. Unit renders green, number renders navy.
- `templates/05-cta.html` — final CTA slide with accent trigger word.
- `templates/render.sh` — HTML → 1080×1350 PNG. Inlines the sprite, compensates for Chrome's 87px window-chrome reservation, crops the result. Usage: `render.sh input.html output.png`.
- `examples/sample-deck/slide-01.png` … `slide-10.png` — reference deck rendered from a synthetic EVC post; use to sanity-check aesthetic drift.

## Related skills — read before diving in

- `../promote-newsletter/SKILL.md` — beehiiv RSS fetch pattern, Buffer channel filter (connected + unlocked), CTA copy, rate-limit/remaining-posts logic, per-platform metadata. **Reuse these patterns verbatim; do not reimplement.**
- `../crosspost-newsletter/SKILL.md` — richer beehiiv DOM extraction (blockquotes, captions, cover meta) if you need quote candidates beyond what RSS exposes.

## Workflow

### Phase 1 — Fetch the newsletter

Ask the user for the beehiiv post URL. If they say "latest" or don't specify, fetch the RSS feed and take item[0]:
- RSS: `https://rss.beehiiv.com/feeds/<pub-slug>.xml` (see `../promote-newsletter/SKILL.md` for the exact feed URL pattern currently in use)
- `WebFetch` the resolved post URL.

Extract:
- **title**
- **subtitle / dek** (if present)
- **section headings** (H2, H3) — becomes slide kickers + section headlines
- **body paragraphs** — source of insights
- **pull-quotes** (`<blockquote>`) — strongest candidates for Quote slides
- **stat-shaped phrases** — regex for `\d+%`, `\d+x`, `\$\d`, `\d+ (minutes|hours|days|weeks)`, or any large bolded number
- **hero image URL** (for reference only; we do not use it in the deck)

Save to `/tmp/carousel-<slug>/source.json` as a structured record so later phases don't refetch.

### Phase 2 — Draft the 10-slide script  ← USER REVIEW GATE

Produce **plain text**, no rendering. Fixed slide structure:

| # | Template | Purpose |
|---|---|---|
| 1 | `01-hook.html` | Title + 1-line tease. Pull the title verbatim; split into 2 lines where the break reads naturally. |
| 2 | `02-section.html` | Stage-setter. "Here's the real problem" / "Why this matters". ≤220 chars body. |
| 3 | `03-quote.html` | Strongest verbatim quote. |
| 4 | `04-stat.html` | A stat or big number from the article. If the article has no real stat, invent a directional one only if it's anchored in the article's thesis AND flag it for user approval. |
| 5 | `03-quote.html` | Second verbatim quote. |
| 6 | `02-section.html` | The "sharper frame" / key insight. |
| 7 | `03-quote.html` | Third verbatim quote. |
| 8 | `04-stat.html` OR `02-section.html` | Supporting point. Use stat if article has a second stat; otherwise another section slide. |
| 9 | `02-section.html` | The payoff / what to do about it. |
| 10 | `05-cta.html` | Fixed CTA — accent word **"newsletter"** (match existing Comment-to-DM trigger). |

**Quote rules**
- Verbatim from the newsletter. No paraphrasing. Include the opening and closing punctuation as it appears in the source.
- ≤260 chars. If a great quote is longer, trim from the ends only (ellipsis allowed); never rewrite internal words.
- Attribution is always `<Article Title> — Enterprise Vibe Code`.

**Hook rules**
- `{{LINE_1}}` + `{{LINE_2}}` together should mirror the article title, split where the eye naturally breaks. If the title is >2 display lines at 148px, pick a 2–6 word hook derived from the title, not the full title.
- `{{TEASE}}` ≤120 chars.

**CTA copy (slide 10, never change)**
- Headline: `Want the full article?`
- Accent word: `newsletter` (lowercase, with quotes in the template)
- Body: `Comment "newsletter" below and I'll DM you the full Enterprise Vibe Code post — free, every week.`

Surface the full script as a numbered list with template assignment and **stop for user approval**. The user can:
- Edit any slide's copy
- Swap a template (`03-quote` ↔ `04-stat` ↔ `02-section`)
- Drop a slide (rare — the deck is meant to be exactly 10 to match IG carousel cap)

Do not proceed to rendering until the user says "looks good" / "render it" / "ship it".

### Phase 3 — Render HTML → PNG

For each approved slide:

1. Copy the template into `/tmp/carousel-<slug>/slide-NN.html`
2. Substitute placeholders via Python (safe for arbitrary user text):
   ```python
   for k, v in placeholders.items():
       html = html.replace('{{' + k + '}}', v)
   ```
3. Run `templates/render.sh slide-NN.html /tmp/carousel-<slug>/slide-NN.png`
4. Verify each output is exactly `1080 x 1350` (the render script checks this).

**Do not** invent your own render path. `render.sh` handles sprite inlining, viewport chrome compensation (Chrome reserves 87px, script oversizes window to 1450 and crops with `sips`), and dimension verification.

If Chrome is not installed at the default path, surface this as a blocker — do not try to fall back to `mcp__chrome-devtools__take_screenshot` unless the user explicitly asks, because that tool depends on a live browser tab and is lossy for batch rendering.

### Phase 4 — Image review gate

Generate a quick preview HTML:
```bash
cd /tmp/carousel-<slug>
python3 -c "print('<html><body style=\"background:#222;padding:20px\">' + ''.join(f'<img src=\"slide-{i:02d}.png\" style=\"width:300px;margin:8px;border:2px solid #fff;vertical-align:top\">' for i in range(1,11)) + '</body></html>')" > preview.html
open preview.html
```

Tell the user the path. If they want changes:
- **Copy-only** edits → back to Phase 2, re-render the single changed slide.
- **Template swap** → re-render just that slide.
- **Palette/layout tweak** → edit `templates/shared.css` (this affects ALL future decks; flag the tradeoff) and re-render everything.

Do not auto-loop. Explicit user approval required before continuing.

### Phase 5 — Host the images publicly

Buffer requires publicly reachable URLs. Default flow:

```bash
# From repo root
mkdir -p generated/<YYYY-MM-DD>-<slug>/
cp /tmp/carousel-<slug>/slide-*.png generated/<YYYY-MM-DD>-<slug>/
git add generated/<YYYY-MM-DD>-<slug>/
git commit -m "Add carousel assets: <article-title>"
git push origin main
```

Construct URLs as:
`https://raw.githubusercontent.com/<owner>/<repo>/main/generated/<YYYY-MM-DD>-<slug>/slide-NN.png`

If the user prefers a different host (S3, Cloudflare R2, beehiiv uploads) — ask first; fall back to the GitHub raw pattern only when they haven't stated a preference.

### Phase 6 — Post to Buffer

Reuse the channel-filter and CTA patterns from `../promote-newsletter/SKILL.md`. Core flow:

1. `mcp__buffer__get_account` → organization ID.
2. `mcp__buffer__list_channels` → filter to `isDisconnected=false AND isLocked=false`.
3. For each channel, compose and schedule:

| Platform | `metadata` | `assets.images` | Caption body |
|---|---|---|---|
| Instagram | `{ instagram: { type: "carousel", shouldShareToFeed: true } }` | All 10 PNG URLs, in order. | `"<strongest quote>"\n\nComment "newsletter" to get my latest post, "<Article Title>".` |
| LinkedIn | `{ linkedin: {} }` (no `type` — LinkedIn metadata doesn't accept one) | All 10 PNG URLs | Same as IG. If Buffer rejects multi-image, fall back to slide-01 only + article URL in text. |
| Facebook | `{ facebook: { type: "post" } }` (`PostTypeFacebook` has no carousel — multi-image posts as a slideshow automatically) | All 10 PNG URLs | Same as IG. |
| Threads | `{ threads: { type: "post" } }` | All 10 PNG URLs (Threads caps at 20) | Same as IG. |

**Note:** Buffer's `PostType` enum includes `carousel`, `AssetsInput.images` accepts an array, and `InstagramPostMetadataInput.type` accepts `carousel`. Verified via `introspect_schema` during skill authorship. If a channel rejects the carousel payload, log the error and continue to the next channel — do not abort the whole run.

**Scheduling**: `mode: "addToQueue"`, `schedulingType: "automatic"` (same as `promote-newsletter`).

**Duplicate-URL guard**: Buffer duplicate-detects image URLs across posts. Each slide PNG has a unique path (slide-01..slide-10) and each deck lives at a unique date-slug dir, so duplicate detection should not fire. If it does, suffix URLs with `?v=<timestamp>`.

**Rate limiting**: Match `promote-newsletter`. After ~40 `create_post` calls in rapid succession, stop and save remaining channels to `remaining-posts.md`.

### Phase 7 — Summary

Print a table:

| Platform | Channel | Status | Queued post URL |
|---|---|---|---|

Also report:
- Path to the 10 local PNGs
- The GitHub raw URL prefix used for hosting
- The accent word in the CTA (should be `newsletter` — if you used a different word, flag it so the user can check the Comment-to-DM automation matches)

## Copy voice

Match the EVC voice already in `promote-newsletter`:
- Direct, confident, slightly dry
- No hype-words ("incredible", "game-changer", "mind-blowing")
- No emoji in headlines or body — keep it typographic. Emoji only in the caption CTA if it matches the existing skill.
- Prefer full sentences on section slides. Quotes keep the newsletter's punctuation verbatim.

## Gotchas

- **Font fidelity**: templates assume `Inter` (system fallback `Helvetica Neue`). On systems without Inter, rendering still works but the display weight may feel lighter. Don't ship a deck without previewing it — the sample deck at `examples/sample-deck/` is the reference.
- **Long quotes wrap awkwardly** on the quote template. If a quote is >260 chars, either trim or split across two sequential quote slides (replace a stat slide to keep the 10-slide total).
- **Chrome version drift**: the 87px window-chrome reservation in `render.sh` is based on Chrome 147. If rendering produces elements clipped at the bottom, measure the actual viewport with a debug page and update the `--window-size` oversize in `render.sh`.
- **Slide 10 accent word**: MUST be `newsletter` verbatim. The Comment-to-DM automation listens for this exact trigger; any variation breaks the funnel.
- **X/Twitter**: not a target. Do not include it even if the channel list returns a connected Twitter account.

## Verification checklist before shipping

- [ ] All 10 PNGs exist at exactly 1080×1350
- [ ] Slide 10 accent word is literally `newsletter`
- [ ] Every quote slide uses a verbatim quote from the newsletter (diff-check against the source)
- [ ] Attribution on every quote slide = `<Article Title> — Enterprise Vibe Code`
- [ ] Hook slide's LINE_1 + LINE_2 together do not exceed ~26 characters (else display scale breaks)
- [ ] Images are committed + pushed (and raw URLs return HTTP 200 — spot-check slide-01)
- [ ] Preview deck visually matches `examples/sample-deck/` palette/scale
- [ ] Buffer `create_post` returned success (not `InvalidInputError`) for every scheduled channel
