---
name: carousel-newsletter
description: Use when user wants to promote a beehiiv newsletter as a 10-slide illustrated carousel to Buffer for Instagram, LinkedIn, Facebook, Threads — "carousel the newsletter", "swipe post", "newsletter carousel", "10-slide post". Generates on-brand EVC slides with AI-generated illustrations (Gemini 2.5 Flash Image / Nano Banana) and schedules them with a "comment 'newsletter'" CTA.
user_invocable: true
---

# carousel-newsletter

Promote a beehiiv newsletter post as a **10-slide illustrated carousel** on every Buffer-connected channel (Instagram, LinkedIn, Facebook, Threads). The carousel summarizes the newsletter using **direct quotes** and drives a `comment "newsletter"` CTA that routes into the Comment-to-DM funnel.

The visual system: **two-zone slides** — a cream text zone + a dedicated illustration zone filled by a Gemini-generated image. The Enterprise Vibe Code (EVC) banner is attached as a style reference on every generation, locking palette / character design / illustration language.

## When to run

User says things like:
- "carousel my latest newsletter"
- "make a swipe post for the EVC article"
- "10-slide post of this newsletter"

If they want a **single-image snippet**, use `promote-newsletter`. If they want to **republish the full article**, use `crosspost-newsletter`.

## Prerequisites

**Auth (one-time):** Google Cloud Application Default Credentials.
```bash
brew install --cask gcloud-cli
gcloud auth application-default login
gcloud config set project gen-lang-client-0527845499
```

**Python deps (one-time):** `pip3 install --user --break-system-packages google-genai`

**Runtime requirements:**
- Google Chrome at `/Applications/Google Chrome.app` (headless render).
- `sips` on PATH (macOS built-in).
- `python3` ≥ 3.10.
- EVC banner reference image at `~/Pictures/evc_banner2.png` (used as style reference for every image-gen call).
- Public hosting for the rendered PNGs — default flow commits them to this repo under `generated/<YYYY-MM-DD>-<slug>/` and serves via GitHub raw URLs. Buffer needs publicly reachable URLs.

## Key files

- `templates/shared.css` — brand tokens + two-zone layout classes (`.split-hook`, `.split-section`, `.split-quote`, `.split-stat`, `.split-cta`). Each `.split-*` class sets the absolute positioning of the `.zone-text` and `.zone-illus` boxes for that slide type.
- `templates/01-hook.html` — cover slide (text 40% top, illus 60% bottom, 4:3 scene).
- `templates/02-section.html` — kicker + headline + body (text left 65%, illus right 35%, 9:16 narrow scene).
- `templates/03-quote.html` — verbatim quote (text top 80%, illus bottom 20% wide strip, 16:9 scene).
- `templates/04-stat.html` — big number + label (text left 60%, illus right 40%, 9:16 scene).
- `templates/05-cta.html` — "Comment 'newsletter'" CTA (text left 60%, illus right 40%, 9:16 scene).
- `templates/gen_illustration.py` — image-gen helper. Calls Gemini 2.5 Flash Image via Vertex AI with the EVC banner as style reference + master brand prompt + per-slide scene prompt. Usage: `gen_illustration.py "<scene>" <output.png> --aspect <ratio>`.
- `templates/render.sh` — HTML → 1080×1350 PNG. Headless Chrome with viewport-chrome compensation + `sips` crop. Usage: `render.sh <filled.html> <output.png>`.
- `templates/illustrations.svg` — **legacy** SVG sprite from the v1 hand-drawn approach. Retained for backward compat (render.sh still inlines it if `<!--SVG_SPRITE-->` marker present) but no longer used by the current templates.
- `examples/sample-deck/` — reference 10-slide rendered deck (pre-image-gen era; visually outdated but shows the structural layout).

## Related skills to read first

- `../promote-newsletter/SKILL.md` — beehiiv RSS fetch, Buffer channel filter (connected + unlocked), CTA copy, rate-limit/remaining-posts pattern.
- `../crosspost-newsletter/SKILL.md` — richer beehiiv DOM extraction if you need quote candidates beyond what RSS exposes.

## Workflow

### Phase 1 — Fetch the newsletter

Same pattern as `promote-newsletter`. WebFetch the beehiiv RSS feed (or the URL the user provides). Extract: title, subtitle, H2 section headings, body paragraphs, blockquotes, stat-shaped phrases, hero image URL (for reference, not used in the deck). Save to `/tmp/carousel-<slug>/source.json`.

