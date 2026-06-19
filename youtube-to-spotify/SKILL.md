---
name: youtube-to-spotify
description: Use when user wants to republish a YouTube video essay as a podcast episode on Spotify for Creators (Anchor) so it syndicates to Apple Podcasts and other podcast platforms via RSS — "turn this YouTube video into a podcast", "publish my video essay as a podcast episode", "post to Spotify for Creators", "republish on Anchor", "syndicate my video to podcast platforms". Downloads the video, transcodes Spotify-compliant audio + video, writes voice-grounded show notes with the newsletter CTA, drives the Spotify upload UI via gstack browse, and records the episode in the post-manifest.
user_invocable: true
---

# youtube-to-spotify

Republish a YouTube video essay as a **Spotify for Creators** podcast episode. You upload **one MP4** — Spotify shows it as native in-app video AND automatically extracts the audio, which its RSS feed syndicates out to Apple Podcasts, Overcast, and every other podcast app (those apps get audio-only). So no separate audio file is needed. The skill downloads the source, transcodes one Spotify-spec MP4, generates voice-grounded show notes with the newsletter CTA, drives the upload UI with `gstack browse` (handing off only for the OS file picker and the final Publish click), and logs the episode in the post-manifest for closed-loop attribution.

## Why this is a browser-driven skill (not an API call)

**Spotify for Creators has no public episode-upload API.** The Distribution API that launched Jan 2026 only works *through* third-party hosting partners (Acast, Audioboom, Libsyn, Omny, Podigee) — not direct, and not for shows hosted on Spotify itself. New episodes must go through the `creators.spotify.com` web UI: **New Episode → upload file → Details (title/description) → Publish/Schedule**. So this skill drives that UI the same way `/crosspost-newsletter` drives LinkedIn/Substack — proven `gstack browse` patterns, with handoffs where the browser hands control to a native OS dialog.

