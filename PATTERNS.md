# Patterns

Cross-skill patterns that involve **cognition** (judgment the LLM exercises in the prompt). For pure-transport helpers (validation, data shaping, deterministic API calls), see [`_shared/`](./_shared/).

When a skill needs one of these patterns, it should reference the relevant section here rather than re-document the workflow in its own SKILL.md. This is the docs-DRY half of the closed-loop architecture (see [ARCHITECTURE.md](ARCHITECTURE.md)).

## Why this isn't code

Each pattern below requires the LLM to make a judgment call that's not safely encodeable in a binary. Per **the Primitive Test** (three necessary conditions: atomicity / Bitter Lesson / ZFC):
- **Atomicity** — Can two callers race? If not, no need for code-level coordination.
- **Bitter Lesson** — Does a smarter model still need this exact thing? If yes, it's plumbing (primitive). If a smarter model would do it differently, it's cognition (consumer layer / prompt).
- **ZFC** — Does any line of the implementation contain a judgment call (`if stuck then X`)? If yes, the decision belongs in the prompt, not the code.

The transport layer (in [`_shared/`](./_shared/)) handles the deterministic plumbing those decisions produce. The patterns below stay in prompts because they all fail one or more of the three conditions.

---

## Pattern: Adversarial review

Every compose-and-publish skill spawns a fresh subagent to audit drafted posts BEFORE the user reviews them. The reviewer has no context from the compose phase — fresh eyes catch fabrications and skill-rule violations the composer might have rationalized.

### Generic prompt scaffold

```
You are an adversarial reviewer for /<<SKILL_NAME>> drafts. Your job is to find problems before the user has to.

<<SOURCE_LABEL>>:
<<SOURCE_CONTENT>>

SKILL RULES (must be enforced):
<<RULES_LIST>>

DRAFTED <<ARTIFACT_NAME>>:
<<DRAFTS as a numbered list, each with its draft_id and content>>

For each draft, return:
- VERDICT: "PASS" or "FAIL"
- ISSUES: array of strings — specific problems with cited exact substrings.
  <<ISSUE_GUIDANCE>>

Return ONLY this JSON object, no surrounding prose:

{
  "summary": "all_pass" or "some_fail",
  "verdicts": [
    {"draft_id": "<id from input>", "verdict": "PASS" or "FAIL", "issues": ["...", "..."]}
  ]
}
```

(Generalized from the original `posts` framing to `drafts` to support cross-project use — code reviews, plan reviews, doc reviews.)

### How to invoke

The reviewer MUST be a fresh subagent with no context from the compose phase. Two equivalent paths:

**Option A (recommended): the standalone `/adversarial-review` skill** (in `~/dev/mike-skills/adversarial-review/`). It accepts SOURCE + RULES + DRAFTS as inputs and returns the JSON verdict. Reusable across projects.

```
Use the Skill tool: Skill name "adversarial-review", args = JSON containing
{source_label, source_content, skill_name, artifact_name, rules_list, issue_guidance, drafts}
```

**Option B (inline): use the Task / Agent tool with a `general-purpose` subagent.** Construct the prompt by substituting all `<<...>>` placeholders in the scaffold above, then send it as the agent's task input.

```
Tool: Agent (subagent_type: general-purpose)
Prompt: <the assembled scaffold with placeholders filled in>
```

Both paths return the same JSON shape. Parse it; if all verdicts are PASS, proceed to Phase 5. If any FAIL, fix the cited issues and re-run — never surface FAIL items to the user.

### Per-skill specifics

| Skill | SOURCE_LABEL | RULES_LIST (key items — see SKILL.md for full) | ISSUE_GUIDANCE focus |
|---|---|---|---|
| `promote-newsletter` | beehiiv article body | verbatim only; no rewriting; required CTA | "Cite verbatim location lines N-M" |
| `tease-newsletter` | beehiiv article body | no contiguous run of 7+ words from source; no spoiling punchline; no unverifiable third-party claims; same core message across channels | "Quote the 7+ word run; quote the spoiled punchline; quote the unverifiable claim" |
| `promote-github` | GitHub PR/commit/release body + diff stats | value/impact framing; no inflated metrics; no fabricated adoption claims; required link | "Quote the unsupported claim; suggest value/impact reframe" |
| `carousel-newsletter` | beehiiv article body | quote slides verbatim; section/stat slides grounded; CTA accent word literally `newsletter` | "Cite source line for quote slides; quote invented stats" |
| `crosspost-newsletter` | beehiiv article + platform-specific rules | per-platform: body source-faithful (full-article); no automod triggers (Reddit); HN-appropriate title shape | "Cite drift from source; flag automod-trigger patterns" |

