---
name: long-form-pulse
description: Use when user wants a short Buffer companion announcement post for an already-published LinkedIn pulse article so its engagement is closed-loop attributable — "announce my linkedin pulse", "buffer companion post for pulse article", "promote published pulse", "schedule a companion post for my pulse", "fill the pulse attribution gap". Takes the published LinkedIn pulse URL, composes a short voice-matched announcement per channel, tags it `format:long-form-pulse`, and schedules it to Buffer.
user_invocable: true
---

# long-form-pulse

Companion to `crosspost-newsletter`. That skill publishes a beehiiv article as a **native LinkedIn pulse article** (directly into LinkedIn's article editor — NOT via Buffer), so the pulse itself has **no Buffer-routed `format:` tag** and `buffer-stats` has nothing to attribute its downstream engagement against. This skill fills that gap.

Given a **published** LinkedIn pulse URL, it composes a SHORT, original, voice-matched companion **announcement** post per channel ("I just published a long-form piece on X — read it here"), tags every post `format:long-form-pulse`, and schedules them to Buffer. Now the announcement's engagement is closed-loop attributable via `buffer-stats`, and the in-text `[lp:<pulse_slug>]` tag carries the source link even off-Buffer (defense in depth).

This is the skill the [ARCHITECTURE.md format-tag table](../ARCHITECTURE.md#the-format-tag-values) reserved `format:long-form-pulse` for.

## What this is NOT

- **Not** a publisher of the pulse article itself. The pulse must already be live on LinkedIn — `crosspost-newsletter` (or a manual LinkedIn post) does that first. This skill only schedules the **companion announcement** that points at it.
- **Not** a verbatim quoter (`promote-newsletter`) or a teaser of the *beehiiv* article (`tease-newsletter`). The companion announces a *published LinkedIn article* and links to it — short original copy, voice-matched.
- **Not** a carousel (`carousel-newsletter`).

## When to Use

Use when:
- A LinkedIn pulse article is already published (you have its URL) and you want a Buffer companion post that announces it across IG / LinkedIn / Facebook / Threads.
- You ran `crosspost-newsletter`, the pulse landed, and you want the pulse's reach reflected in `buffer-stats` / `/flywheel` rather than only `linkedin-stats`.

Do NOT use when:
- The pulse isn't published yet → run `crosspost-newsletter` first.
- You want to promote the underlying *beehiiv* article (not the LinkedIn pulse) → `tease-newsletter` / `promote-newsletter`.

## ⚙️ One-time setup PREREQ — the `format:long-form-pulse` Tag ID

Closed-loop attribution requires a **Buffer Tag ID** (24-char hex), not a tag-name string — Buffer's `CreatePostInput` schema has `tagIds: [TagId!]`; a `tags: ["format:long-form-pulse"]` string is **silently dropped**. Buffer's public GraphQL API has no `createTag` mutation, so the tag must be created in Buffer's web UI **first**:

1. **Create the tag in Buffer's UI.** Buffer → post composer / settings → Tag picker → create a tag named **exactly** `format:long-form-pulse`.
2. **Capture its Tag ID.** Attach it to any one test post, then list IDs (per [`_shared/buffer-post-prep/README.md` § Tag IDs](../_shared/buffer-post-prep/README.md)):
   ```
   mcp__buffer__execute_query summary:"List format:* Tag IDs" query:'
   query { posts(input: {organizationId: "<your-org-id>"}, first: 50) {
     edges { node { tags { id name } } } } }'
   ```
3. **Add it to `tag-ids.local.json`.** Append the `long_form_pulse` key to `<repo-root>/_shared/buffer-post-prep/tag-ids.local.json` (gitignored, per-org):
   ```json
   {
     "verbatim_quote": "…",
     "teaser":         "…",
     "carousel":       "…",
     "link_share":     "…",
     "batch_summary":  "…",
     "long_form_pulse": "<the-24-hex-id-you-captured>"
   }
   ```

**Do not invent this ID.** It is per-organization user config and lives only in the gitignored local file. If the key is absent, `buffer-post-prep` still emits the post args **without** `tagIds` (graceful degradation, warns on stderr) — the post publishes but is **untagged**, defeating the whole point of this skill. `audit-buffer-queue` will flag the untagged post at the next weekly review. The `buffer-post-prep` binary already accepts `--format-tag long_form_pulse` → `format:long-form-pulse`; only the Tag ID needs setup.

## 🟢 Happy Path (read first; everything below is edge-case detail)

For a companion-announcement run when nothing goes wrong. ~8-12 min wall-clock.

**Phase 1 — Fetch (1-2 min).** Take the published LinkedIn pulse URL (the invocation arg). `WebFetch` it to extract the article **title** and a one-line **hook** (the lede / strongest sentence). Derive the **`pulse_slug`** from the URL (the trailing slug after the last `-`-joined segment, or the beehiiv source slug if known). Run `_shared/voice-corpus/voice-corpus` and hold the JSON for the compose phase (this is original copy — voice grounding is required). See [PATTERNS.md § Voice grounding](../PATTERNS.md#pattern-voice-grounding-for-original-copy-generation).

**Phase 2 — Fan-out level + channels (1 min).** Ask the user up-front the fan-out level — **1/channel, 3/channel, or all/channel** ([PATTERNS.md § Buffer create_post — caps](../PATTERNS.md#pattern-buffer-create_post-with-channel-filter--caps)). No universal default. Target channels are IG / LinkedIn / Facebook / Threads (X is off the list per `project_channels.md`).

**Phase 3 — Compose (2-4 min).** Inline the voice corpus as-is (no second truncation). Write ONE short announcement message, length-adapted per channel — same message, never angle-varied. Each post: a short voice-matched announcement + the pulse URL + (where appropriate) the canonical `Comment "newsletter"…` CTA + the `[lp:<pulse_slug>]` in-text tag on the last line.

**Adversarial review (REQUIRED).** Run `_shared/adversarial-review/adversarial-review` with `SKILL_NAME long-form-pulse` and the per-skill RULES_LIST. Must return `all_pass` before the user sees drafts.

**Phase 4 — User review (1 min).** Show each drafted post per channel with char count vs budget. Ask "Ready to schedule these to Buffer?"

**Phase 5 — Schedule to Buffer (1-2 min).** `mcp__buffer__get_account` → org ID + timezone. `mcp__buffer__list_channels` → filter `isDisconnected` / `isLocked` / `service: "startPage"` / below `min_followers_to_promote=50`. Per approved post: route through `_shared/buffer-post-prep/buffer-post-prep --format-tag long_form_pulse --mode addToQueue`, then `mcp__buffer__create_post` with the resulting args (carries `tagIds: [<format:long-form-pulse Tag ID>]`). Report per-channel.

## Process

### Phase 1 — Fetch the Pulse + Voice Corpus

The invocation arg is the **published LinkedIn pulse URL** (e.g. `https://www.linkedin.com/pulse/<slug>/`). If the user didn't supply one, ask for it — this skill cannot run without a live pulse URL.

**Extract the article title + hook:**
```
WebFetch <pulse-url>  # extract: article title, the lede / strongest one-line hook, author byline
```
If `WebFetch` can't read the pulse (LinkedIn auth wall), ask the user to paste the **title** and a **one-line hook**, and confirm the canonical URL. Do not fabricate the title.

**Derive the `pulse_slug`** for the in-text tag. Priority:
1. If this pulse was produced from a beehiiv article and you know that article's slug, reuse the **beehiiv slug** (keeps `[lp:<slug>]` joinable to the same source as the `[bh:<slug>]` chain). E.g. `[lp:tokens-from-our-past]`.
2. Otherwise use the LinkedIn pulse URL's trailing slug (the human-readable segment, minus LinkedIn's trailing opaque ID). E.g. `…/pulse/scaling-without-the-slop-mike-lady-abc1/` → `scaling-without-the-slop`.

Keep the slug short, lowercase, hyphenated, and **stable** — `buffer-stats` / `/flywheel` / a future LinkedIn-pulse fetcher will grep for `[lp:<slug>]` to correlate engagement back to the source. See [`_shared/post-manifest/README.md` § In-text tag convention](../_shared/post-manifest/README.md#in-text-tag-convention) (scheme `lp` = LinkedIn pulse slug).

**Fetch the voice corpus** — this skill generates ORIGINAL copy, so voice grounding is required (same weight as a fabrication check):
```bash
_shared/voice-corpus/voice-corpus  # auto-refreshes if cache > 7 days old
```
Output is JSON with `posts: [{title, url, published_at, source_type, body_text}]` (`source_type` = `newsletter` or `youtube_live`; the corpus spans newsletters + livestream transcripts). Hold it for Phase 3. **Use `body_text` as-is — do NOT add a second truncation** (no `body_text[:N]` inline); the binary already caps each post (see [`_shared/voice-corpus/README.md` § Consumers](../_shared/voice-corpus/README.md#consumers-of-this-binary--do-not-add-a-second-truncation)). Rationale: [PATTERNS.md § Voice grounding](../PATTERNS.md#pattern-voice-grounding-for-original-copy-generation).

### Phase 2 — Fan-out Level + Channels (ask up-front)

**Ask the user the fan-out level before composing** — per [PATTERNS.md § Buffer create_post — caps](../PATTERNS.md#pattern-buffer-create_post-with-channel-filter--caps), there is **no universal default**:

> Fan-out level for this pulse announcement: **1/channel**, **3/channel**, or **all/channel**?

Buffer Insights (2026-04-27) showed reactions ↓52% M-o-M when posts ↑24.5% — so caution past ~3/channel — but a major-launch pulse often wants saturation. Surface the choice; don't bake in a number. A companion *announcement* is usually a single post per channel (1/channel) unless the pulse is a flagship piece.

**Target channels:** Instagram, LinkedIn (page + personal), Facebook, Threads. **X/Twitter is off the channel list** (`project_channels.md`). The actual channel list comes from `mcp__buffer__list_channels` in Phase 5 — never guess channel IDs here.

### Phase 3 — Compose the Companion Announcement

**VOICE GROUNDING (read this BEFORE writing any draft):**

Prepend the Phase 1 voice-corpus output as inline excerpts, **grouped by `source_type`** so the model can tell the written register from the spoken one:

> The author's recent **newsletters** (written register, from beehiiv RSS):
> ---
> [for each post where source_type=="newsletter"] **<Title>** (<published_at>): <body_text>
> ---
> The author's recent **livestream transcripts** (spoken register — raw captions, filler included; mine for phrasing/idiom/stance, not for verbatim lifting):
> ---
> [for each post where source_type=="youtube_live"] **<Title>** (<published_at>): <body_text>
> ---

The drafts MUST sound like a continuation of this voice. Match:
- **Sentence rhythm** — short-to-medium with the occasional intentional fragment
- **Vocabulary** — phrases the author actually uses (avoid LinkedIn-corporate-speak unless the author uses it)
- **Recurring framings** — "vibe coding", "agentic", "tokens from our past", "compounding taste"
- **First-person stance** — "I just published" not "We're excited to announce"
- **Tone** — slight irreverence + grounded practicality + first-hand observation

**Mismatched voice is a fail signal — same weight as a fabrication.** A draft that's factually correct but reads as generic LinkedIn announcement copy fails this rule. See [PATTERNS.md § Voice grounding](../PATTERNS.md#pattern-voice-grounding-for-original-copy-generation).

**CRITICAL RULES:**

1. **It's an ANNOUNCEMENT, not the article.** Short original copy that says "I published a long-form piece on X — here's the link," framed around *why someone should read it*. Do NOT paste the pulse body. The whole value of a companion post is that it's a fresh, scannable nudge toward the long-form piece.
2. **One message, length-adapted per channel — never angle-varied.** Write the longest applicable version (LinkedIn/Instagram) first, then length-trim the SAME message for shorter channels. Same hook, same framing. (Same discipline as `tease-newsletter` Phase 4.)
3. **Stay faithful to the published pulse.** Don't claim the article says things it doesn't. Don't invent metrics ("read by thousands"). **Banned:** unverifiable third-party claims ("every leader I respect…", "everyone in [industry] knows…").
4. **No emoji unless the user requests it.**
5. **Always link the pulse URL.** The companion post's job is to drive readers to the live LinkedIn article — every post includes the pulse URL.
6. **CTA — use the canonical `Comment "newsletter"…` CTA where appropriate.** If the pulse was syndicated from a beehiiv newsletter (the usual case via `crosspost-newsletter`), append the standard CTA so the same Manychat / Comment-to-DM funnel works. Build it with the shared helper — **never hand-edit it** (varying casing/quotes/synonyms breaks the DM trigger silently — [PATTERNS.md § Comment-newsletter CTA](../PATTERNS.md#pattern-comment-newsletter-cta--dm-trigger)):
   ```bash
   _shared/cta.sh "<Pulse / Article Title>"
   # → Comment "newsletter" to get my latest post, "<Title>"
   ```
   If the pulse is **not** newsletter-backed (standalone LinkedIn article), the CTA doesn't apply — drop it; don't force the trigger word onto an article that has no DM funnel.
7. **In-text `[lp:<pulse_slug>]` tag — defense in depth.** The **last line** of every post body, separated by a blank line, carries `[lp:<pulse_slug>]` (the slug from Phase 1). This is the [`_shared/post-manifest`](../_shared/post-manifest/README.md#in-text-tag-convention) `lp:` scheme — it travels with the post even off-Buffer so a future fetcher can recover the source link without the manifest. It's **in addition to** the Buffer `format:long-form-pulse` tag (which is the primary attribution), not a replacement. Don't put it in the title.

**Post structure (long-form channels — LinkedIn / Instagram):**
```
<voice-matched announcement: 1-3 short paragraphs — what the piece is, why it's worth the read, withhold the full payoff>

<pulse URL>

[optional, if newsletter-backed] Comment "newsletter" to get my latest post, "<Title>"

[lp:<pulse_slug>]
```

**Post structure (short-form channels — Threads / others):**
```
<one-to-two-line announcement + the implicit promise of payoff>

<pulse URL>

[lp:<pulse_slug>]
```

**Character budgets** — total whole-post count (announcement body + blank line + URL + optional CTA + blank line + `[lp:…]` tag) must fit under the platform budget. The LinkedIn pulse URL is ~60-90 chars and the `[lp:…]` tag ~15-30 chars — account for both. Budget = hard limit − URL − CTA (if present) − `[lp:…]` tag − 7 margin.

| Platform   | Hard limit | Notes |
|------------|-----------:|-------|
| Threads    | 500        | 1-2 lines + URL + tag |
| Facebook   | 500        | 2-4 lines + URL + tag |
| Instagram  | 2,200      | long-form caption; image optional (see below) |
| LinkedIn   | 3,000      | 2-4 short paragraphs + URL + CTA + tag |

(X/Twitter intentionally omitted — off the channel list.)

**Media:** A companion announcement is **text + link by default** — the LinkedIn link preview supplies the visual on most channels. If the user wants an image and one is available (e.g. the pulse's hero), attach it via `assets.images` in Phase 5. **Instagram requires an image** — if no image is available, skip the IG companion post (don't post text-only to IG; Buffer rejects it). Don't block the run on image generation; this skill does not generate images.

### Adversarial review (REQUIRED before user review)

Apply the **[Adversarial Review pattern](../PATTERNS.md#pattern-adversarial-review)** — spawn fresh reviewers (no compose-phase context) via the `_shared/adversarial-review/adversarial-review` Go binary (the vendored converge `audit` fan-out; default reviewers `claude,codex,agy`, merged FAIL-OR). Build once: `cd _shared/adversarial-review && go build .`. Assemble the prompt by substituting the scaffold's `<<…>>` placeholders, then:
```bash
printf '%s' "$ASSEMBLED_PROMPT" | _shared/adversarial-review/adversarial-review
```

Per-skill specifics:

- **SOURCE_LABEL:** "PUBLISHED LINKEDIN PULSE BEING ANNOUNCED"
- **SOURCE_CONTENT:** the pulse **title**, the extracted **hook/lede**, the canonical **pulse URL**, and the `pulse_slug`. **Prepend a `Publication: Enterprise Vibe Code` line** so strict reviewers (e.g. `agy`) don't flag the publication/author name as fabrication.
- **SKILL_NAME:** `long-form-pulse`
- **ARTIFACT_NAME:** "companion announcement"
- **RULES_LIST:**
  - Each post is a SHORT original announcement of the published pulse — NOT a paste of the article body, NOT a verbatim quote.
  - Claims must be SUPPORTED by the pulse title/hook. No invented metrics ("read by thousands"), no fabricated takeaways the article doesn't deliver.
  - BANNED: unverifiable third-party claims ("every leader I respect…", "everyone in [industry] knows…").
  - BANNED: emoji unless explicitly requested.
  - REQUIRED: every post links the canonical pulse URL.
  - REQUIRED: every post ends with the `[lp:<pulse_slug>]` in-text tag on its own last line.
  - REQUIRED (newsletter-backed pulses only): the canonical `Comment "newsletter" to get my latest post, "<Title>"` CTA, exact string (built via `_shared/cta.sh`). For standalone (non-newsletter) pulses the CTA is correctly ABSENT — do not flag its absence.
  - REQUIRED: same core message across all channels — only length-adapted, not angle-varied.
  - REQUIRED: total post char count within the platform budget (hard limit − URL − CTA − `[lp:…]` tag − 7 margin).
- **ISSUE_GUIDANCE:** "For an unsupported claim, quote it and explain why the title/hook doesn't support it. For body-paste drift, quote the run that reads like article copy rather than an announcement. For a missing/edited CTA on a newsletter-backed pulse, quote the CTA line and show the deviation from the canonical string. For a missing `[lp:…]` tag, say which draft omits it. For over-budget posts, give the channel + count."

**🔒 HARD GATE:** the binary must return `summary == "all_pass"` before Phase 4. On `some_fail`, fix the cited issues and re-run — never surface FAIL items to the user. Honor the [5-round cap](../PATTERNS.md#round-cap-5-iterations-max); after 5 rounds, surface remaining items as deadlocks for the user to arbitrate.

### Phase 4 — Review Before Scheduling

Present every drafted post for approval:
```
Channel: @handle (LinkedIn personal)
Post (487/3000 chars):
---
<announcement body>

https://www.linkedin.com/pulse/<slug>/

Comment "newsletter" to get my latest post, "<Title>"

[lp:<pulse_slug>]
---
Image: <URL or "text-only">
```
Repeat per channel. Ask: **"Ready to schedule these to Buffer?"** The user can approve all, edit individual posts, skip channels, or cancel.

### Phase 5 — Schedule to Buffer

**🔒 HARD GATE — adversarial review must have returned `summary == "all_pass"` before this phase.** If the prior run returned `some_fail` for any draft, do NOT call `mcp__buffer__create_post` for it. Iterate fixes per the [round cap](../PATTERNS.md#round-cap-5-iterations-max), then surface deadlocks. Manual spot-checking does NOT substitute.

1. `mcp__buffer__get_account` → organization ID + timezone. If multiple orgs, ask which one.
2. `mcp__buffer__list_channels` with the org ID. **Never guess channel IDs.**
3. Filter out `isDisconnected`, `isLocked`, `service: "startPage"`, and channels below `min_followers_to_promote = 50` before posting.

Apply the **[Buffer create_post pattern](../PATTERNS.md#pattern-buffer-create_post-with-channel-filter--caps)** for transport. **🚦 Every post MUST go through `buffer-post-prep` before `mcp__buffer__create_post`** — do not hand-roll args (the helper's `platformLimits` pre-validation catches char-limit overruns before the API round-trip; bypassing it is the 2026-05-03 700-char-Threads bug).

```bash
_shared/buffer-post-prep/buffer-post-prep \
  --channel-id <id> \
  --service linkedin \
  --text "<announcement + pulse URL + CTA + [lp:<slug>]>" \
  --format-tag long_form_pulse \
  --mode addToQueue
```

The helper:
- Skips disconnected/locked/startPage channels and channels below `min_followers_to_promote=50`
- Validates post text against the platform char limit
- Attaches `tagIds: [<format:long-form-pulse Tag ID>]` from `_shared/buffer-post-prep/tag-ids.local.json` (24-char hex — Buffer's schema requires Tag IDs, not name strings; `tags: [...]` is silently dropped). **Requires the one-time Tag ID setup above** — if the `long_form_pulse` key is absent, the binary warns on stderr and emits args without `tagIds`; surface that warning to the user (the post would be untagged and break closed-loop attribution).
- Pass `--image-url <url> --image-alt "<title>"` if attaching the pulse hero image. Instagram requires an image — skip the IG cell if none.

For each approved post, build args via the helper, then call `mcp__buffer__create_post` with the resulting JSON. Mode is `addToQueue` (a companion announcement schedules into the queue, not instant-publish — unless the user says otherwise).

**Failure handling** (per [PATTERNS.md § retry handling](../PATTERNS.md#retry-handling-for-known-transient--idempotent-failures)):
- **HTTP 429 rate limit:** stop immediately, save remaining posts (channel ID, text, tags, mode) to `remaining-posts.md` in the project dir, report which succeeded.
- **Timeout (10000ms):** the post likely landed — wait 5s, retry the SAME payload once; a "posted that one recently"/"duplicate" error means the original went through (treat as success).
- **Char-limit rejection:** the helper should have caught it; if it slipped through, log + skip the post (do NOT silently truncate — that's an editorial change) and surface to the user.

Report per-channel: success (with post ID) or error message. Confirm to the user that every scheduled post carries `tagIds: [<format:long-form-pulse Tag ID>]` so `buffer-stats` Phase 5 will attribute its engagement — this is the gap this skill closes.

## References

- Closest sibling (single-item, format-tagged, voice-grounded, adversarial-reviewed): [`promote-github/SKILL.md`](../promote-github/SKILL.md)
- CTA + length-adapt-not-angle-vary discipline: [`tease-newsletter/SKILL.md`](../tease-newsletter/SKILL.md)
- In-text `[lp:<slug>]` tag scheme: [`_shared/post-manifest/README.md`](../_shared/post-manifest/README.md#in-text-tag-convention)
- Format-tag registry: [ARCHITECTURE.md § format-tag values](../ARCHITECTURE.md#the-format-tag-values), [`_shared/format_tags.json`](../_shared/format_tags.json)
- Memory: `project_channels.md` (no X), `feedback_ask_fanout_upfront.md` (ask fan-out level up-front)