**Why one MP4 (not MP4 + MP3).** Per [Spotify's docs](https://support.spotify.com/us/creators/article/publishing-videos/), uploading a *video* episode makes Spotify auto-extract the audio and deliver it over your RSS feed to all other apps — the video is Spotify-exclusive, everyone else gets audio-only, all from the single upload. A separate MP3 is only a **fallback** (see `Edge: video-upload-rejected`).

## Usage

```
/youtube-to-spotify <youtube-url>            # full run
/youtube-to-spotify <youtube-url> --dry-run  # stop after producing the MP4; no upload
```

Accepts a full watch URL, a `youtu.be`/`shorts`/`embed` URL, or a bare video ID.

## 🟢 Happy Path

A single video essay → one MP4 → Spotify episode (in-app video + auto-extracted audio) → syndicated to all podcast platforms. ~8–15 min wall-clock, most of it download + transcode.

**Phase 1 — Resolve + ownership + idempotency (~1 min).** Resolve the URL to a video ID, confirm it's the user's own channel, and check the manifest so you never double-post.

**Phase 2 — Download + transcode (~3–8 min).** `youtube-to-spotify.sh fetch <url>` produces one `episode-spotify.mp4` (H.264/AAC ≥192k) under `/tmp/yt2sp/<id>/`. `--dry-run` stops here.

**Phase 3 — Episode metadata (~2 min).** Title from the YouTube title; 2–4 paragraph show notes summarized from the transcript, grounded in the user's voice corpus, ending with the newsletter CTA link and the `[sp:<slug>]` tag. Gate on adversarial review.

**Phase 4 — Upload via gstack browse.** Auth-check Spotify, open New Episode, hand off for the native file picker to select the MP4, fill title + description (React setter), hand off for the final Publish/Schedule click, capture the published episode URL.

**Phase 5 — Record in the post-manifest.** Write the episode + captured Spotify URL to `youtube-analytics/data/spotify_episodes/<id>.json` with the `[sp:]` scheme so `/flywheel` and future fetchers can attribute it.

## Prerequisites

- **yt-dlp** — `pipx install yt-dlp` (isolated venv; PEP 668 blocks system pip on macOS). Fallback `brew install yt-dlp`.
- **ffmpeg** — `brew install ffmpeg` (yt-dlp muxing + the H.264/AAC transcode; also the optional `--with-audio` MP3 fallback).
- **youtube-transcript-api** — `pipx install youtube-transcript-api` (already a repo prereq via `youtube-analytics/scripts/generate-transcript.sh`; reused for show notes).
- **jq** — for the manifest helpers and field-set JSON encoding.
- **gstack browse** — verify the binary: `B="${B:-$HOME/.claude/skills/gstack/browse/dist/browse}"; [ -x "$B" ] && echo READY || echo NEEDS_SETUP`. Run **headed** so the file-picker and publish handoffs are visible: `$B connect` then `$B status` (expect `Mode: headed`). Don't use launched mode — handoff opens `about:blank` and can drop cookies (`/crosspost-newsletter` `Edge: launched-mode-invisible`).
- **Spotify for Creators login** — a logged-in Chrome session at `creators.spotify.com`, imported via `_shared/gstack_auth.sh`.
- **config** — edit `youtube-to-spotify/config.json`: set `channel_id` (your YouTube channel, for the ownership check) and confirm `newsletter_url`.

This skill fires many browser tool calls; run it in a dedicated `claude --dangerously-skip-permissions` instance, but keep the human Publish click (Phase 4f) — that's the publish gate.

## Process

Repo root in examples: `~/dev/claude-social-media-skills`. The helper script: `youtube-to-spotify/youtube-to-spotify.sh` (subcommands `resolve | fetch | probe | setfield`).

### Phase 1 — Resolve + ownership + idempotency

```bash
cd ~/dev/claude-social-media-skills
ID=$(youtube-to-spotify/youtube-to-spotify.sh resolve "<url>")
SLUG=$ID   # episode slug = video id (stable, collision-free)
MANIFEST=youtube-analytics/data/spotify_episodes/$ID.json
```

**Idempotency gate (before any download).** If the manifest already records a published Spotify URL for this video, STOP and report it — don't re-download or double-post (`Edge: already-published`):

```bash
source _shared/post-manifest/post_manifest.sh
if [ -f "$MANIFEST" ] && pm_find_clip "$MANIFEST" --clip-id "$SLUG" \
     | jq -e '.scheduled_posts[]?.api_response.episode_url // empty' >/dev/null 2>&1; then
  echo "Already published:"; pm_find_clip "$MANIFEST" --clip-id "$SLUG" | jq -r '.scheduled_posts[].api_response.episode_url'
  exit 0
fi
```

**Ownership gate.** This is the user's own content by contract, but verify — don't silently republish a third party's video. Check the uploader against `config.json:channel_id`, and/or cross-reference the voice corpus:

```bash
yt-dlp --print "%(channel_id)s|%(channel)s|%(title)s" "<url>"   # compare channel_id to config
# secondary signal: the user's own livestreams/essays show up here as source_type:"youtube_live"
_shared/voice-corpus/voice-corpus | jq -r '.posts[] | select(.source_type=="youtube_live") | .title'
```

If neither confirms ownership, use **AskUserQuestion** once to confirm before proceeding.

### Phase 2 — Download + transcode

```bash
youtube-to-spotify/youtube-to-spotify.sh fetch "<url>"
# → VIDEO_ID=…  WORKDIR=/tmp/yt2sp/<id>
#   MP4=/tmp/yt2sp/<id>/episode-spotify.mp4 (h264 aac)
```

The script downloads the best ≤1080p stream and, **skipping the transcode if the download is already H.264/AAC** (saves minutes on long essays — `Edge: long-video`), otherwise transcodes to H.264 high / AAC 192k with `+faststart`. **It does not extract a separate MP3** — Spotify derives the syndicated audio from the MP4's AAC track. Verify codecs anytime with `youtube-to-spotify.sh probe <file>`. (The `--with-audio` flag additionally writes `episode.mp3` — only needed for the `Edge: video-upload-rejected` fallback.)

**`--dry-run` stops here.** Confirm `episode-spotify.mp4` exists and `probe` shows `h264 aac`, then report the path and stop.

### Phase 3 — Episode metadata

- **Title** = the YouTube title (podcast episode titles have generous limits — no truncation needed, unlike the ≤100-char YouTube-title cap in `/opus-clips`).
- **Show notes** — fetch the transcript and summarize into **2–4 short paragraphs in the author's voice**. Do **not** paste the raw transcript.

  ```bash
  youtube-analytics/scripts/generate-transcript.sh "$ID" > /tmp/yt2sp/$ID/transcript.txt
  #   exit 4 = api missing, exit 5 = captions disabled/private → Edge: captions-disabled
  _shared/voice-corpus/voice-corpus | jq '[.posts[] | select(.source_type=="newsletter")]'  # voice grounding
  ```

- **Newsletter CTA (REQUIRED).** Podcast show notes support clickable links, so use a **direct UTM-tagged beehiiv link** as the CTA (mirrors `/opus-clips` "direct link on link-platforms"):

  ```
  📬 Full essay + free weekly newsletter: https://www.enterprisevibecode.com/?utm_source=spotify&utm_medium=podcast&utm_campaign=yt2sp_<ID>
  ```

  End the description with the closed-loop tag on its own last line: `[sp:<ID>]`.
- **Adversarial review (gate).** Run the generated show notes through the repo's reviewer before publishing — every generative skill does this:

  ```bash
  _shared/adversarial-review/adversarial-review   # must return all_pass; ground on transcript.txt
  ```

### Phase 4 — Drive creators.spotify.com via gstack browse

```bash
B="${B:-$HOME/.claude/skills/gstack/browse/dist/browse}"
```

**4a — Auth.** `_shared/gstack_auth.sh` imports cookies + self-heals a wedged session:

```bash
_shared/gstack_auth.sh spotify.com https://creators.spotify.com/ ; echo "exit=$?"
```
- `0` → logged in, proceed.
- `1` → not logged in. **Interactive:** `$B handoff "Switch to the Spotify for Creators tab, log in, then tell me when you're ready."` → `$B resume` → re-run the auth check. **Autonomous:** stop with `auth_failed`.
- `2` → bug in the call — stop.

**4b — Open New Episode.** Navigate to the dashboard and start an upload; capture refs with a snapshot (the exact "New episode" control is discovered at runtime — Spotify A/B-tests this UI):

```bash
$B goto https://creators.spotify.com/
$B snapshot -i        # find the "New episode" button ref
$B click @e<NewEpisode>
$B snapshot -i        # find the upload control + (later) the title/description fields
```

**4c — Upload the MP4 → native picker (HANDOFF).** This single video upload IS the episode — Spotify auto-extracts the audio for RSS. The upload triggers the browser's native OS file dialog, which JS cannot populate. First *try* the in-page input (works only if it's a settable `<input type=file>`, the way `/crosspost-newsletter` uploads images):

