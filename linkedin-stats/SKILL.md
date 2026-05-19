---
name: linkedin-stats
description: Use when user wants LinkedIn newsletter / profile / company-page stats — "linkedin stats", "how's my linkedin doing", "newsletter subscribers", "linkedin followers", "top linkedin posts", "linkedin dashboard", "weekly linkedin report".
user_invocable: true
---

# linkedin-stats

Scrape LinkedIn's Creator analytics pages via `gstack browse` and produce a one-shot report: newsletter subscriber count, profile follower count, company-page follower count, recent article/post engagement, and week-over-week deltas against cached snapshots.

Why this exists: Priority 3 of the growth plan is "cross-post the newsletter to LinkedIn every week." Today we have zero signal on whether that's working. This skill is the signal.

## Usage

`/linkedin-stats` — full report (all three surfaces + deltas)
`/linkedin-stats newsletter` — newsletter only (fast path)
`/linkedin-stats --no-cache` — skip writing the snapshot (for ad-hoc checks that shouldn't disturb trend tracking)
`/linkedin-stats --since YYYY-MM-DD` — compute delta against a specific snapshot instead of the 7-day default

## 🟢 Happy Path (read first; everything below is edge-case detail)

For a full `/linkedin-stats` run when nothing goes wrong. ~60-90 sec wall-clock. Each step links to a labeled edge case (`Edge: <name>`) you only need to read if that step fails.

**Phase 0 — Load config (2 sec).** Read `config.local.json` if present, else `config.json`. Pull `profile_url`, `company_url`, `newsletter_url`, `creator_analytics_url`, `post_limit`, `delta_window_days`. The newsletter URL has a random `<id>` suffix that must be in `config.local.json` — see `Edge: newsletter-url-shape`.

**Phase 1 — Browser + login check (5-10 sec).** Verify `gstack browse` (`$B`) binary exists. `$B goto linkedin.com/feed/` → `$B snapshot -i`. If "Email or phone" / "Sign in" markers appear, run `$B cookie-import-browser chrome linkedin.com` and retry. If still gated, `$B handoff` for manual login. Headless 999/403 responses also route through cookie re-import — see `Edge: headless-detection`.

**Phase 2 — Newsletter stats (10-15 sec).** `$B goto $NEWSLETTER_URL` → extract subscriber count via regex on `body.innerText` (`/([0-9,]+)\s+subscribers?/i`). Then `$B goto linkedin.com/dashboard/` (the new Creator analytics home; the old `/creator/dashboard/` 404s) → `sleep 4` → JS-grab the 6 metric tiles (`post_impressions`, `followers`, `profile_viewers`, `search_appearances`, `new_newsletter_subs`, `newsletter_article_views`) each with 7-day delta. Then `$B goto linkedin.com/analytics/creator/content/?timeRange=past_7_days` → `sleep 5` → JS-grab summary (impressions, members reached, social engagements, reactions, comments, reposts) + top-N posts list.

**Phase 3 — Profile followers (5 sec).** `$B goto $PROFILE_URL` → JS regex on `body.innerText` for `/([0-9,]+)\s+followers?/i`.

**Phase 4 — Company page followers (5 sec).** `$B goto $COMPANY_URL` → same follower regex. If the user is a page admin, deeper analytics live at `/company/<slug>/admin/analytics/visitors/`.

**Phase 5 — Delta vs cached snapshot (2 sec).** Look in `cache/` for newest `snapshot-*.json` older than `delta_window_days` (default 7). Diff today's `nl_subs / profile_followers / company_followers` against it. First run has no prior snapshot → deltas render as `—` (expected — see `Edge: delta-bootstrap`).

**Phase 6 — Render report.** Single markdown block: newsletter (subs + Δ + recent articles), profile (followers + Δ + top posts), company (followers + Δ + top page posts), then the Priority 3 growth-plan check (subs added this week vs target rate).

**Phase 7 — Write snapshot (1 sec).** Unless `--no-cache`, write `cache/snapshot-$(date -u +%Y-%m-%d).json` with `newsletter.subscribers`, `profile.followers`, `company.followers`, `fetched_at`. Cache dir is gitignored; `/flywheel` reads the newest snapshot here.

### Edge labels (jump to these only when you hit the matching failure signal)

| Label | Symptom |
|---|---|
| `Edge: selector-breakage` | A regex/JS grab returns null because LinkedIn re-skinned the page |
| `Edge: headless-detection` | Navigation returns 999 or 403, or a verification challenge appears |
| `Edge: newsletter-url-shape` | Newsletter URL missing the `<id>` suffix; subscriber count fails to parse |
| `Edge: paid-features-missing` | A field comes back blank because it requires LinkedIn Premium / Creator Pro |
| `Edge: delta-bootstrap` | Deltas render as `—` because there's no prior snapshot to compare against |
| `Edge: activity-feed-selector-drift` | Phase 3b returns 0 cards from the activity feed |
| `Edge: abbreviated-counts` | Phase 3b reactions/comments come back as `1.2K` instead of an int |
| `Edge: lazy-load-stalled` | Phase 3b scroll-loop captures fewer cards than `max_posts_per_scrape` |

Each label corresponds to a heading in **Known issues / robustness notes** below.

## Config

The skill reads config from (in priority order):
1. `~/dev/claude-social-media-skills/linkedin-stats/config.local.json` (gitignored — put personal URLs here)
2. `~/dev/claude-social-media-skills/linkedin-stats/config.json` (committed defaults)

Fields:
- `profile_url` — your LinkedIn profile page
- `company_url` — the EVC (or primary company) page
- `newsletter_url` — the newsletter's home page (`/newsletters/<slug-id>/`)
- `creator_analytics_url` — `https://www.linkedin.com/creator/dashboard/`
- `post_limit` — how many recent posts/articles to list (default 5)
- `delta_window_days` — compare against the newest snapshot older than this (default 7)

Load config at the start of every run:

```bash
CONFIG_DIR=~/dev/claude-social-media-skills/linkedin-stats
if [ -f "$CONFIG_DIR/config.local.json" ]; then CONFIG_FILE="$CONFIG_DIR/config.local.json"; else CONFIG_FILE="$CONFIG_DIR/config.json"; fi
PROFILE_URL=$(jq -r .profile_url "$CONFIG_FILE")
COMPANY_URL=$(jq -r .company_url "$CONFIG_FILE")
NEWSLETTER_URL=$(jq -r .newsletter_url "$CONFIG_FILE")
```

## Process

### Phase 1 — Initialize browser + verify login

Bring the `$B` (gstack browse) binary online, navigate to a known-protected LinkedIn page, and check for login markers. If not logged in, import cookies from Chrome; if that fails, hand off.

```bash
B=~/.claude/skills/gstack/browse/dist/browse
if [ ! -x "$B" ]; then echo "gstack browse not installed"; exit 1; fi

$B goto "https://www.linkedin.com/feed/"
$B snapshot -i > /tmp/ln-login-check.txt
# Logged-in markers: "Start a post", "My Network" button, "Messaging" link.
# Not-logged-in markers: "Sign in" button, "Email or phone" textbox.
if grep -qE '"Email or phone"|"Sign in"' /tmp/ln-login-check.txt; then
  echo "Not logged in — importing cookies from Chrome..."
  $B cookie-import-browser chrome linkedin.com
  $B goto "https://www.linkedin.com/feed/"
  $B snapshot -i > /tmp/ln-login-check.txt
  if grep -qE '"Email or phone"|"Sign in"' /tmp/ln-login-check.txt; then
    $B handoff "Please log in to LinkedIn in the browse tab — I'll continue once you're done."
    # user resumes; re-check after $B resume
  fi
fi
```

### Phase 2 — Newsletter stats

Navigate to the newsletter's about/analytics page. The subscriber count is on the public about page; the per-article stats are behind the Creator analytics view.

**Subscriber count** (from the public newsletter page):

```bash
$B goto "$NEWSLETTER_URL"
$B js "
  // The subscribers count is a text node near the 'subscribers' label.
  const text = document.body.innerText;
  const m = text.match(/([0-9,]+)\\s+subscribers?/i);
  m ? m[1].replace(/,/g, '') : null;
" > /tmp/ln-newsletter-subs.txt
NL_SUBS=$(cat /tmp/ln-newsletter-subs.txt | tr -d '"')
```

**Aggregated 7-day analytics** (from the new LinkedIn dashboard — confirmed 2026-04-27):

The old `https://www.linkedin.com/creator/dashboard/` and `/creator/analytics/posts/` URLs return 404 — LinkedIn moved Creator analytics. The replacement is `https://www.linkedin.com/dashboard/`, which exposes 6 metrics with 7-day deltas in plain text:

```bash
$B goto "https://www.linkedin.com/dashboard/"
sleep 4
$B js "
  const text = (document.querySelector('main') || document.body).innerText;
  // Each metric appears as: '<value>\n<label>\n<delta>%? past N days'
  const grab = (label) => {
    const re = new RegExp('([0-9,.]+)\\\\s*\\\\n\\\\s*' + label.replace(/ /g, '\\\\s+') + '\\\\s*\\\\n\\\\s*([0-9.]+%(?:\\\\s+past\\\\s+\\\\d+\\\\s+days)?)', 'i');
    const m = text.match(re);
    return m ? { value: m[1].replace(/,/g, ''), delta: m[2] } : null;
  };
  ({
    post_impressions: grab('Post impressions'),
    followers: grab('Followers'),
    profile_viewers: grab('Profile viewers'),
    search_appearances: grab('Search appearances'),
    new_newsletter_subs: grab('New newsletter subscribers'),
    newsletter_article_views: grab('Newsletter article views')
  })
"
```

This gives:
- `post_impressions` — total impressions across all posts (rolling 7d, with %Δ)
- `followers` — connection count + 7d delta
- `profile_viewers` — past 90 days
- `search_appearances` — previous calendar week
- `new_newsletter_subs` — **new subs added in past 7 days** (much better signal than the static subscriber count from the newsletter page)
- `newsletter_article_views` — total article views past 7 days

**Per-post engagement** (confirmed 2026-04-27):

```bash
$B goto "https://www.linkedin.com/analytics/creator/content/?timeRange=past_7_days"
sleep 5
$B js "
  const text = (document.querySelector('main') || document.body).innerText;
  // Top section gives Discovery + Engagement summary:
  //   1,839 Impressions, 647 Members reached
  //   15 Social engagements, 13 Reactions, 2 Comments, 0 Reposts, 0 Saves, 0 Sends
  // Below that, top posts list each appears as:
  //   '<N> impressions • <M> engagement\nView analytics\n<post type>\n<post text...>'
  const summary = {
    impressions: text.match(/Cumulative\\s+\\n\\s*([0-9,]+)/)?.[1]?.replace(/,/g, ''),
    members_reached: text.match(/([0-9,]+)\\s+Members reached/)?.[1]?.replace(/,/g, ''),
    social_engagements: text.match(/([0-9,]+)\\s+Social engagements/)?.[1]?.replace(/,/g, ''),
    reactions: text.match(/Reactions\\s+\\n\\s*([0-9,]+)/)?.[1]?.replace(/,/g, ''),
    comments: text.match(/([0-9,]+)\\s+Comments/)?.[1]?.replace(/,/g, ''),
    reposts: text.match(/([0-9,]+)\\s+Reposts/)?.[1]?.replace(/,/g, '')
  };
  const postRegex = /(\\d+(?:,\\d+)*)\\s+impressions\\s+•\\s+(\\d+(?:,\\d+)*)\\s+engagements?\\s*\\n+View analytics\\s*\\n+([^\\n]+)\\s*\\n+([\\s\\S]+?)(?=\\n\\d+(?:,\\d+)*\\s+impressions|$)/g;
  const posts = [];
  let m;
  while ((m = postRegex.exec(text))) {
    posts.push({
      impressions: parseInt(m[1].replace(/,/g, '')),
      engagements: parseInt(m[2].replace(/,/g, '')),
      type: m[3].trim().slice(0, 40),
      snippet: m[4].trim().slice(0, 200)
    });
  }
  ({ summary, top_posts: posts.slice(0, 10) })
"
```

Also discovered:
- `https://www.linkedin.com/analytics/creator/audience` — audience-side analytics (industry, job title, location breakdowns)
- `https://www.linkedin.com/analytics/newsletter/urn:li:fsd_contentSeries:<id>/?metricType=NEWSLETTER_SUBSCRIBERS` — per-newsletter subscriber detail (the `<id>` matches the newsletter URL slug)
- `https://www.linkedin.com/analytics/newsletter/urn:li:fsd_contentSeries:<id>/?metricType=ARTICLE_VIEWS` — per-newsletter article view detail

These deep-link directly from `/dashboard/` — find them by querying `a[href]` on the dashboard for `/analytics/` href patterns.

If selectors break in the future, fall back to screenshot + handoff:

```bash
$B screenshot /tmp/ln-analytics.png
$B handoff "LinkedIn dashboard DOM changed. Screenshot at /tmp/ln-analytics.png — please read the top metrics and paste here."
```

### Phase 3 — Profile follower count + top posts

```bash
$B goto "$PROFILE_URL"
$B js "
  const text = document.body.innerText;
  const m = text.match(/([0-9,]+)\\s+followers?/i);
  m ? m[1].replace(/,/g, '') : null;
" > /tmp/ln-profile-followers.txt
PROFILE_FOLLOWERS=$(cat /tmp/ln-profile-followers.txt | tr -d '"')
```

Top posts: from Creator analytics posts view (already loaded in Phase 2 if we're doing the full report) — or navigate to `https://www.linkedin.com/in/<handle>/recent-activity/all/` and pull the first N entries with engagement counts. The full per-post scrape lives in Phase 3b below.

### Phase 3b — Per-post engagement scrape (added 2026-05-19)

Closes the #1 closed-loop measurement gap: scrape `linkedin.com/in/<handle>/recent-activity/all/` for the last N posts and emit structured per-post engagement under `profile.recent_posts[]` in the snapshot. See `SPEC-per-post-scrape.md` for the design rationale.

Triggered as part of the default `/linkedin-stats` run (additive — Phase 3 still emits the aggregate follower count). Skipped when only `newsletter` is requested.

```bash
PROFILE_HANDLE=$(jq -r .profile_handle "$CONFIG_FILE")
MAX_POSTS=$(jq -r '.profile.max_posts_per_scrape // 20' "$CONFIG_FILE")
SKIP_REPOSTS=$(jq -r '.profile.skip_reposts // false' "$CONFIG_FILE")
RECENT_OUT=/tmp/ln-recent-posts.json

$B goto "https://www.linkedin.com/in/${PROFILE_HANDLE}/recent-activity/all/"
sleep 3

$B js "
  const MAX = ${MAX_POSTS};
  const container = document.querySelector('main') || document.scrollingElement;
  const sleep = (ms) => new Promise(r => setTimeout(r, ms));
  for (let i = 0; i < 6; i++) {
    container.scrollTop = container.scrollHeight;
    await sleep(1500);
    if (document.querySelectorAll('[data-id^=\"urn:li:activity:\"]').length >= MAX) break;
  }
  const seeMores = document.querySelectorAll('.feed-shared-inline-show-more-text button, button.see-more');
  for (const b of seeMores) { try { b.click(); } catch (_) {} }
  await sleep(400);

  const parseCount = (s) => {
    if (!s) return 0;
    const m = String(s).replace(/,/g, '').match(/([\\d.]+)\\s*([KMkm]?)/);
    if (!m) return 0;
    const n = parseFloat(m[1]);
    const suf = m[2].toUpperCase();
    return Math.round(suf === 'K' ? n * 1e3 : suf === 'M' ? n * 1e6 : n);
  };

  const cards = Array.from(document.querySelectorAll('[data-id^=\"urn:li:activity:\"]')).slice(0, MAX);
  const out = cards.map(card => {
    const urn = card.getAttribute('data-id');
    const timeEl = card.querySelector('.update-components-actor__sub-description time, time[datetime]');
    const posted_at = timeEl?.getAttribute('datetime') || null;
    const posted_at_relative = (card.querySelector('.update-components-actor__sub-description')?.innerText || '').trim().split('\\n')[0] || null;
    const textEl = card.querySelector('.feed-shared-inline-show-more-text, .update-components-text');
    let text = (textEl?.innerText || '').trim();
    const text_truncated_chars = text.length;
    text = text.replace(/…\\s*see more$/i, '').trim();

    const reactionsEl = card.querySelector('.social-details-social-counts__reactions-count, .social-details-social-counts__reactions span[aria-hidden=\"true\"]');
    const commentsEl = Array.from(card.querySelectorAll('.social-details-social-counts__comments, .social-details-social-counts__item')).find(e => /comment/i.test(e.innerText || ''));
    const repostsEl = Array.from(card.querySelectorAll('.social-details-social-counts__item')).find(e => /repost|share/i.test(e.innerText || ''));
    const reactions = parseCount(reactionsEl?.innerText);
    const comments = parseCount(commentsEl?.innerText);
    const reposts = parseCount(repostsEl?.innerText);

    let media_type = 'text';
    if (card.querySelector('.update-components-linkedin-video, video')) media_type = 'video';
    else if (card.querySelector('.update-components-image, img.update-components-image__image')) media_type = 'image';
    else if (card.querySelector('.update-components-article')) media_type = 'article';

    const is_repost = !!card.querySelector('.update-components-mini-update-v2, .update-components-header__text-view');

    let source_tag = null;
    const m = text.match(/\\[(opus|lp|gh|bh):([A-Za-z0-9_-]+)\\]/);
    if (m) source_tag = { scheme: m[1], id: m[2] };

    return {
      post_urn: urn,
      posted_at,
      posted_at_relative,
      text,
      text_truncated_chars,
      reactions,
      comments,
      reposts,
      engagement_total: reactions + comments + reposts,
      impressions: null,
      media_type,
      is_repost,
      source_tag
    };
  });
  JSON.stringify(out);
" > "$RECENT_OUT.raw"

jq -r 'fromjson? // .' "$RECENT_OUT.raw" > "$RECENT_OUT"

if [ "$SKIP_REPOSTS" = "true" ]; then
  jq '[.[] | select(.is_repost == false)]' "$RECENT_OUT" > "$RECENT_OUT.filtered" && mv "$RECENT_OUT.filtered" "$RECENT_OUT"
fi
```

The output JSON file `$RECENT_OUT` is then merged into the Phase 7 snapshot — see the updated Phase 7 snippet below.

#### Phase 3c — Per-post impressions (opt-in, `--with-impressions`)

Off by default (slow: one navigation per post, ~30-60 sec for 20 posts, and rate-limit-risky). Enable via the CLI flag `--with-impressions` OR by setting `profile.include_impressions: true` in config.

For each `post_urn` in `$RECENT_OUT`, navigate to `https://www.linkedin.com/analytics/post/<urn>/` and grab the impressions tile, then merge back into the same JSON:

```bash
INCLUDE_IMPRESSIONS=$(jq -r '.profile.include_impressions // false' "$CONFIG_FILE")
if [ "$WITH_IMPRESSIONS_FLAG" = "1" ] || [ "$INCLUDE_IMPRESSIONS" = "true" ]; then
  TMP_IMPR=/tmp/ln-impressions.json
  echo '{}' > "$TMP_IMPR"
  for URN in $(jq -r '.[].post_urn' "$RECENT_OUT"); do
    $B goto "https://www.linkedin.com/analytics/post/${URN}/"
    sleep 3
    IMPR=$($B js "
      const t = (document.querySelector('main') || document.body).innerText;
      const m = t.match(/([0-9,]+)\\s+Impressions/i);
      m ? parseInt(m[1].replace(/,/g, '')) : null;
    " | tr -d '"')
    jq --arg urn "$URN" --argjson v "${IMPR:-null}" '. + {($urn): $v}' "$TMP_IMPR" > "$TMP_IMPR.next" && mv "$TMP_IMPR.next" "$TMP_IMPR"
  done
  jq --slurpfile impr <(jq -s . "$TMP_IMPR") \
    '[.[] | . + {impressions: ($impr[0][0][.post_urn] // null)}]' "$RECENT_OUT" > "$RECENT_OUT.merged" && mv "$RECENT_OUT.merged" "$RECENT_OUT"
fi
```

#### Edge labels for Phase 3b/3c

| Label | Symptom |
|---|---|
| `Edge: activity-feed-selector-drift` | `[data-id^="urn:li:activity:"]` returns 0 cards (LinkedIn re-skinned the activity DOM) |
| `Edge: abbreviated-counts` | Reaction/comment counts come back as `1.2K` or `All` instead of an integer |
| `Edge: lazy-load-stalled` | Scroll-loop captures fewer cards than `max_posts_per_scrape` (Intersection Observer didn't fire) |
| `Edge: source-tag-missing` | Post is from OpusClip but `[opus:<id>]` not found in body (LinkedIn truncated the footer text) |
| `Edge: analytics-post-not-yours` | `/analytics/post/<urn>/` returns "not your post" — login session belongs to the wrong account |

See **Known issues / robustness notes** below for fixes.

### Phase 4 — Company page follower count + top posts

```bash
$B goto "$COMPANY_URL"
$B js "
  const text = document.body.innerText;
  const m = text.match(/([0-9,]+)\\s+followers?/i);
  m ? m[1].replace(/,/g, '') : null;
" > /tmp/ln-company-followers.txt
COMPANY_FOLLOWERS=$(cat /tmp/ln-company-followers.txt | tr -d '"')
```

Page admins have richer analytics at `/company/<slug>/admin/analytics/visitors/` — use if the user is a page admin.

### Phase 5 — Delta vs cached snapshot

```bash
CACHE_DIR=~/dev/claude-social-media-skills/linkedin-stats/cache
mkdir -p "$CACHE_DIR"

# Find the newest snapshot older than `delta_window_days` days.
TARGET_AGE=$(jq -r .delta_window_days "$CONFIG_FILE")
CUTOFF=$(date -v-${TARGET_AGE}d -u +%Y-%m-%d 2>/dev/null || date -d "$TARGET_AGE days ago" -u +%Y-%m-%d)
PRIOR_SNAP=$(ls -1 "$CACHE_DIR"/snapshot-*.json 2>/dev/null | awk -v c="snapshot-$CUTOFF" '$0 <= c' | tail -1)

if [ -n "$PRIOR_SNAP" ]; then
  PRIOR_NL=$(jq -r .newsletter.subscribers "$PRIOR_SNAP")
  PRIOR_PROFILE=$(jq -r .profile.followers "$PRIOR_SNAP")
  PRIOR_COMPANY=$(jq -r .company.followers "$PRIOR_SNAP")
  NL_DELTA=$((NL_SUBS - PRIOR_NL))
  PROFILE_DELTA=$((PROFILE_FOLLOWERS - PRIOR_PROFILE))
  COMPANY_DELTA=$((COMPANY_FOLLOWERS - PRIOR_COMPANY))
else
  NL_DELTA=""; PROFILE_DELTA=""; COMPANY_DELTA=""
fi
```

### Phase 6 — Render report

```
LinkedIn — weekly snapshot (YYYY-MM-DD)

Newsletter (Enterprise Vibe Code):
  Subscribers:   N (+Δ vs last week)
  Recent articles:
    YYYY-MM-DD  "<title>"   V views · R reactions · C comments

Profile (Mike Lady):
  Followers:     N (+Δ)
  Top posts (last 30d):
    "<snippet>"  N impressions · R reactions

Company page (Enterprise Vibe Code):
  Followers:     N (+Δ)
  Top page posts (last 30d):
    "<snippet>"  N impressions · R reactions

Growth plan Priority 3 check:
  Newsletter subs added this week: Δ
  Target rate to hit [plan target] subs in 12 months: ~X/week
  Status: [on track | behind | ahead]
```

### Phase 7 — Write snapshot (unless `--no-cache`)

```bash
SNAP_PATH="$CACHE_DIR/snapshot-$(date -u +%Y-%m-%d).json"
RECENT_JSON=$([ -s /tmp/ln-recent-posts.json ] && cat /tmp/ln-recent-posts.json || echo '[]')
jq -n \
  --argjson nl_subs "$NL_SUBS" \
  --argjson profile_fol "$PROFILE_FOLLOWERS" \
  --argjson company_fol "$COMPANY_FOLLOWERS" \
  --argjson recent_posts "$RECENT_JSON" \
  '{
    fetched_at: (now | todateiso8601),
    newsletter: { subscribers: $nl_subs },
    profile: { followers: $profile_fol, recent_posts: $recent_posts },
    company: { followers: $company_fol }
  }' > "$SNAP_PATH"
```

The cache directory is gitignored — snapshots stay local, private.

## Known issues / robustness notes

- **LinkedIn re-skins their analytics pages frequently.** The DOM selectors above are best-effort. When a selector breaks, the skill should fall back to `$B screenshot` + `$B handoff` so the user can eyeball the number rather than silently returning a stale value.
  *Label: `Edge: selector-breakage`*
- **Headless detection.** LinkedIn sometimes shows an extra verification challenge to gstack browse. If navigation returns 999 or 403, run `$B cookie-import-browser chrome linkedin.com` and retry. If still blocked, the user may need to run the skill from Claude in Chrome instead (see `crosspost-newsletter` for the pattern).
  *Label: `Edge: headless-detection`*
- **Newsletter URL shape.** LinkedIn newsletters have a random `<id>` suffix in the URL — copy it from the address bar once and save to `config.local.json` so the skill doesn't have to guess.
  *Label: `Edge: newsletter-url-shape`*
- **Paid features.** Some analytics (e.g., detailed impression breakdowns) require LinkedIn Premium / Creator Mode Pro. The skill reports what's visible to the current login; missing data is noted as `unknown` not silently dropped.
  *Label: `Edge: paid-features-missing`*
- **Delta bootstrap.** The first run has no prior snapshot so deltas render as `—`. After one week of snapshots the numbers mean something.
  *Label: `Edge: delta-bootstrap`*
- **Activity-feed selector drift (Phase 3b).** If `[data-id^="urn:li:activity:"]` returns 0 cards, fall back to `[id^="urn:li:activity:"]`, and if that also misses, hunt by `time[datetime]` ancestor — every post card has one. As a last resort, `$B screenshot` + `$B handoff` and skip `recent_posts` for the run (snapshot still writes with `recent_posts: []`).
  *Label: `Edge: activity-feed-selector-drift`*
- **Abbreviated counts (Phase 3b).** LinkedIn renders `1.2K` / `3.4M` once a count crosses 1000. The `parseCount` helper in Phase 3b handles this — multiplies by 1e3/1e6 based on the suffix. If a new suffix appears (`B`?), extend the regex.
  *Label: `Edge: abbreviated-counts`*
- **Lazy-load stalled (Phase 3b).** If the scroll loop finishes with fewer cards than `max_posts_per_scrape`, the Intersection Observer probably bound to a different scroll container. Try `document.scrollingElement` and each `[role="main"]` descendant in turn. Capturing fewer posts is not fatal — the snapshot just has a shorter list.
  *Label: `Edge: lazy-load-stalled`*

## Feeds into Phase D (/flywheel)

The `/flywheel` skill (Phase D of the metrics plan) reads the most recent snapshot from `cache/` to pull LinkedIn numbers into the weekly rollup without needing to run this skill every time. That's why we cache.
