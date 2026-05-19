---
name: opus-clips
description: Use when user wants to turn a long-form video (YouTube URL or local file) into short-form social clips via OpusClip — "clip this video", "opus clip my stream", "make shorts from this", "clip my latest YouTube longform", "clip the stream". Submits to the OpusClip API, polls until clips are ready, optionally strips the default brand overlay, and hands off to `/promote-newsletter` or `opusclip post` for scheduling.
user_invocable: true
---

# opus-clips

End-to-end OpusClip wrapper over the **official `opusclip` CLI** (installed via the `opusclip` plugin at `~/.claude/plugins/cache/opus-skills/opusclip/<version>/scripts/opusclip`). This skill layers user-specific defaults + workflow on top of that CLI — it doesn't reimplement the API surface.

## What this skill owns vs the plugin

| Concern | Where it lives |
|---|---|
| Auth (`OPUSCLIP_API_KEY`), CLI commands, API endpoints, EditingScript schema | **`opusclip:opusclip` plugin skill** — call it / read its docs for CLI specifics |
| 30-90s shorts only, 5/day/channel cap, 6 connected accounts, slot times, viral-score filter | **This skill** |
| Brand-overlay-strip workflow (undocumented API surface), polling cadence, batch-loop rate-limiting | **This skill** (learned 2026-05-18 session) |

## Non-negotiables

Two hard rules from the user — verified across multiple sessions, don't override without asking:

1. **30-90s shorts only.** `opusclip submit ... --durations "30,60,90"`. Never let OpusClip default to "Auto" which can produce >90s clips.
2. **5 posts/day/channel hard cap, not a lower bound.** Diminishing returns above this. Never schedule a 6th slot on the same calendar day — roll over to next day. See `daily_time_slots` in `config.json`.

## Prerequisites

- `OPUSCLIP_API_KEY` set in shell env (stored in `~/.zshrc` as of 2026-05-18). API access requires Enterprise or Pro plan; key at https://clip.opus.pro/dashboard.
- `opusclip` CLI on disk via the `opusclip` plugin. Verify with `~/.claude/plugins/cache/opus-skills/opusclip/*/scripts/opusclip templates`.
- `jq`, `curl`, `ffmpeg` (last one only needed for local trim / frame extraction).

If the key isn't set, defer to the `opusclip:opusclip` plugin skill — its setup is canonical.

## 🟢 Happy Path

