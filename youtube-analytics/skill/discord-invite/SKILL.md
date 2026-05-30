---
name: discord-invite
description: Use when user wants a Discord invite (or other lead-magnet) URL with UTM parameters auto-filled from the latest live stream — "discord link", "discord invite", "give me the stream link", "UTM link for stream", "newsletter link for this stream".
user_invocable: true
---

# discord-invite

Generate a Discord invite URL (or other lead-magnet URL) with `utm_source`, `utm_medium`, and `utm_campaign` pre-filled from the current or most-recent live stream. Drop the result into the stream chat or description so beehiiv can attribute signups back to the exact episode.

## Usage

The skill has two axes:
- **Context** (where the link will be clicked from) — sets `utm_source` + `utm_medium`
- **Target** (what URL the user lands on) — the actual destination

### Context shortcuts

| Invocation | utm_source | utm_medium | Campaign slug from |
|---|---|---|---|
| `/discord-invite` *(default)* | youtube | livestream | latest YouTube live |
| `/discord-invite <youtube-url>` | youtube | livestream | that specific video |
| `/discord-invite <video-id>` | youtube | livestream | that bare-ID video |
| `/discord-invite from-beehiiv` | beehiiv | newsletter | latest Beehiiv post title |
| `/discord-invite from-linkedin` | linkedin | newsletter | latest Beehiiv post title (same issue, different context) |

### Target override (appended to any context)

| Flag | Destination |
|---|---|
| *(default)* | Discord invite (`enterprisevibecode.com/discord-server-invite`) |
| `target=newsletter` | Beehiiv signup page (`enterprisevibecode.com/subscribe`) |

### Other overrides

- `/discord-invite --campaign my-custom-slug` — force a specific slug, bypass auto-derivation
- `/discord-invite --post <beehiiv-post-id>` — with `from-beehiiv`/`from-linkedin`, tag to a specific past newsletter issue instead of the latest

Accepted YouTube URL shapes (all extract the same 11-char video ID):
- `https://www.youtube.com/watch?v=VIDEOID`
- `https://youtu.be/VIDEOID`
- `https://www.youtube.com/live/VIDEOID`
- `VIDEOID` (bare)

## Lead magnets (edit in this file to add more)

| Target | Base URL |
|---|---|
| `discord` (default) | `https://www.enterprisevibecode.com/discord-server-invite` |
| `newsletter` | `https://www.enterprisevibecode.com/subscribe` |

To add a lead magnet, append to the table above AND the `case` block in Phase 3 below.

## Process

### Phase 0 — Resolve the context (source + medium)

Decide which source/medium to stamp on the link:

```bash
# Default: YouTube live stream.
UTM_SOURCE="youtube"
UTM_MEDIUM="livestream"
SLUG_FROM="live"   # latest YouTube live

case "$CONTEXT_ARG" in
  from-beehiiv)
    UTM_SOURCE="beehiiv"
    UTM_MEDIUM="newsletter"
    SLUG_FROM="beehiiv-post" ;;
  from-linkedin)
    UTM_SOURCE="linkedin"
    UTM_MEDIUM="newsletter"
    SLUG_FROM="beehiiv-post"   # same issue, just different channel
    ;;
esac
```

If `SLUG_FROM == "beehiiv-post"`, the skill fetches the most recent Beehiiv post title via `mcp__beehiiv__beehiiv_stats` (or accepts `--post <id>` for a specific past issue) and uses that title for the slug. Otherwise it falls through to Phase 1 to resolve the YouTube stream.

### Phase 1 — Resolve the stream context

Three paths depending on what the user passed:

**A. Explicit YouTube URL or video ID (positional arg):**

Extract the 11-char video ID from any of the accepted URL shapes:

```bash
extract_video_id() {
  local input="$1"
  # Already a bare 11-char ID?
  if printf '%s' "$input" | grep -Eq '^[A-Za-z0-9_-]{11}$'; then
    printf '%s' "$input"
    return 0
  fi
  # watch?v=ID, youtu.be/ID, youtube.com/live/ID, youtube.com/shorts/ID
  printf '%s' "$input" | sed -En 's#.*(youtu\.be/|v=|youtube\.com/live/|youtube\.com/shorts/)([A-Za-z0-9_-]{11}).*#\2#p' | head -1
}

VIDEO_ID=$(extract_video_id "$INPUT_URL")
if [ -z "$VIDEO_ID" ]; then
  echo "Could not extract a YouTube video ID from '$INPUT_URL'." >&2
  exit 1
fi
```

Then try `data/videos.json` first (fast path, full metadata):

