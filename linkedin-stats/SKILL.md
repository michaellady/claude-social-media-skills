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

Top posts: from Creator analytics posts view (already loaded in Phase 2 if we're doing the full report) — or navigate to `https://www.linkedin.com/in/<handle>/recent-activity/all/` and pull the first N entries with engagement counts.

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
jq -n \
  --argjson nl_subs "$NL_SUBS" \
  --argjson profile_fol "$PROFILE_FOLLOWERS" \
  --argjson company_fol "$COMPANY_FOLLOWERS" \
  '{
    fetched_at: (now | todateiso8601),
    newsletter: { subscribers: $nl_subs },
    profile: { followers: $profile_fol },
    company: { followers: $company_fol }
  }' > "$SNAP_PATH"
```

The cache directory is gitignored — snapshots stay local, private.

## Known issues / robustness notes

- **LinkedIn re-skins their analytics pages frequently.** The DOM selectors above are best-effort. When a selector breaks, the skill should fall back to `$B screenshot` + `$B handoff` so the user can eyeball the number rather than silently returning a stale value.
- **Headless detection.** LinkedIn sometimes shows an extra verification challenge to gstack browse. If navigation returns 999 or 403, run `$B cookie-import-browser chrome linkedin.com` and retry. If still blocked, the user may need to run the skill from Claude in Chrome instead (see `crosspost-newsletter` for the pattern).
- **Newsletter URL shape.** LinkedIn newsletters have a random `<id>` suffix in the URL — copy it from the address bar once and save to `config.local.json` so the skill doesn't have to guess.
- **Paid features.** Some analytics (e.g., detailed impression breakdowns) require LinkedIn Premium / Creator Mode Pro. The skill reports what's visible to the current login; missing data is noted as `unknown` not silently dropped.
- **Delta bootstrap.** The first run has no prior snapshot so deltas render as `—`. After one week of snapshots the numbers mean something.

## Feeds into Phase D (/flywheel)

The `/flywheel` skill (Phase D of the metrics plan) reads the most recent snapshot from `cache/` to pull LinkedIn numbers into the weekly rollup without needing to run this skill every time. That's why we cache.
