---
name: opus-clips
description: Use when user wants to turn a livestream video file into short-form social clips via Opus Clip — "clip my stream", "make shorts from this stream", "run opus on this video". Uploads the file, waits for AI clip generation, toggles the Enhancer (filler word + silence removal) on selected clips, then schedules them in Opus at up to 5 per day per connected platform.
user_invocable: true
---

# opus-clips

**⚠️ SCAFFOLD — NOT YET IMPLEMENTED.** This skill's workflow is specified but the DOM selectors for app.opus.pro have not been captured yet. Before this skill can run, a one-time Claude-in-Chrome exploration session must:

1. Open `app.opus.pro` logged in
2. Observe + record stable selectors for: upload dropzone, clip library grid, individual clip detail view, Enhancer toggles (Remove filler words, Remove silences), Apply button, Schedule modal, platform multi-select, date picker, time picker, Confirm button
3. Write those selectors into `selectors.json` alongside this file
4. Update the Phase sections below with concrete `$B js` / `$B click` commands

Everything else (config shape, flow, scheduling math) is already decided.

## Why this skill

Manual Opus Clip workflow: upload stream → wait → open each clip → toggle Enhancer → save → open scheduler → pick 4+ platforms × date × time per clip → repeat. For a 2-hour stream generating ~20 clips across 6 connected channels at 5/day/platform, that's ~80–120 manual clicks. This skill automates the clicks.

## Connected accounts (as of 2026-04-19)

| Platform | Account | Posts/day cap |
|---|---|---|
| Facebook | Enterprise Vibe Code | 5 |
| Instagram | Enterprise Vibe Code | 5 |
| LinkedIn page | Enterprise Vibe Code | 5 |
| LinkedIn profile | Mike Lady | 5 |
| TikTok | mikelady | 5 |
| YouTube | Enterprise Vibe Code | 5 |

Six channels total. Same clip posts to all of them; the 5/day cap is per channel, so 5 clips/day fills the queue everywhere.

## Scheduling math

- N clips generated × 6 channels = 6N scheduled slots
- 5/day/channel means N/5 days of content across all channels
- 20 clips → 4 days; 40 clips → 8 days

## Prerequisites

- `gstack browse` installed (`~/.claude/skills/gstack/browse/dist/browse`) — same tool as `crosspost-newsletter`.
- Opus Clip web login: a valid browser cookie jar imported via `$B cookie-import-browser app.opus.pro`. Set up once.
- Local video file path for the livestream (user provides as skill argument).
- `config.json` in this directory (committed) defines time slots, platforms, Enhancer settings. `config.local.json` (gitignored) overrides per-run.

## Key files

- `SKILL.md` — this workflow spec
- `config.json` — defaults: 5 daily time slots, 6 channels, Enhancer settings
- `selectors.json` — **TO BE CAPTURED** — per-page DOM selectors for Opus's UI
- `upload.sh` — **TBD** — drag-drop local file via gstack
- `wait_for_clips.sh` — **TBD** — polls DOM every 5 min until processing completes
- `process_clips.py` — **TBD** — iterates clips, toggles Enhancer, applies
- `schedule_clips.py` — **TBD** — assigns (channel, date, time) per clip from config, drives scheduler

## Workflow

### Phase 0 — Setup (one-time)

```bash
B=~/.claude/skills/gstack/browse/dist/browse
$B cookie-import-browser app.opus.pro   # Import Opus cookies from real browser
$B goto https://app.opus.pro           # Verify logged-in state
$B snapshot                            # Save baseline screenshot
```

### Phase 1 — Upload the livestream

```bash
$B goto https://app.opus.pro/upload    # Or whatever the actual upload URL is
$B upload '<dropzone selector>' /path/to/stream.mp4
# Wait for upload progress to finish (DOM poll)
```

Record the Opus project ID from the URL after upload completes — needed for later phases.

### Phase 2 — Wait for clip generation