```bash
STREAM_JSON=$(jq -r --arg id "$VIDEO_ID" '[.videos[] | select(.id == $id)] | .[0] | "\(.title)\t\(.published_at)"' ~/dev/claude-social-media-skills/youtube-analytics/data/videos.json)
```

If videos.json doesn't have it yet (typical during a just-started live stream — videos.json is only updated on `go run . fetch`), fall back to the public YouTube oEmbed endpoint. No auth required:

```bash
if [ -z "$STREAM_JSON" ] || [ "$STREAM_JSON" = "null" ] || [ "$STREAM_JSON" = "	null" ]; then
  OEMBED=$(curl -sL --max-time 5 "https://www.youtube.com/oembed?url=https://www.youtube.com/watch?v=${VIDEO_ID}&format=json")
  TITLE=$(printf '%s' "$OEMBED" | jq -r '.title // empty')
  if [ -z "$TITLE" ]; then
    echo "Video $VIDEO_ID not in videos.json and oEmbed lookup failed. Pass --campaign <slug> to override." >&2
    exit 1
  fi
  PUBLISHED_AT=$(date -u +%Y-%m-%dT%H:%M:%SZ)   # best we can do without the API key
fi
```

**B. No positional arg — use the latest live from videos.json:**

```bash
STREAM_JSON=$(jq -r '[.videos[] | select(.video_type == "live")] | sort_by(.published_at) | reverse | .[0] | "\(.id)\t\(.title)\t\(.published_at)"' ~/dev/claude-social-media-skills/youtube-analytics/data/videos.json)
IFS=$'\t' read -r VIDEO_ID TITLE PUBLISHED_AT <<<"$STREAM_JSON"
```

**Staleness check (only matters for path B):** if `data/videos.json` mtime is older than 24h, warn the user before returning the URL:

```bash
MTIME=$(stat -f %m ~/dev/claude-social-media-skills/youtube-analytics/data/videos.json)
NOW=$(date +%s)
AGE_HOURS=$(( (NOW - MTIME) / 3600 ))
if [ "$AGE_HOURS" -gt 24 ]; then
  echo "videos.json is ${AGE_HOURS}h old — the 'latest live' may not be today's stream."
  echo "Run: (cd ~/dev/claude-social-media-skills/youtube-analytics && go run . fetch) to refresh."
fi
```

**C. Explicit `--campaign <slug>`:** skip stream resolution entirely and use the slug as-is.

If after all three paths the title is empty and no campaign was supplied, stop with an instructive error.

### Phase 2 — Slugify the title into a campaign name

Derive `utm_campaign` from the stream title. Rules:

1. Lowercase everything.
2. Replace any non-alphanumeric character with `-`.
3. Collapse consecutive `-` to single `-`.
4. Trim leading/trailing `-`.
5. Cap at 60 characters (UTM values stay short for clean click logs).

```bash
slugify() {
  printf '%s' "$1" \
    | tr '[:upper:]' '[:lower:]' \
    | tr -c 'a-z0-9' '-' \
    | sed -E 's/-+/-/g; s/^-//; s/-$//' \
    | cut -c 1-60 \
    | sed 's/-$//'
}
CAMPAIGN=$(slugify "$TITLE")
```

If `--campaign` is supplied on the command line, skip this step and use it verbatim (after the same slugify pass, to normalize).

### Phase 3 — Resolve the base URL for the chosen lead magnet

Default target is `discord`. If the user's arg is `newsletter`, use that instead:

```bash
case "$TARGET" in
  discord|"")  BASE="https://www.enterprisevibecode.com/discord-server-invite" ;;
  newsletter)  BASE="https://www.enterprisevibecode.com/subscribe" ;;
  *)           echo "Unknown target '$TARGET'. Edit SKILL.md to add it." >&2; exit 1 ;;
esac
```

### Phase 4 — Build the final URL

UTM values:
- `utm_source` — where the click originates (`youtube` / `beehiiv` / `linkedin`, set in Phase 0)
- `utm_medium` — the channel format (`livestream` / `newsletter`, set in Phase 0)
- `utm_campaign=<slug>` — per-issue identifier from Phase 2 (YouTube live title) or Phase 0 (Beehiiv post title)
- `utm_content=<target>` — so you can tell Discord vs newsletter-signup clicks apart

```bash
URL="${BASE}?utm_source=${UTM_SOURCE}&utm_medium=${UTM_MEDIUM}&utm_campaign=${CAMPAIGN}&utm_content=${TARGET:-discord}"
```

### Phase 5 — Output + copy to clipboard