```bash
$B upload "<file-input-selector>" /tmp/yt2sp/<id>/episode-spotify.mp4
```

If Spotify routes to a true OS dialog, hand off (`Edge: native-file-picker`):

```bash
$B handoff "Switch to the Spotify tab, click Upload, and choose /tmp/yt2sp/<id>/episode-spotify.mp4 in the file picker. Tell me when the upload bar completes. (The about:blank tab can be ignored.)"
$B resume
```

**4d — Title + description (React setter).** Spotify's editor is React-controlled, so a plain `.value =` reverts (`Edge: react-field-revert`). Discover each field's selector from the snapshot, then set it via the helper (which applies the native-setter + dispatch pattern and verifies):

```bash
youtube-to-spotify/youtube-to-spotify.sh setfield "<title-input-selector>" "<episode title>"
youtube-to-spotify/youtube-to-spotify.sh setfield "<description-textarea-selector>" "$(cat /tmp/yt2sp/<id>/shownotes.txt)"
# each prints SET:<first 80 chars> — confirm it stuck.
```

If the description box is `contenteditable` (not a `<textarea>`), fall back to the clipboard-paste pattern used for LinkedIn/Substack bodies in `/crosspost-newsletter` (dispatch a `ClipboardEvent('paste', {clipboardData})`).