Opus typically takes 10–30 min for a 1–2 hour stream. Poll the project page every 5 min:

```bash
until $B js "document.querySelector('<clip-list selector>')?.children.length > 0"; do
  sleep 300
done
```

Capture the count of generated clips. Report to user.

### Phase 3 — User review gate (clip selection)

**IMPORTANT:** Do not auto-process every clip. Opus generates good and bad clips; the user should approve the list before we burn time editing each one.

Produce a list:
```
Clip 1: "title" — 43s — viral score 72
Clip 2: "title" — 58s — viral score 89
...
```
Ask user which to keep. Default: all clips with viral score ≥ `config.viral_score_threshold` (default 70).

### Phase 4 — Per-clip Enhancer toggle

For each approved clip:
1. `$B click '<clip-card selector for clip N>'`
2. Wait for detail view to load
3. Open Enhancer panel: `$B click '<enhancer-button selector>'`
4. Toggle **Remove filler words** on (if not already)
5. Toggle **Remove silences** on (if not already)
6. Any other settings from `config.enhancer_settings`
7. Click **Apply** / **Save**
8. Wait for re-render
9. Close clip detail, return to library

Rate: ~30s/clip. 20 clips = ~10 min.

### Phase 5 — Schedule

Build the schedule from config + number of approved clips:
- Clip 1 → day 1, slot 1 (09:00 local)
- Clip 2 → day 1, slot 2 (12:00 local)
- Clip 3 → day 1, slot 3 (15:00 local)
- Clip 4 → day 1, slot 4 (18:00 local)
- Clip 5 → day 1, slot 5 (21:00 local)
- Clip 6 → day 2, slot 1
- …

For each clip, open the Schedule modal and:
1. **IF Opus supports multi-platform scheduling in one modal** — select all 6 channels at once, pick date + time, confirm. 1 action = 6 scheduled posts.
2. **IF not** — loop per channel: select channel, pick date + time, confirm. 6 actions per clip.

The UI exploration session MUST answer this. It's the single biggest factor in total run time.

**Time zone:** times in config are interpreted as `config.timezone` (default `America/Los_Angeles`). Convert to whatever timezone Opus's scheduler expects (usually matches user's account setting).

### Phase 6 — Review + summary

Print a table:

| Clip | Viral score | Scheduled for | Channels |
|---|---|---|---|

Report: total clips scheduled, total posts queued, next 24 hours of posts.

## Gotchas (anticipated — confirm during UI exploration)

- **Opus detects headless automation** — React apps often do. If so, switch from gstack to `mcp__claude-in-chrome__*` tools (same fallback as `crosspost-newsletter` uses for Medium). Claude in Chrome runs in the user's real browser, bypasses detection.
- **Clip list may lazy-load** — scroll-to-bottom before iterating.
- **Schedule modal may reset selections** — when switching clips, the date/time picker may reset to defaults. Re-set per clip.
- **Multi-platform scheduling** — if Opus charges per-platform seats on Auto Post but not on manual schedule, make sure we're not accidentally triggering paid features. Surface the pricing page in the exploration session.
- **Processing timeout** — a very long stream may take hours. If wait loop runs too long (>2 hr), surface and ask the user to confirm before continuing.

## Cost

Opus Clip cost is your existing subscription — this skill adds no per-use API charges. Time cost: ~15–30 min of automation runtime per livestream, depending on stream length and clip count.

## Out of scope

- Uploading the livestream to YouTube first (use a separate skill or manual)
- Captions / branding customization (configured once in Opus dashboard Brand Templates; referenced by name, not controlled from here)
- Cross-posting to Buffer in parallel (Opus handles the scheduling, Buffer is not involved in this skill)
- X / Twitter (not a connected account per 2026-04-19 review)

## Related skills

- `../crosspost-newsletter/SKILL.md` — reference patterns for gstack browse automation, cookie imports, selector-based clicking, handoff-on-failure.