Print the result block AND copy the URL to the clipboard for immediate paste:

```bash
printf '%s' "$URL" | pbcopy
cat <<EOF
Stream:   ${TITLE}
ID:       ${VIDEO_ID}
Date:     ${PUBLISHED_AT%%T*}
Target:   ${TARGET:-discord}
Campaign: ${CAMPAIGN}

${URL}

(copied to clipboard — paste into the stream chat or description)
EOF
```

## One-shot reference

Whole thing in one bash block. Accepts `[target] [youtube-url-or-id]` in either order, or a bare URL/ID as the sole argument:

```bash
# Parse args: anything matching a youtube-like token is the stream; otherwise it's the target.
TARGET=""; INPUT_URL=""
for arg in "$@"; do
  case "$arg" in
    discord|newsletter) TARGET="$arg" ;;
    *youtube.com*|*youtu.be*|*)
      if printf '%s' "$arg" | grep -Eq '^[A-Za-z0-9_-]{11}$' || printf '%s' "$arg" | grep -qE 'youtu'; then
        INPUT_URL="$arg"
      elif [ -z "$TARGET" ]; then
        TARGET="$arg"
      fi
      ;;
  esac
done
TARGET=${TARGET:-discord}

# Resolve stream context.
if [ -n "$INPUT_URL" ]; then
  VIDEO_ID=$(printf '%s' "$INPUT_URL" | grep -Eo '^[A-Za-z0-9_-]{11}$' || \
             printf '%s' "$INPUT_URL" | sed -En 's#.*(youtu\.be/|v=|youtube\.com/live/|youtube\.com/shorts/)([A-Za-z0-9_-]{11}).*#\2#p' | head -1)
  STREAM=$(jq -r --arg id "$VIDEO_ID" '[.videos[] | select(.id == $id)] | .[0] | "\(.title)\t\(.published_at)"' ~/dev/claude-social-media-skills/youtube-analytics/data/videos.json 2>/dev/null)
  if [ -z "$STREAM" ] || [ "$STREAM" = "null" ] || printf '%s' "$STREAM" | grep -q '^\tnull$\|^null\t'; then
    TITLE=$(curl -sL --max-time 5 "https://www.youtube.com/oembed?url=https://www.youtube.com/watch?v=${VIDEO_ID}&format=json" | jq -r '.title // empty')
    PUBLISHED_AT=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  else
    IFS=$'\t' read -r TITLE PUBLISHED_AT <<<"$STREAM"
  fi
else
  STREAM=$(jq -r '[.videos[] | select(.video_type == "live")] | sort_by(.published_at) | reverse | .[0] | "\(.id)\t\(.title)\t\(.published_at)"' ~/dev/claude-social-media-skills/youtube-analytics/data/videos.json)
  IFS=$'\t' read -r VIDEO_ID TITLE PUBLISHED_AT <<<"$STREAM"
fi

CAMPAIGN=$(printf '%s' "$TITLE" | tr '[:upper:]' '[:lower:]' | tr -c 'a-z0-9' '-' | sed -E 's/-+/-/g; s/^-//; s/-$//' | cut -c 1-60 | sed 's/-$//')
case "$TARGET" in
  discord)     BASE="https://www.enterprisevibecode.com/discord-server-invite" ;;
  newsletter)  BASE="https://www.enterprisevibecode.com/subscribe" ;;
esac
URL="${BASE}?utm_source=youtube&utm_medium=livestream&utm_campaign=${CAMPAIGN}&utm_content=${TARGET}"
printf '%s' "$URL" | pbcopy
echo "$URL"
```

## Growth-plan hook

Priority 2 of the growth plan says to push every viewer to Beehiiv. The `beehiiv_attribution` MCP tool has a `top_campaigns` field that's currently empty — this skill is what populates it. After a few streams using these tagged URLs, `beehiiv_attribution` will rank which episodes actually convert.

## Known issues

- **Discord redirects don't forward UTM params by default.** The `enterprisevibecode.com/discord-server-invite` page is probably a redirect to `discord.gg/...`. UTMs on the incoming URL won't reach Discord, but your redirect page's analytics (Cloudflare, Plausible, etc.) WILL see them — that's where the attribution lives. For beehiiv subs, the `newsletter` target is the direct path because it lands on a beehiiv subscribe page that reads UTMs natively.
- **videos.json staleness.** The latest live stream may not be in videos.json until after `go run . fetch` runs. Consider a launchd agent or cron to fetch every 15 min during stream days.
- **Slugification collisions.** If two streams share a truncated title, they'll get the same campaign slug. Rare, but `--campaign` override handles it.