### When to apply verdicts

- All PASS → proceed to Phase 5 user review
- Any FAIL → fix the failing drafts using the cited issues, re-run the reviewer until clean. **Do not surface FAIL items to the user; the user should see only PASS-grade copy at Phase 5.**

### Why this matters

The user caught a fabrication ("every leader I respect keeps a token on their desk") manually on the 2026-04-26 Tokens From Our Past run. Adversarial review prevents the next one automatically. This is the only way to scale up promotion volume without scaling up the user's review burden.

---

## Pattern: When to handoff vs proceed (gstack auth)

`_shared/gstack_auth.sh` handles the deterministic part of cookie import + login check. The DECISION of what to do when it fails (exit code 1) is cognition.

### Decision tree

| Exit code | What to do |
|---|---|
| 0 (logged in) | Proceed with platform workflow |
| 1 (not logged in) | If interactive (user is at the keyboard, no autonomous mode): `$B handoff` for manual login + `$B resume`. If autonomous (no human reviewer): skip the platform, mark as `auth_failed` in the run summary. |
| 2 (bad usage) | Stop entire skill — bug in caller |

### Per-platform overrides

- **Substack:** cookie import has been confirmed to NOT work (HttpOnly session cookies). Skip cookie attempt entirely; go straight to handoff.
- **Reddit:** the helper sets the UA spoof proactively (otherwise 403). No additional override needed.
- **Buffer Analyze:** cookie import for `buffer.com` carries to `analyze.buffer.com` as of 2026-04-27. Helper handles this transparently.

---

## Pattern: Comment-newsletter CTA + DM trigger

Every newsletter-promoting post ends with the canonical CTA:

```
Comment "newsletter" to get my latest post, "<Article Title>"
```

The exact string `newsletter` is the trigger word for the Manychat / Comment-to-DM automation. **Do not edit ad-hoc** — vary the casing, omit the quotes, swap to a synonym ("post"/"article") and the DM funnel breaks silently.

Use `_shared/cta.sh "<Article Title>"` to construct the canonical string. Skills concatenate it onto the post body with a blank line separator:

```python
body = "<snippet or teaser>"
cta = subprocess.check_output(["_shared/cta.sh", article_title], text=True)
post_text = f"{body}\n\n{cta}"
```

---

## Pattern: Buffer queue + recently-sent overlap check

When promoting an article, check both the queue and the last 7 days of sent posts for overlap with proposed snippets. Repeated content fatigues the audience (Buffer Insights data 2026-04-27 confirmed reactions ↓52% M-o-M during heavy fan-out weeks).

### Cognition (skill prompt)

The DECISION of which phrases to check for, and what to do when matches are found, is judgment:

- For `promote-newsletter`: check distinctive 4-8 word phrases from each candidate snippet + the article title
- For `tease-newsletter`: check the article title (teasers are by design original copy, low overlap risk)
- For `promote-github`: check the repo slug + release tag + a distinctive noun phrase
- When matches found: surface to user with annotations (`✅ new` / `⚠️ queued Nx` / `⚠️ sent + queued`); recommend skipping `⚠️ sent` items unless the user opts in with a fresh angle

### Transport (`_shared/buffer-queue-check`)

The substring matching itself is deterministic. The skill calls `mcp__buffer__list_posts` (which has Buffer auth via MCP) to get the queued + recent-sent JSON, pipes it to `buffer-queue-check --keywords "phrase1,phrase2,..."`, and gets back a JSON dict of matches per keyword. Skill then renders the annotations.

```bash
# Skill (inside Claude harness) calls MCP, captures JSON, pipes to helper
echo "$LIST_POSTS_JSON" | _shared/buffer-queue-check/buffer-queue-check \
  --keywords "Mac Pro,paperweight,Trash Can,Tokens From Our Past"
```

---

## Pattern: Format tag values (link to ARCHITECTURE.md)