**4e — Publish (HANDOFF — keep the human in the loop).** Screenshot the filled draft and let the user do the final click:

```bash
$B screenshot /tmp/yt2sp/<id>/draft.png
$B handoff "Review the episode (title, description, video uploaded) and click Publish — or set a schedule and click Schedule. Tell me when it's live."
$B resume
EPISODE_URL=$($B url | head -1)   # capture the published-episode permalink
```

Verify `EPISODE_URL` changed to a published-episode URL (canonical-signal verification, same idiom as `/crosspost-newsletter`); if it still shows the editor, the publish didn't land — re-check with the user.

### Phase 5 — Record in the post-manifest

```bash
source _shared/post-manifest/post_manifest.sh
pm_init "$MANIFEST" --source-video "$ID" --source-title "<yt title>" --source-url "<url>"
pm_ensure_clip "$MANIFEST" --clip-id "$SLUG" --title "<episode title>" \
  --description "<show notes … ending with [sp:$SLUG]>"
pm_append_post "$MANIFEST" --clip-id "$SLUG" --label "SPOTIFY_PODCAST" \
  --scheduled-at-utc "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --api-response "$(jq -nc --arg u "$EPISODE_URL" '{episode_url:$u}')"
```

The `[sp:<id>]` in-text tag is registered in `_shared/post-manifest/README.md`. Spotify bypasses Buffer, so there is **no `format:` Buffer tag** here — attribution rides on the manifest + `[sp:]` tag, exactly like `/opus-clips`' `[opus:]`.

## Closed-loop attribution

Spotify publishing never touches Buffer, so `buffer-stats`' `format:<name>` system can't see it. Attribution rides the shared **post-manifest** primitive (`_shared/post-manifest/`):

- **In-text tag:** every episode's show notes end with `[sp:<video_id>]` on its own line — grep-able across Spotify search even if the manifest is lost.
- **Manifest:** `youtube-analytics/data/spotify_episodes/<video_id>.json` records the source video, the composed title/description, and the captured Spotify episode URL.
- **Downstream:** `/flywheel` reads non-Buffer manifests to count podcast republishing toward throughput; a future Spotify-native fetcher can walk `scheduled_posts[].api_response.episode_url` for per-episode plays.

## Verification (dry-run ladder — no publish needed)

1. **Resolve:** `youtube-to-spotify.sh resolve` on a full URL, a `youtu.be` URL, and a bare ID all print the same video ID.
2. **Download:** `/youtube-to-spotify <url> --dry-run` → `episode-spotify.mp4` exists; `youtube-to-spotify.sh probe` shows `h264 aac`, 16:9. (`fetch --with-audio` additionally yields `episode.mp3`.)
3. **Metadata:** transcript fetch returns text (or the documented exit 4/5); show notes are 2–4 paragraphs; adversarial review returns `all_pass`; the CTA link + `[sp:<id>]` tag are present.
4. **Auth:** `_shared/gstack_auth.sh spotify.com https://creators.spotify.com/; echo $?` → `0` when logged in.
5. **Manifest / idempotency:** after a publish, `pm_count_scheduled "$MANIFEST"` ≥ 1 and `pm_find_clip --clip-id <id>` shows the episode URL in `api_response`. Re-run the skill → the idempotency gate reports "already published" and does not re-download or re-post.

## Edge cases (read only when the matching signal appears)

| Label | Symptom |
|---|---|
| `Edge: private-or-members-only` | yt-dlp errors out; nothing downloads |
| `Edge: captions-disabled` | `generate-transcript.sh` exits 4 or 5 |
| `Edge: gstack-not-logged-in` | `gstack_auth.sh` exits 1 |
| `Edge: native-file-picker` | `$B upload` can't set the file; an OS dialog appears |
| `Edge: video-upload-rejected` | Spotify rejects the MP4 (size/length/codec) |
| `Edge: long-video` | multi-hour essay; slow transcode / large files |
| `Edge: react-field-revert` | title/description reverts after you fill it |
| `Edge: already-published` | manifest already has a Spotify URL for this video |