### Phase 2 — Draft the 10-slide script  ← USER REVIEW GATE (COPY)

Plain text outline, no rendering yet. Fixed structure:

| # | Template | Purpose |
|---|---|---|
| 1 | `01-hook.html` | Title + 1-line tease. `LINE_1` + `LINE_2` ≤ ~26 chars combined for clean 2-line wrap. `TEASE` ≤ 120 chars. |
| 2 | `02-section.html` | Stage-setter. Body ≤ 220 chars. |
| 3 | `03-quote.html` | Strongest verbatim quote (≤260 chars). Attribution = `<Article Title> — Enterprise Vibe Code`. |
| 4 | `04-stat.html` | Stat or big number from the article. |
| 5 | `03-quote.html` | Second verbatim quote. |
| 6 | `02-section.html` | Key insight / "the shift". |
| 7 | `03-quote.html` | Third verbatim quote. |
| 8 | `04-stat.html` OR `02-section.html` | Supporting point. |
| 9 | `02-section.html` | Payoff / what to do. |
| 10 | `05-cta.html` | Fixed CTA, accent word = `newsletter` (verbatim; must match Comment-to-DM trigger). |

Surface this outline and **stop for user approval** before generating illustrations (which cost ~$0.04 each and hit rate limits).

### Phase 3 — Generate per-slide illustrations

For each slide, compose a **scene prompt** that describes what fills the illustration zone. Key rules:

1. **Describe a scene that FILLS the entire frame edge-to-edge.** Don't say "leave empty" — Nano Banana ignores region constraints. Composition is guaranteed by the two-zone HTML layout, not by the prompt.
2. **Tie the scene to the slide's meaning.** The robot, bricks, and tracks should *do something relevant to the slide's text* — a robot meditating on a stat slide about patience, a robot running on tracks for a "velocity" slide, a robot carefully stacking bricks for a "build slowly" slide. Generic scenes feel boring after a few decks.
3. **Match the aspect to the zone:**
   - Hook → `--aspect 4:3`
   - Section → `--aspect 9:16`
   - Quote → `--aspect 16:9`
   - Stat → `--aspect 9:16`
   - CTA → `--aspect 9:16`

**Example scene prompts (contextual):**

| Slide concept | Scene |
|---|---|
| Hook: "3 Lessons from Black Belt" | Horizontal scene: robot on rail tracks with wrench, minecart of bricks, 3-brick tower, large gear, small gear accent. |
| Section: "Train so you can train tomorrow" | Narrow portrait: robot patiently placing one brick on a 2-brick base — showing careful incremental work. |
| Quote about sustainability | Wide strip: robot seated on a rail tie next to a water-jug-shaped brick, relaxed posture. |
| Stat: "14 yrs" | Narrow portrait: tall 4-tier brick tower with tiny gear at top, robot looking up at it. |
| Section: "Democratized knowing and doing" | Narrow portrait: robot handing a wrench to a second smaller robot — teaching/enabling. |
| CTA | Narrow portrait: cheerful robot waving, celebratory brick tower, confetti-like gears floating. |

**Call pattern:**
```bash
python3 templates/gen_illustration.py "<scene>" /tmp/carousel-<slug>/illustrations/slide-NN.png --aspect <ratio>
```

**Rate limiting:** Nano Banana defaults to ~5 req/min on a new project. Sleep ~15s between calls, OR wrap each call in a retry loop with 30s backoff on 429. A full 10-slide deck takes ~3–5 min wall-clock including backoff.

**Cost:** ~$0.04/image = ~$0.40 per 10-slide deck in steady state. Billed to the `gen-lang-client-0527845499` project.

**Auth model:** `gen_illustration.py` uses `GOOGLE_GENAI_USE_VERTEXAI=true` + ADC. If you see `google.auth.exceptions.DefaultCredentialsError`, the user needs to re-run `gcloud auth application-default login`.

### Phase 4 — Render HTML → PNG

For each slide:
1. Copy the template, substitute placeholders:
   ```python
   html = html.replace('{{ILLUS_IMG}}', f'file://{illus_path}')
   for k, v in copy.items():
       html = html.replace('{{' + k + '}}', v)
   ```
2. `templates/render.sh <filled.html> <output.png>` — always produces exactly 1080×1350.

The render script handles Chrome's 87px window-chrome offset and crops via `sips`. Don't reinvent it.

### Phase 5 — User review gate (IMAGES)

