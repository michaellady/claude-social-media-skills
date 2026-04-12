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

### Phase 1 — Fetch GitHub Data

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
  --jq '.items[] | {sha: .sha[:7], message: .commit.message, html_url, repo: .repository.full_name, private: .repository.private}'
```
Discard any result where `private` is `true` (belt-and-suspenders — `is:public` should already filter, but verify).

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

### Phase 3 — User Selection and Post Mode

Ask the user two questions:

1. **Which contributions to promote?** (by number, e.g., "1, 3, 4" or "all")
2. **Post mode:**
   - **Batch** — one summary post covering all selected contributions
   - **Individual** — one separate post per contribution

**Wait for user input before proceeding.**

### Phase 4 — Compose Platform-Specific Posts

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

Repeat for each channel. Ask: **"Ready to post these to Buffer?"**

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
4. For each approved post, call `mcp__buffer__create_post` with:
   - `channelId`: exact ID from `list_channels`
   - `text`: the composed post text (impact statement + GitHub link)
   - `mode`: `"direct"`
   - `schedulingType`: `"direct"`
   - `assets`: only if user provided an image — `{ images: [{ url: "<image-url>", metadata: { altText: "<contribution-title>" } }] }`
   - Platform-specific `metadata`:
     - **Facebook**: `metadata.facebook.type: "post"`
     - **Instagram**: `metadata.instagram.type: "post"`, `metadata.instagram.shouldShareToFeed: true`
     - **Pinterest**: `metadata.pinterest.boardServiceId` (get from `get_channel` response, under `metadata.boards[].serviceId`)
     - **LinkedIn, Twitter, Threads, Bluesky, Mastodon**: no extra metadata required
5. **Rate limiting:** Buffer's API enforces rate limits (HTTP 429). When rate limited:
   - Stop immediately — do not retry in a loop.
   - Save all remaining posts (post text, channel IDs, image URLs if any) to `remaining-posts.md` in the project directory so they can be posted in a later session.
   - Report to the user which posts succeeded and which are saved for later.
6. Report results per channel: success or error message.