The 6 valid `format:<name>` tag values are the [authoritative table in ARCHITECTURE.md](ARCHITECTURE.md#the-format-tag-values). The constants are also in [`_shared/format_tags.json`](./_shared/format_tags.json) for programmatic access.

When adding a new compose skill, define a new format tag and update both files (the table in ARCHITECTURE.md AND `_shared/format_tags.json`).

`_shared/buffer-post-prep` validates the `--format-tag` argument against this list and refuses unknown values — a structural enforcement that the closed-loop measurement system stays consistent.

---

## Pattern: Buffer create_post with channel filter + caps

Every promote-* skill that calls `mcp__buffer__create_post` should pre-filter channels and validate args via `_shared/buffer-post-prep`. The transport layer enforces:

- `min_followers_to_promote = 50` (skip below-threshold channels)
- `max_posts_per_channel_per_article = 3` (cap fan-out per article)
- `format:<name>` tag attached
- Platform-specific metadata (Facebook `type: "post"`, Instagram `type: "post" + shouldShareToFeed: true`, Pinterest `boardServiceId`, etc.)

### Cognition (skill prompt)

What stays in the skill: deciding WHICH snippet/teaser/carousel to send to which channel, in what order, with what image attached. The user's intent ("promote this article on these channels") doesn't compress to deterministic rules.

### Transport (`_shared/buffer-post-prep`)

```bash
_shared/buffer-post-prep/buffer-post-prep \
  --channel-id <id> \
  --service linkedin \
  --text "<text>" \
  --format-tag teaser \
  --image-url "<url>" \
  --image-alt "<title>"
```

Outputs validated JSON ready to pass as `mcp__buffer__create_post` args. Exits non-zero on validation failure (caller stops).

---

## Pattern: Per-skill format tag

Each compose skill always uses ONE format tag (the skill's identity is one-to-one with its tag value). When the skill calls `_shared/buffer-post-prep`, pass the **underscored key** as `--format-tag`. The binary maps it to the hyphenated `format:<name>` Buffer tag value:

| Skill | `--format-tag` value (underscored, what the binary expects) | Resulting Buffer tag |
|---|---|---|
| promote-newsletter | `verbatim_quote` | `format:verbatim-quote` |
| tease-newsletter | `teaser` | `format:teaser` |
| carousel-newsletter | `carousel` | `format:carousel` |
| promote-github (individual posts) | `link_share` | `format:link-share` |
| promote-github (batched) | `batch_summary` | `format:batch-summary` |
| crosspost-newsletter (companion Buffer announcement, if any) | `long_form_pulse` | `format:long-form-pulse` |

The keys match `_shared/format_tags.json` exactly. If you pass a hyphenated form (e.g. `link-share`), `buffer-post-prep` will fail with "invalid --format-tag".

**Two exceptions to the binary path:**
- **Carousel posts** bypass `buffer-post-prep` because they need 10-image asset arrays — different shape. Tag is still `format:carousel`, applied via direct `mcp__buffer__create_post` call with `tagIds: [<format:carousel Tag ID>]` (24-char hex from `_shared/buffer-post-prep/tag-ids.local.json` — Buffer's schema requires Tag IDs, not name strings; `tags: [...]` is silently dropped).
- **crosspost-newsletter** publishes directly to each platform's native editor (LinkedIn pulse, Substack, Medium, HN, Reddit) — none of these go through Buffer. The `format:long-form-pulse` tag is only applied if a future skill schedules a companion Buffer announcement post for the published article. Closed-loop attribution for the native-published versions instead comes from `linkedin-stats` (LinkedIn pulse) and platform-specific dashboards (Medium, HN, Reddit — not yet scraped).

---

## Pattern: React form input setter

When automating a web form built with React (Buffer's posting-goal field, LinkedIn's post composer, Medium's title input, Substack's subject line, etc.), setting `input.value = "X"` directly does NOT work. React tracks the previous value via its synthetic event system; assigning to `.value` updates the DOM but the React state — and therefore the underlying form submission — still holds the old value.

The fix is to invoke the **native** value setter (the one from `HTMLInputElement.prototype`, before React monkey-patched it) so React's `onChange` handler fires with the new value.

### Canonical pattern

```javascript
const input = /* find your input element */;
const proto = Object.getPrototypeOf(input);                          // HTMLInputElement.prototype
const setter = Object.getOwnPropertyDescriptor(proto, "value").set;  // native setter
setter.call(input, "<new value>");                                   // bypasses React's intercept
input.dispatchEvent(new Event("input", { bubbles: true }));          // notify React
input.dispatchEvent(new Event("change", { bubbles: true }));         // notify validators
input.blur();                                                        // commit (some forms auto-save on blur)
```

For `<textarea>` use `HTMLTextAreaElement.prototype` instead (same shape).

### When to use it

- **Programmatic value setting on any React-controlled input.** If you set `.value` directly and the form behaves like nothing changed (or reverts on submit), you've hit the React-controlled-input wall. Use this pattern.
- **Examples in this repo:** `_shared/buffer-schedule-edit/buffer-schedule-edit.sh` (`window.__setGoal`); HN submit form fix in `crosspost-newsletter` (per memory `feedback_clipboard_paste_pattern.md` — "batched gstack typing freezes the renderer; React-native value setter via JS works first try").

### When this is NOT the right tool

- **Radix popup menus / dropdowns** — these aren't `<input>` elements. Use `PointerEvent` + `MouseEvent` dispatch on the trigger button, then click the menu item (see `__radixClick` in `_shared/buffer-schedule-edit/`).
- **Contenteditable rich-text editors** (Medium body, LinkedIn post composer body) — these intercept paste and resist value injection. Use the `osascript HTML to NSPasteboard + cmd+v` pattern documented in memory `feedback_clipboard_paste_pattern.md`.
- **Form fields that auto-save on every keystroke and block automation** — these usually need full keystroke simulation via `$B type` (gstack browse), not value-set.

### Why this works

React's controlled inputs override the prototype's `value` setter to track changes. The override stores the new value in React's fiber state. Direct `.value = "X"` bypasses React's tracking — React still thinks the value is the old one, so its `onChange` doesn't fire and form submission uses the stale value. The native setter (`Object.getOwnPropertyDescriptor(proto, "value").set`) is the original, un-overridden setter; calling it with `setter.call(input, "X")` writes to the DOM AND triggers React's change-tracking when paired with the `input` event dispatch.

---

## Pattern: Voice grounding for original-copy generation

Any skill that generates **NEW copy** (not verbatim extraction, not full-article syndication) MUST inject the author's recent newsletters as a **voice corpus** into the compose-phase prompt. Without this, model output reverts to baseline corporate-LinkedIn voice — factually correct but tonally off (generic "shipped a thing!" energy instead of the author's slight-irreverent grounded-practical first-person voice).

### Skills using this pattern

| Skill | Phase | Notes |
|---|---|---|
| `/tease-newsletter` | Phase 4 | Original teaser hooks per channel |
| `/promote-github` | Phase 4 | Value/impact framing for GitHub posts. **No source article** — the corpus is the only voice anchor. |
| `/carousel-newsletter` | Phase 2 | Slides 1, 2, 4, 6, 8, 9 (hook, sections, stats) only. Quote slides 3/5/7 stay verbatim; CTA slide 10 is a fixed template. |

### Skills NOT in scope

- `/promote-newsletter` — verbatim quotes only, no original copy to voice-match
- `/crosspost-newsletter` — syndicates the article body to native editors; no original copy

### How

**Phase 1 (or earliest Phase the skill has) — fetch:**

```bash
_shared/voice-corpus/voice-corpus  # auto-refreshes if cache > 7 days old
```

Output: JSON with `posts: [{title, url, published_at, body_text}]`.

**Compose phase — inject + enforce:**

Prepend the voice-corpus output into the compose context as inline excerpts:

> The author's recent newsletters (full corpus from beehiiv RSS — all available; default ~12 posts truncated to 2000 chars each ≈ 24k total):
> ---
> [for each post] **<Title>** (<published_at>): <body_text>
> ---

Then state the voice-grounding rule alongside the existing CRITICAL RULES (verbatim, no fabrication, etc.) — same enforcement weight. The drafts MUST match: sentence rhythm, vocabulary preferences, recurring framings ("vibe coding", "agentic", "tokens from our past"), first-person stance, the slight-irreverent grounded-practical tone. **Mismatched voice is a fail signal — same weight as a fabrication.**

### Why a shared corpus, not per-skill

Voice doesn't vary by skill. Caching once amortizes the fetch cost across all skills in a run (`/carousel-newsletter` + `/tease-newsletter` + `/promote-github` in the same session re-use the same cache.json). Stale-after-7-days TTL keeps the corpus fresh without re-fetching every invocation.

### Why this isn't in the adversarial reviewer (yet)

Voice judgment is fuzzy ("does this sound like the author?") and hard to enforce mechanically — the reviewer would have a high false-positive rate. The composer is responsible; user review catches drift. If voice drift keeps surfacing across multiple runs, escalate to a v2 that passes the corpus to the reviewer with a "tone matches author voice" rule.
