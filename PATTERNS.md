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

## Pattern: Adversarial review (Phase 4.5)

Every compose-and-publish skill spawns a fresh subagent to audit drafted posts BEFORE the user reviews them. The reviewer has no context from the compose phase — fresh eyes catch fabrications and skill-rule violations the composer might have rationalized.

### Generic prompt scaffold

```
You are an adversarial reviewer for /<<SKILL_NAME>> posts. Your job is to find problems before the user has to.

<<SOURCE_LABEL>>:
<<SOURCE_CONTENT>>

SKILL RULES (must be enforced):
<<RULES_LIST>>

DRAFTED <<ARTIFACT_NAME>>:
<<DRAFTS>>

For each draft, return:
- VERDICT: PASS or FAIL
- ISSUES: array of specific problems. Cite exact strings.
  <<ISSUE_GUIDANCE>>

Return only the JSON: {"verdict": [...], "issues": [...]} per draft.
```

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

The user caught a fabrication ("every leader I respect keeps a token on their desk") manually on the 2026-04-26 Tokens From Our Past run. Phase 4.5 prevents the next one automatically. This is the only way to scale up promotion volume without scaling up the user's review burden.

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
- **Carousel posts** bypass `buffer-post-prep` because they need 10-image asset arrays — different shape. Tag is still `format:carousel`, applied via direct `mcp__buffer__create_post` call with `tags: ["format:carousel"]`.
- **crosspost-newsletter** publishes directly to each platform's native editor (LinkedIn pulse, Substack, Medium, HN, Reddit) — none of these go through Buffer. The `format:long-form-pulse` tag is only applied if a future skill schedules a companion Buffer announcement post for the published article. Closed-loop attribution for the native-published versions instead comes from `linkedin-stats` (LinkedIn pulse) and platform-specific dashboards (Medium, HN, Reddit — not yet scraped).