For a long-form YouTube video → ~20 clips at 30/60/90s → review → optionally strip brand overlay → ready for fan-out. ~10-15 min wall-clock (most of it is OpusClip's processing).

**Phase 1 — Find source.** YouTube URL the user provides, OR "latest long-form" via `~/dev/youtube_analytics`:

```bash
jq -r 'if type=="array" then . else (.videos // .items) end
  | map(select(.video_type == "long-form"))
  | sort_by(.published_at) | reverse | .[0]
  | "https://www.youtube.com/watch?v=\(.id)  \(.title)  \(.duration_seconds)s"
' ~/dev/youtube_analytics/data/videos.json
```

**Phase 2 — Submit.**

```bash
opusclip submit \
  --url "<YOUTUBE_URL>" \
  --durations "30,60,90" \
  --model ClipBasic \
  --title "<short title>"
```

`ClipBasic` for talking-head essays (default for EVC long-form). `ClipAnything --prompt "..."` only when you want directed curation around a specific theme — see `Edge: curation-misses-theme`.

Capture the returned `projectId` (e.g. `P3051823ab0w`) — every downstream call needs it.

**Phase 3 — Poll until ready (~5-15 min for a 10-20 min source).** Background bash with ~270s tick (just under 5 min — stays in Claude's prompt cache window so each poll is cheap on re-entry):

```bash
PROJECT=<id>
for i in $(seq 1 12); do
  sleep 270
  COUNT=$(opusclip list --project $PROJECT --summary | jq 'length')
  [ "${COUNT:-0}" -gt 0 ] && { echo "READY ($COUNT clips)"; exit 0; }
  echo "[tick $i] still processing"
done
exit 1
```

Run this with `Bash run_in_background=true`. You get notified when it exits.

**Phase 4 — Filter top clips.** Filter to `viral_score >= config.viral_score_threshold` (currently 70), then present sorted descending. Note: OpusClip generates **bonus clips** with `_bonus` suffix on the clip_id (alternate versions of the top picks) — they appear in `list` like any other clip; treat them as normal candidates.

```bash
opusclip list --project $PROJECT --summary | jq -r '
  map(select(.score >= 70))
  | sort_by(-.score) | .[]
  | "[\(.score)] \(.duration_sec)s — \(.title)  (\(.clip_id))"
'
```

**Phase 5 — (Optional) strip brand overlay.** See `Edge: brand-overlay-baked-in` for full details — overlays are applied at render-time from account-level defaults, not from the template, so the only way to remove them from existing clips is the undocumented `renderPreferenceOverride` body field on the re-render endpoint. Confirmed working 2026-05-18, **does not consume credits**.

**Phase 6 — Preview / hand off.**

```bash
opusclip preview --project $PROJECT   # opens HTML in browser, sorted by score
```

For scheduling, two paths:
- **Via OpusClip's native posting** (`opusclip post schedule`) — fan-out to OpusClip's connected accounts. Beta pricing, may diverge from web UX. Subject to OpusClip's per-day cap interpretation.
- **Via Buffer** (this user's preferred path) — download the mp4s from each clip's `uriForExport`, schedule via `/promote-newsletter` or `/carousel-newsletter` flows. Uses the user's existing slot math (5/day across 6 channels at 09:00 / 12:00 / 15:00 / 18:00 / 21:00 PT).

## Edge cases (read only when the matching signal appears)

### `Edge: brand-overlay-baked-in`

**Signal:** clips render with the user's default brand overlay (e.g. `sandboxbjj.png`) even though `opusclip templates` shows the active template has `screenOverlays: []`.

**Cause:** the overlay is merged in from an account-level default at render time, not from the template. Confirmed 2026-05-18 by inspecting the submit response renderPref vs the template config.

**Fix:** undocumented `renderPreferenceOverride` body field on the re-render endpoint. The plugin CLI does **not** expose this — call the API directly with curl. Verified payload shape:

```bash
OVERRIDE='{
  "screenOverlays": [],
  "screenOverlay": null,
  "screenOverlayFileId": null,
  "enableScreenOverlay": false,
  "disableScreenOverlay": true
}'

for C in $(opusclip list --project $PROJECT --summary | jq -r '.[].clip_id'); do
  SCRIPT=$(curl -sS "https://api.opus.pro/api/exportable-clips/$PROJECT.$C?include=editingScript" \
    -H "Authorization: Bearer $OPUSCLIP_API_KEY" | jq -c '.editingScript // empty')
  [ -z "$SCRIPT" ] && { echo "skip $C"; continue; }
  BODY=$(jq -nc --argjson s "$SCRIPT" --argjson r "$OVERRIDE" '{editingScript: $s, renderPreferenceOverride: $r}')
  curl -sS -X POST "https://api.opus.pro/api/exportable-clips/$PROJECT.$C/re-render" \
    -H "Authorization: Bearer $OPUSCLIP_API_KEY" \
    -H "Content-Type: application/json" -d "$BODY" | jq -r '.jobId // .error'
  sleep 3  # see Edge: rate-limit-30-per-min
done
```

Each clip re-renders in ~30-45s. Confirmed 2026-05-18: **no credit charge** for these re-renders (dashboard balance unchanged after test of 1 clip with override). The user's dashboard view is authoritative; do NOT trust `md5` comparison of downloaded mp4s — see `Edge: cdn-serves-stale-bytes`.

**Verification:** the user opens the re-rendered clip in the OpusClip dashboard or `opusclip preview --project $PROJECT` and confirms the overlay is gone. Do not assume the override worked from the API response alone (it returns `{jobId: ...}` whether the override is honored or silently ignored).

**Test before batching.** When you don't know if a particular override payload works for a new field, fire on ONE clip first. Wait for `renderAsVideoFile.pending == false`. Have the user verify in the dashboard. Then loop the remaining clips. Catching a silently-ignored override on clip 1 saves 22 wasted calls.

### `Edge: cdn-serves-stale-bytes`

**Signal:** after a successful re-render (new `concludedAt`, new `v=` URL signature), downloading the mp4 returns byte-identical content to the previous render. `md5` hashes match.

**Cause:** OpusClip's CDN keys cached content by **path** (`/media/.../c.<clipId>/VIDEO_FILE_-0-<duration>.mp4`), not by signed-URL query params. A new render at the same path may take time to propagate, OR the CDN may continue serving the old object until the cache expires.

**Implication:** **do not use md5/file-bytes diff to verify a re-render took effect.** Use the dashboard, `opusclip preview`, or wait several minutes and retry the download. The API's `renderAsVideoFile.concludedAt` + a new `v=` query param is the authoritative signal that a new render happened on the server.

### `Edge: rate-limit-30-per-min`

**Signal:** batch loop returns 429s after ~10-15 calls.

**Fix:** OpusClip is documented at 30 req/min. Each clip in a batch typically takes 2 calls (GET script + POST re-render). Pace with `sleep 3` between iterations to stay under the ceiling.

### `Edge: bash-for-loop-ifs-quirk`

**Signal:** batch loop only runs ONE iteration; the entire newline-separated input ends up as a single `$VAR` value. URL becomes malformed.

**Cause:** `for C in $CLIPS` depends on `$IFS` doing word-splitting on newlines, but `$IFS` can be modified by prior code in the same script.

**Fix:** always use `while IFS= read -r C; do ... done <<< "$CLIPS"` — robust against any `$IFS` state. Hit this 2026-05-18 on the first batch re-render attempt; the loop only ran once with all 22 clip IDs concatenated into one URL.

### `Edge: curation-misses-theme`

**Signal:** `ClipBasic` returns clips whose titles don't match the source video's actual narrative — the AI picked tangential moments instead of the spine.

**Example 2026-05-18:** source "How to Scale Without the Slop" produced clips titled "Master AI Productivity," "AI Teammates," "Beads: JSON for Agents," "Ballmer Peak," etc. — adjacent topics, not the scaling-without-slop thesis.

**Fix:** re-submit with `--model ClipAnything --prompt "find moments where the speaker explains <theme>"`. Costs another ~$N credits to re-clip but produces directed curation. Don't try to fix this via `edit-clip` operations on the existing clips — the curation already chose the wrong source moments; you'd just be re-rendering off-theme content.

### `Edge: poll-cadence-cache-window`

**Signal:** polling more often than necessary burns Claude's prompt-cache window on every re-entry.

**Fix:** pick poll intervals deliberately. Anthropic's prompt cache TTL is 5 minutes. Polling cadence options:
- **< 270s ticks:** cache stays warm; cheap re-entries; use when actively waiting on something that may finish quickly.
- **300s exactly:** worst-of-both — pay the cache miss without amortizing it. **Avoid.**
- **> 1200s ticks:** one cache miss buys a long idle stretch; use when the work takes 10+ minutes.

For OpusClip clip-generation polling: **270s** is the right default. Most jobs finish in 5-15 min, so 1-3 ticks gets you there.

### `Edge: linkedin-self-conflict`

**Signal:** `opusclip post schedule` returns `hasConflict: true` on a LinkedIn schedule even though the user has nothing else queued at that time.

**Cause (confirmed 2026-05-18):** OpusClip's `postAccountId` is shared across `LINKEDIN` page (`urn:li:organization:*`) and `LINKEDIN` personal profile (`urn:li:person:*`) — they share one connector but have different `subAccountId`s. The API's conflict detector appears keyed on `postAccountId` alone, so scheduling a LinkedIn page post + LinkedIn profile post at the same slot triggers a false-positive conflict on the second one. The schedule still goes through; `scheduleId` and `postId` are returned normally.

**Fix:** ignore the flag if both schedules in the batch hit the same `postAccountId` with different `subAccountId`s. Don't waste cycles probing the API for conflict details — there's no listing endpoint exposed, and the API does not return `conflictsWith` IDs alongside the flag. User can verify visually at https://clip.opus.pro → Scheduled Posts.

### `Edge: bonus-clips`

**Signal:** `opusclip list --project P --summary` returns 23 clips for a project you only submitted with 3 duration buckets (would expect ~3-15).

**Cause:** OpusClip generates alternate versions of high-scoring clips with `_bonus` suffix on the clip_id (e.g. `La4Wghg6IX` + `La4Wghg6IX_bonus`). These are real, scored clips — not duplicates. Treat as normal candidates.

## Cost (as of 2026-05-18)

- **`opusclip submit` (new curation):** 1 credit per minute of source video. A 15-min long-form ≈ 15 credits.
- **`opusclip edit-clip *` (server-side re-render):** beta pricing per call; the skill docs warn "may incur charges that don't match the web UX." Confirmed 2026-05-18: at least *some* re-renders (the brand-overlay-strip case) consume **no credits**. Other edit operations (caption-fix, censor, trim) are documented as charged but per-op cost is not published — check dashboard balance after first call to calibrate.
- **`opusclip post publish` / `post schedule`:** beta; X (Twitter) posts cost 1 credit each, other platforms unspecified.

Before kicking off a batch (>3 ops on one clip, OR >5 ops total in one session), have the user check their dashboard balance so they can audit per-op cost themselves.

## Closed-loop attribution (per-post manifest)

OpusClip's native scheduler bypasses Buffer, so the `format:<name>` tag system can't attribute these posts. Use the shared **post-manifest** primitive at [`_shared/post-manifest/`](../_shared/post-manifest/README.md) instead:

```bash
source ~/dev/claude-social-media-skills/_shared/post-manifest/post_manifest.sh

MANIFEST=~/dev/youtube_analytics/data/opus_clips/$PROJECT.json
pm_init "$MANIFEST" --project "$PROJECT" --source-video "$SOURCE_YT_ID" --source-title "$TITLE"
pm_ensure_clip "$MANIFEST" --clip-id "$CLIP" --title "$NEW_TITLE" --description "$NEW_DESC" --score "$SCORE" --duration-sec "$DUR"
# After each `opusclip post schedule` call:
pm_append_post "$MANIFEST" --clip-id "$CLIP" --label "$CHANNEL_LABEL" --account-id "$AID" --sub-account-id "$SUB" --scheduled-at-utc "$AT_UTC" --api-response "$RAW_RESP_JSON"
```

Two non-negotiables when composing the description:
1. **End with `[opus:<clip_id>]` on its own line** — grep-able across any platform's native search; survives manifest loss.
2. **Persist the verbatim API response** in the manifest (not just the scheduleId) — preserves `hasConflict` and any future fields OpusClip adds.

See [PATTERNS.md § Closed-loop post manifest](../PATTERNS.md) for the broader rationale and how this composes with the Buffer `format:` system in `/flywheel`.

## Workflow integration

This skill is the **ingest + clip-prep** step. Downstream consumers:

- **`/promote-newsletter`** — fan-out individual clips to Buffer channels with the user's posting schedule + copy patterns.
- **`/carousel-newsletter`** — if a clip is being repurposed as a carousel slide instead of a video post.
- **`/flywheel`** — counts clip output toward Priority 1 (long-form throughput, since each clip is a derivative of a long-form essay) and toward fan-out reach metrics.

## Out of scope

- Uploading the source video to YouTube first (use the user's existing publishing flow).
- Captions / branding configuration (set once at https://clip.opus.pro/dashboard).
- X / Twitter cross-posting (not in the user's six standard channels).
- Real-time stream clipping (this is post-production only).

## Files in this directory

- `SKILL.md` — this workflow (CLI-driven, 2026-05-18 rewrite).
- `config.json` — channels, slot times, posts-per-day cap, viral-score threshold.
- Legacy browser-automation helpers (`drive_upload.py`, `process_clips.py`, `schedule_clips.py`, `verify_schedule.py`, `wait_for_clips.sh`, `upload.sh`, `selectors.json`, `IMPROVEMENTS.md`) — from the pre-CLI era when this skill drove the OpusClip web UI via Claude-in-Chrome MCP. Kept for reference but not invoked by the current flow. Most logic is now in the `opusclip` CLI directly. The one helper still worth its keep is `schedule_clips.py` (pure slot-math for the 5/day/channel cap); if you're hand-rolling a Buffer schedule from clip output, it's reusable.

## Related skills

- **`opusclip:opusclip` (plugin)** — canonical CLI reference. Always defer to this for CLI command surface; it stays up to date with the plugin version on disk.
- `/promote-newsletter`, `/carousel-newsletter` — downstream fan-out.
- `/flywheel` — measures clip-output impact on long-form throughput priority.
