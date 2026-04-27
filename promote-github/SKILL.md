---
name: promote-github
description: Use when user wants to promote their GitHub contributions on social media, create social posts from new repos, commits, PRs, or releases, or post GitHub activity to Buffer channels — "promote my github", "share what I shipped", "post about my PRs", "promote this PR", "promote this repo".
user_invocable: true
---

# promote-github

Fetch public GitHub contributions (new repos, merged PRs, commits, releases) and create platform-specific social media posts via Buffer MCP. Frame every post around value and impact — not technical jargon.

## Usage

`/promote-github today` — contributions from today
`/promote-github this-week` — contributions from the past 7 days
`/promote-github 2026-03-01..2026-03-30` — contributions in a date range
`/promote-github https://github.com/user/repo/pull/123` — a specific PR, commit, or release URL

## Process

### Phase 1 — Fetch GitHub Data + Voice Corpus

**Also fetch the voice corpus** (recent newsletters, used as voice reference in Phase 4 — there's no source article for GitHub posts, so the corpus is the ONLY voice anchor):

```bash
_shared/voice-corpus/voice-corpus  # auto-refreshes if cache > 7 days old
```

Output is JSON with `posts: [{title, url, published_at, body_text}]`. Hold onto this output for Phase 4. See [PATTERNS.md#pattern-voice-grounding-for-original-copy-generation](../PATTERNS.md#pattern-voice-grounding-for-original-copy-generation) for the rationale.

**Detect the authenticated GitHub user:**
```bash
gh api user --jq '.login'
```

**Parse the invocation argument** to determine the mode:
- If the argument is a URL (starts with `https://github.com/`), use **Specific URL Mode**.
- Otherwise, use **Time Range Mode**.

---

**Time Range Mode:**

Convert the argument to a GitHub search date qualifier:
- `today` → `>YYYY-MM-DD` using today's date
- `this-week` → `>YYYY-MM-DD` using the date 7 days ago
- `YYYY-MM-DD..YYYY-MM-DD` → use as-is

Fetch four data sources. Run all in parallel:

**1. Merged PRs:**
```bash
gh api "search/issues?q=author:{username}+type:pr+is:merged+is:public+merged:{date_qualifier}&sort=updated&per_page=100" \
  --jq '.items[] | {title, html_url, body: .body[:500], closed_at, repository_url}'
```

**2. Commits:**
```bash
gh api "search/commits?q=author:{username}+committer-date:{date_qualifier}+is:public&sort=committer-date&per_page=100" \
  --jq ".items[] | select(.repository.owner.login == \"{username}\") | {sha: .sha[:7], message: .commit.message, html_url, repo: .repository.full_name, private: .repository.private}"
```
**Filter to user-owned repos only** (`.repository.owner.login == username`). Without this, the search returns spam from public repos where the user's email happens to appear in commits (e.g. `ading2210-alt/bad-apple-git` returned 100+ unrelated frame-by-frame commits in the 2026-04-27 run). Also discard any result where `private` is `true` (belt-and-suspenders — `is:public` should already filter, but verify).

**Important: this skill SHOULD pick up commits across ALL of the user's owned repos, not just the cwd.** The 2026-04-27 run accidentally fell back to local `git log` of one repo and missed `michaellady/youtube-analytics`'s same-day commit. Always use the `search/commits` API path; never substitute `git log` of the current directory.

**3. Releases:**
First, get the user's public repos updated in the date range:
```bash
gh api "users/{username}/repos?type=public&sort=updated&per_page=100" --jq '.[].full_name'
```
Then for each repo, fetch releases published in the date range:
```bash
gh api "repos/{owner}/{repo}/releases?per_page=30" \
  --jq '.[] | select(.published_at >= "{start_date}") | {tag_name, name, html_url, published_at, body: .body[:500]}'
```

**4. New Repos Created:**
Fetch repos created by the user in the date range:
```bash
gh api "search/repositories?q=user:{username}+created:{date_qualifier}&sort=created&per_page=100" \
  --jq '.items[] | select(.private == false) | {full_name, html_url, description, created_at, language}'
```
For each new repo found, also fetch its commits to understand what was included at creation:
```bash
gh api "repos/{owner}/{repo}/commits?per_page=100" \
  --jq '[.[] | {sha: .sha[:7], message: .commit.message}]'
```
Present the new repo as a single contribution with its description and a summary of what was included (based on commit messages). This gives context for composing an impact-framed post about launching the project.

**Deduplicate:**
- If a commit belongs to a repo that was newly created in the same date range, drop the standalone commit — it will be covered by the "New Repo" entry.
- If a commit's repo and date overlap with a merged PR in the same repo, prefer the PR as the higher-level unit of work. Drop the standalone commit entry.

---

**Specific URL Mode:**

Parse the GitHub URL to extract `{owner}`, `{repo}`, and the resource type.

**First, verify the repo is public:**
```bash
gh api "repos/{owner}/{repo}" --jq '.private'
```
If `true`, stop and tell the user: "That repository is private. This skill only promotes public contributions to avoid leaking private work."

Then fetch based on URL pattern:

- **PR** (`/pull/{number}`):
  ```bash
  gh pr view {url} --json title,body,url,state,mergedAt,additions,deletions,headRefName
  ```

- **Commit** (`/commit/{sha}`):
  ```bash
  gh api "repos/{owner}/{repo}/commits/{sha}" \
    --jq '{sha: .sha[:7], message: .commit.message, html_url, stats: .stats}'
  ```

- **Release** (`/releases/tag/{tag}`):
  ```bash
  gh api "repos/{owner}/{repo}/releases/tags/{tag}" \
    --jq '{tag_name, name, html_url, published_at, body: .body[:500]}'
  ```

### Phase 2 — Identify and Summarize Contributions

Present fetched contributions as a numbered list, grouped by type:

```
New Repos:
1. [claude-social-media-skills] "Claude Code skills for promoting your work on social media via Buffer" — created Mar 30
   Includes: promote-github skill, promote-newsletter skill, README
   → https://github.com/user/claude-social-media-skills

Merged PRs:
2. [repo-name] "Add real-time notifications" — merged Mar 28
   → https://github.com/user/repo/pull/12
3. [repo-name] "Fix Docker arch detection" — merged Mar 27
   → https://github.com/user/repo/pull/2548

Commits (not part of a merged PR or new repo):
4. [repo-name] "Update README with setup docs" — Mar 29
   → https://github.com/user/repo/commit/36cca4d

Releases:
5. [repo-name] v0.13.0 — published Mar 29
   → https://github.com/user/repo/releases/tag/v0.13.0
```

If no contributions are found, tell the user and suggest a wider date range.

**Before presenting the list, check BOTH the Buffer queue AND recently-sent posts for duplicates.**

Users often run this skill on a regular cadence. Two failure modes to prevent:
1. **Queue overlap** — re-promoting something already in the Buffer queue → noisy double-post.
2. **Recently-sent overlap** — re-promoting something that just went out in the last few days → looks like you're padding the feed.

Annotate each contribution with its status so the user can skip duplicates.

1. Call `mcp__buffer__get_account` to get the organization ID. If Buffer MCP is unreachable, skip this check and warn the user at the top of the list: *"Buffer duplicate check skipped — unable to reach Buffer MCP. Queue and recent posts may overlap."* Then proceed without annotations.

2. **Queue check** — call `mcp__buffer__list_posts` with:
   - `organizationId`: from step 1
   - `status`: `["scheduled", "needs_approval", "draft"]`
   - `first: 100`
   - `sort: [{field: "dueAt", direction: "asc"}]`

3. **Recently-sent check** — call `mcp__buffer__list_posts` with:
   - `organizationId`: from step 1
   - `status`: `["sent"]`
   - `first: 100`
   - `sort: [{field: "dueAt", direction: "desc"}]`
   - `dueAt`: `{start: <7 days ago ISO>, end: <now ISO>}` — scoped window prevents scanning years of history. 7 days is the default "still feels recent" window; widen if the user explicitly asks.

4. For each call: if the response exceeds the tool-result size limit, save-to-file is automatic — use `jq` to extract only `{dueAt, channelService, text}` per post. You're scanning text, not rendering the full payload.

5. For each GitHub contribution in the list, match against both sets using the same substring rules:
   - The **repo slug** (e.g., `beehiiv-mcp`, `claude-social-media-skills`)
   - The **release tag** (e.g., `v0.0.2`) if this contribution is a release
   - A **distinctive phrase** from the contribution — skill name (`linkedin-stats`, `/flywheel`), PR number, or a unique noun phrase from the title/commit message
   - Be specific — don't flag on common words like "shipped" or "skill" alone.

6. Annotate each line in the list with a status tag. **Recent-sent overlap takes precedence over queue overlap** (sent is more final than queued):
   - `✅ new` — zero matching queued or recent-sent posts
   - `⚠️ sent Nx (most recent: YYYY-MM-DD)` — N matching posts went out within the last 7 days; show the most recent `dueAt`
   - `⚠️ queued Nx (earliest YYYY-MM-DD)` — N matching queued posts; show the soonest `dueAt`
   - `⚠️ sent + queued` — both; combine the counts and show both dates
   - `⚠️ partially queued — <note>` — a related item is queued/sent but this specific variant isn't (e.g., *v0.0.1 launch posted, but v0.0.2 release is new*). Keep the note short.

### Phase 3 — User Selection and Post Mode

Ask the user two questions:

1. **Which contributions to promote?** (by number, e.g., "1, 3, 4" or "all")
   **Default recommendation: only `✅ new` items.** For `⚠️ sent` items, strongly recommend skipping — the audience just saw this. For `⚠️ queued` items, recommend skipping unless the user opts in with a fresh angle. Only include `⚠️` items when the user explicitly confirms a distinguishing angle vs. what's already out or scheduled.
2. **Post mode:**
   - **Batch** — one summary post covering all selected contributions
   - **Individual** — one separate post per contribution

**Wait for user input before proceeding.**

### Phase 4 — Compose Platform-Specific Posts

**VOICE GROUNDING (read this BEFORE writing any draft):**

Before composing, prepend the Phase 1 voice-corpus output as inline excerpts in your working context, framed as:

> The author's recent newsletters (sample of the last 5):
> ---
> [for each post in the corpus] **<Title>** (<published_at>): <body_text>
> ---

GitHub posts have **no source article** — the voice corpus is the ONLY anchor for matching the author's voice. Posts that read like generic "shipped a thing!" tech-Twitter copy fail this rule.

The drafts you produce MUST sound like a continuation of this voice. Match:
- **Sentence rhythm** — short-to-medium with the occasional intentional fragment
- **Vocabulary preferences** — specific phrases the author actually uses (avoid LinkedIn-corporate-speak unless the author uses it)
- **Recurring framings** — e.g. "vibe coding", "agentic", "tokens from our past", "compounding taste"
- **First-person stance** — "I shipped" / "I just wired" not "We're excited to announce"
- **Tone** — slight irreverence + grounded practicality + first-hand observation

**Mismatched voice is a fail signal — same weight as a fabrication.** See [PATTERNS.md#pattern-voice-grounding-for-original-copy-generation](../PATTERNS.md#pattern-voice-grounding-for-original-copy-generation).

**CRITICAL RULE — Value/Impact Framing:**
Frame every post around what the contribution does for users or the project — not the technical implementation. Read the PR body, commit message, or release notes to understand the "why," then write a plain-language impact statement.

| Do this | Not this |
|---------|----------|
| "Just shipped retry logic so webhooks stop silently failing" | "Merged PR #42: added retry logic with exponential backoff" |
| "Released v2.0 — 3x faster builds and a new plugin system" | "Tagged v2.0.0, 47 files changed across 12 commits" |
| "Fixed the bug where signup emails vanished into the void" | "Fixed null pointer exception in EmailService.send()" |

Use the PR title, body, commit message, or release notes as source material to understand the change, then compose an original impact-framed statement.

**Individual post structure:**
```
[Impact statement — 1-2 sentences]

[GitHub URL]
```

**Batch post structure:**
```
[Theme sentence — what unifies these contributions]

[Impact line for contribution 1]
[Impact line for contribution 2]
[Impact line for contribution 3]

[Link to GitHub profile or most significant contribution]
```

**Character limits** (the GitHub URL is typically 60-90 chars — account for it in the budget):

| Platform   | Limit | Post budget (after ~80 char URL + 7 margin) |
|------------|-------|----------------------------------------------|
| Twitter/X  | 280   | ~193 chars for text                          |
| Bluesky    | 300   | ~213 chars for text                          |
| Pinterest  | 300   | ~213 chars for text                          |
| Threads    | 500   | ~413 chars for text                          |
| Facebook   | 500   | ~413 chars for text                          |
| Mastodon   | 500   | ~413 chars for text                          |
| Instagram  | 2,200 | ~2,113 chars for text                        |
| TikTok     | 2,200 | ~2,113 chars for text                        |
| LinkedIn   | 3,000 | ~2,913 chars for text                        |

Calculate the actual GitHub URL length for each contribution to determine exact budgets. The budget column above is an estimate — always count the real URL.

For short-form platforms (Twitter, Bluesky, Pinterest): single-sentence impact + link. Be ruthless about brevity.
For long-form platforms (LinkedIn, Instagram): expand with context — what problem it solves, why it matters, what you learned.
For batch posts on short-form platforms: theme sentence + link to profile. Individual contribution lines will not fit.

**Media:**
- GitHub contributions do not have built-in images. Posts are text-only by default.
- If the user provides an image URL, attach it using the same `assets.images` format as other skills.
- **Instagram requires an image** — skip Instagram for text-only posts.
- **Skip TikTok and YouTube** — they require video assets.

### Adversarial review (REQUIRED before user review)

Apply the **[Adversarial Review pattern](../PATTERNS.md#pattern-adversarial-review)** with these per-skill specifics:

- **SOURCE_LABEL:** "GITHUB CONTRIBUTIONS BEING PROMOTED"
- **SOURCE_CONTENT:** for each contribution — full title, full body/commit message, files changed, additions/deletions, canonical URL
- **SKILL_NAME:** `promote-github`
- **ARTIFACT_NAME:** "post"
- **RULES_LIST:**
  - Posts must be VALUE/IMPACT-framed, not technical. ("Just shipped retry logic so webhooks stop silently failing" not "Merged PR #42: added retry logic with exponential backoff").
  - Claims must be SUPPORTED by the contribution body. No invented features, no inflated metrics.
  - BANNED: emoji unless requested.
  - BANNED: claims about adoption/users/engagement that the source doesn't support ("everyone is using this", "popular among X").
  - REQUIRED: every post links to the canonical GitHub URL.
  - REQUIRED: factual accuracy — if the post says "5 of 5 platforms shipped," verify the source supports 5/5.
  - For batched posts: theme sentence must accurately unify the listed contributions, not stretch them into a narrative they don't support.
- **ISSUE_GUIDANCE:** "For unsupported claims, quote the claim and explain why the source doesn't support it. For technical-jargon framing, quote the post and suggest the value/impact reframe. For inflated metrics, quote the number and what the source actually shows."

### Phase 5 — Review Before Publishing

Present all drafted posts for approval:

```
Channel: @handle (Twitter)
Post (237/280 chars):
---
Just shipped retry logic so webhooks stop silently failing.

https://github.com/user/repo/pull/42
---
```

Repeat for each channel. Ask: **"Ready to publish these now?"** (Default is instant-publish via `shareNow`. If the user wants to queue instead, they can say "queue them" — then use `addToQueue` in Phase 6.)

The user can:
- Approve all
- Edit individual posts
- Skip specific channels
- Cancel entirely

### Phase 6 — Post to Buffer

1. Call `mcp__buffer__get_account` to get the organization ID and timezone. If multiple orgs, ask the user which one.
2. Call `mcp__buffer__list_channels` with the org ID. Never guess channel IDs.
3. **Filter out disconnected and locked channels before composing or posting.** Each channel has `isDisconnected` and `isLocked` booleans — skip any where either is `true`. Also skip `service: "startPage"` (not a social channel).
   ```
   const usable = channels.filter(c =>
     !c.isDisconnected && !c.isLocked && c.service !== 'startPage'
   );
   ```
   Silently omitting disconnected channels prevents wasted API calls and avoids posting to channels the user can't actually see.

Apply the **[Buffer create_post pattern](../PATTERNS.md#pattern-buffer-create_post-with-channel-filter--caps)** for the transport layer. Two key differences from the newsletter skills:

- **Mode:** `--mode shareNow` (this skill defaults to instant-publish, not queue). User overrides with explicit "queue it" → `--mode addToQueue`.
- **Format tag:** `--format-tag link_share` for individual contribution posts, `--format-tag batch_summary` for batched theme posts (note: underscored keys — the binary rejects the hyphenated form). The skill picks one per post based on whether the user chose individual vs batch mode in Phase 3.

Example invocation per individual post:

```bash
_shared/buffer-post-prep/buffer-post-prep \
  --channel-id <id> \
  --service linkedin \
  --text "<impact statement + GitHub URL>" \
  --format-tag link_share \
  --mode shareNow
```

For each approved post, build args via `_shared/buffer-post-prep/buffer-post-prep`, then call `mcp__buffer__create_post` with the resulting JSON. The transport layer enforces:

- Skips disconnected/locked/startPage channels
- Skips channels below `min_followers_to_promote = 50`
- Validates the post text against the platform char limit
- Attaches the appropriate `tags: ["format:link-share"]` or `tags: ["format:batch-summary"]`
- Sets platform-specific metadata correctly

**Rate limiting:** Buffer's API enforces rate limits (HTTP 429). When rate limited:
- Stop immediately — do not retry in a loop.
- Save all remaining posts (post text, channel IDs, image URLs if any) to `remaining-posts.md` in the project directory so they can be posted in a later session.
- Report to the user which posts succeeded and which are saved for later.

Report results per channel: success or error message.