Open a preview grid:
```bash
python3 -c "..." > /tmp/carousel-<slug>/preview.html && open /tmp/carousel-<slug>/preview.html
```
Let the user approve or request regenerations. **Single-slide regen is cheap** (~$0.04), so encourage iteration on slides that missed.

Common failure modes to watch for:
- Illustration went off-palette (teal/orange snuck in) → regenerate with stronger palette emphasis in the scene prompt.
- Text from the article accidentally appeared in the illustration → regenerate (model sometimes draws words).
- Zone cropping cut off a key element → shift the scene prompt to position the subject where the zone crops it well (center, left, right).

### Phase 6 — Host publicly

Buffer requires public URLs. Default flow:
```bash
mkdir -p generated/<YYYY-MM-DD>-<slug>/
cp /tmp/carousel-<slug>/slide-*.png generated/<YYYY-MM-DD>-<slug>/
git add generated/<YYYY-MM-DD>-<slug>/ && git commit -m "Add carousel assets: <title>" && git push
```
Raw URL: `https://raw.githubusercontent.com/<owner>/<repo>/main/generated/<YYYY-MM-DD>-<slug>/slide-NN.png`

### Phase 7 — Post to Buffer

Reuse patterns from `../promote-newsletter/SKILL.md`. Filter `list_channels` to `isDisconnected=false AND isLocked=false`. Per-channel:

| Platform | `metadata` | `assets.images` | Caption |
|---|---|---|---|
| Instagram | `{ instagram: { type: "carousel", shouldShareToFeed: true } }` | All 10 PNG URLs | `"<strongest quote>"\n\nComment "newsletter" to get my latest post, "<Title>".` |
| LinkedIn | `{ linkedin: {} }` | All 10 PNG URLs | Same as IG. |
| Facebook | `{ facebook: { type: "post" } }` | All 10 PNG URLs | Same as IG. |
| Threads | `{ threads: { type: "post" } }` | All 10 PNG URLs (max 20) | Same as IG. |

`mode: "addToQueue"`, `schedulingType: "automatic"`. Verified via `introspect_schema`: `PostType.carousel` exists, `AssetsInput.images` is an array, `InstagramPostMetadataInput.type` accepts `carousel`.

If a channel rejects the carousel payload, log and continue — do not abort the whole run. Channels not targeted: X/Twitter (not connected), TikTok/YouTube (need video).

### Phase 8 — Summary

Print a table: Platform | Channel | Status | Buffer queue URL. Report path to local PNGs and the GitHub raw URL prefix. Confirm the CTA accent word is literally `newsletter`.

## The master brand prompt

Locked in `gen_illustration.py` as the `BRAND_PROMPT` constant. It covers: illustration style, strict palette (navy / green / yellow / blue / gray / brown / white / cream), robot character design, lego brick rules, gears, rails, minecart, composition rules (no text in images, small navy triangle watermark bottom-right). Every image-gen call prepends this prompt before the per-slide scene description. Do not tweak it per-post — consistency across decks is the point.

## Gotchas

- **ADC expiry:** the creds file at `~/.config/gcloud/application_default_credentials.json` is long-lived but tied to the user's Google account. Revoking Google access or `gcloud auth application-default revoke` breaks the skill.
- **Banner size:** the full `evc_banner2.png` is ~4.9 MB; Vertex silently drops attachments that large. `gen_illustration.py` caches a downscaled 1024px version at `/tmp/evc-banner-1024.png`.
- **Slide 10 accent word:** MUST be `newsletter` verbatim — the Comment-to-DM automation listens for this exact trigger.
- **Inter font fallback:** templates assume Inter with `Helvetica Neue` fallback. If Inter isn't installed, the display weight reads a little lighter but still on-brand.
- **Chrome version drift:** the 87-px window-chrome reservation in `render.sh` is based on Chrome 147. If elements clip at the bottom, measure the actual viewport and bump the `--window-size` value.

## Verification checklist before shipping

- [ ] All 10 PNGs exist at exactly 1080×1350.
- [ ] Slide 10 accent word is literally `newsletter`.
- [ ] Every quote slide uses a verbatim quote (diff-check against source).
- [ ] Every illustration is on-palette (no teal/orange/red).
- [ ] No stray text in any illustration (model occasionally draws words).
- [ ] Scene per slide is *related to the slide's copy*, not a generic EVC tableau.
- [ ] Images committed + pushed; spot-check slide-01 raw URL returns HTTP 200.
- [ ] Buffer `create_post` returned success (not `InvalidInputError`) for every scheduled channel.