### `Edge: private-or-members-only`
yt-dlp fails on private/members-only/age-gated videos. The `fetch` subcommand reports this and exits non-zero. Don't upload a partial file. Ask the user to grant a logged-in session — `yt-dlp --cookies-from-browser chrome "<url>"` — or to export the file manually, then re-run `fetch` (it reuses an existing `/tmp/yt2sp/<id>/episode.mp4`).

### `Edge: captions-disabled`
`generate-transcript.sh` exits 5 (captions disabled/private/region-locked) or 4 (api not installed). **Never block publishing on the transcript.** Fall back to writing show notes from the YouTube title + description + the voice corpus alone. Note it for the user so they can paste a transcript if they want richer notes.

### `Edge: gstack-not-logged-in`
`gstack_auth.sh` exit 1. Interactive → `$B handoff` for in-window login, then `$B resume` and re-check. Autonomous → stop with `auth_failed`; don't attempt a programmatic login. (PATTERNS.md → "When to handoff vs proceed".)

### `Edge: native-file-picker`
`$B upload "<selector>" <file>` only works when the target is an in-page `<input type=file>`. If Spotify opens a true OS file dialog, that dialog is outside the page DOM and neither JS nor `$B upload` can drive it — hand off and have the user pick the exact path you print (`/tmp/yt2sp/<id>/episode-spotify.mp4`). Always *try* `$B upload` first (some Spotify upload widgets do expose a settable input); fall back to handoff only when it fails.

### `Edge: video-upload-rejected`
Spotify rejects the MP4 — too large, too long (max 12 h), or an unsupported codec. The MP4 is the primary path because one upload covers both Spotify video and RSS audio, but if it won't take, **don't lose the syndication**: produce the audio fallback and publish an audio-only episode (the video can be added later from the episode's three-dot menu → Upload video):

```bash
youtube-to-spotify/youtube-to-spotify.sh fetch "<url>" --with-audio   # writes episode.mp3
# then upload /tmp/yt2sp/<id>/episode.mp3 at Phase 4c instead of the MP4
```

### `Edge: long-video`
Multi-hour essays produce large files and slow transcodes. Warn on duration (`yt-dlp --print "%(duration)s"`); the `fetch` subcommand already **skips the transcode when the download is already H.264/AAC**, which is the big win. Spotify's video max is 12 h; if Spotify rejects the upload on size/length, split into parts rather than re-encoding blindly.

### `Edge: react-field-revert`
If the title or description reverts to empty/old after you set it, you set `.value` directly instead of via the native setter — that's exactly what `setfield` avoids. Re-run `youtube-to-spotify.sh setfield`, which dispatches `input` + `change` + `blur` after calling the prototype's native value setter (PATTERNS.md → React form input setter).

### `Edge: already-published`
The Phase 1 idempotency gate found a manifest entry with a captured `episode_url` → STOP and report the existing URL; don't double-post. A re-run after a *failed* publish (no URL recorded) resumes from where it stopped and reuses the `/tmp/yt2sp/<id>/` artifacts rather than re-downloading.

## Files in this directory

- `SKILL.md` — this workflow.
- `youtube-to-spotify.sh` — pure-transport helper (`resolve`, `fetch`, `probe`, `setfield`).
- `config.json` — channel ID (ownership check), newsletter URL, UTM defaults.

## Related skills

- `/opus-clips` — the other non-Buffer publishing path; shares the post-manifest + in-text-tag attribution pattern (`[opus:]` ↔ `[sp:]`).
- `/crosspost-newsletter` — the canonical `gstack browse` UI-driver precedent (auth, snapshot/upload/handoff, React fills, canonical-signal publish verify).
- `/flywheel` — counts this episode toward throughput and (future) reads the Spotify manifest for per-episode plays.

## Out of scope

- Uploading the original video to YouTube (use the user's existing publishing flow — this skill starts from a live YouTube URL).
- Editing the audio/video (trim, intro/outro, loudness normalization) — bring a finished essay.
- Cover art / show-level settings (set once in Spotify for Creators).
- Switching podcast hosts or using Spotify's Distribution API (only relevant if the show moves off Spotify-hosting to Acast/Libsyn/etc.).
