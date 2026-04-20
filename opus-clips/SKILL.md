---
name: opus-clips
description: Use when user wants to turn a livestream video file into short-form social clips via Opus Clip — "clip my stream", "make shorts from this stream", "run opus on this video". Uploads to Drive, hands the 3-click picker to the user, waits for AI clip generation (30-90s shorts only), applies AI enhance (Remove filler words + Remove pauses subpanel), then schedules at 5 per day (hard cap, diminishing returns above) across 6 connected channels via Opus's per-clip Publish-on-Social modal.

user_invocable: true
---

# opus-clips

End-to-end automation for Opus Clip: livestream → 30-90s shorts → scheduled posts. Everything except three picker clicks at ingest is automated; the rest of the flow (clip generation wait, AI enhance per clip, schedule fan-out to 6 channels, schedule verification) runs unattended.

## Why this skill

Manual Opus Clip workflow for a 2-hour livestream: upload → wait 10-30 min → open each clip → AI enhance → Save → Publish on Social → pick platforms × date × time → repeat. For 20 clips across 6 channels, that's 80-120 clicks. This skill automates every click except **one human gesture at ingest** (Google Picker iframe requires a real HID click — see "Ingest limitation" below).

## Non-negotiables

Two hard rules from the user:

1. **30-90s shorts only.** On Opus's `/workflow` setup page, the `Clip Length` selector MUST be set to `30s-90s` (not the default `Auto (<90s)`). Configured in `config.workflow_setup.clip_length_preset`.
2. **5 videos/day/channel is the hard cap**, not a lower bound. Diminishing returns above this — skill must NEVER schedule a 6th post on the same calendar day. Configured in `config.posts_per_day_cap`. `schedule_clips.py` rolls over to the next day at slot 6.

## How it runs

Driven by **Claude-in-Chrome MCP tools** (real browser). Opus is a Next.js/Radix SPA — gstack (Playwright headless) cannot log in because auth tokens live in localStorage, not cookies. The helper scripts in this directory emit JSON runbooks that the skill executes via `mcp__claude-in-chrome__*` tools inline. No separate runner process.

## Connected accounts (as of 2026-04-19)

| Platform | Account | Posts/day cap |
|---|---|---|
| Facebook | Enterprise Vibe Code | 5 |
| Instagram | Enterprise Vibe Code | 5 |
| LinkedIn page | Enterprise Vibe Code | 5 |
| LinkedIn profile | Mike Lady | 5 |
| TikTok | mikelady | 5 |
| YouTube | Enterprise Vibe Code | 5 |

Six channels. One Opus "Schedule post" modal publishes a clip to all 6 at once.

## Scheduling math

- 1 approved clip = 1 Opus schedule action = 6 platform posts (fan-out happens inside the modal)
- 5 slots per day × 1 clip per slot → 5 clips/day max (per diminishing-returns rule)
- N approved clips → ceil(N/5) days
- 5 slots local PT: 09:00 / 12:00 / 15:00 / 18:00 / 21:00

Run `./schedule_clips.py --n-clips N --start-date YYYY-MM-DD` for the plan. Use `--pre-scheduled K` if the start-date already has K slots committed (rare; mostly for rerun recovery).

## The ingest limitation (Phase C finding, 2026-04-19)

Opus's upload presents three ingest options: local **Upload** button, **Google Drive** picker, link-paste (Rumble/Twitch/YouTube/Zoom). Every path requires exactly ONE real human click:

| Path | Blocker |
|---|---|
| `Upload` → native file chooser | Chrome requires `isTrusted=true` for `<input type=file>` activation; CDP mouse + keyboard events both fail |
| `Google Drive` → in-page picker | `docs.google.com/picker` iframe is cross-origin; Google's isTrusted guard ignores CDP clicks |
| Link-paste (Rumble/YouTube/etc.) | Accepts specific video-platform URLs only, not Drive share URLs |

**Adopted path: Drive upload + picker handoff.**
1. Skill uploads video to Drive folder `opus-clips-automation` via `drive_upload.py` (fully automated). ~60-120s for a 300MB file.
2. Skill opens Opus dashboard + clicks `Google Drive` button (works — Opus's own origin).
3. **User performs 3 clicks in the picker:** `opus-clips-automation` folder → video file → `Select`. ~5 seconds.
4. Skill sets clip length to 30s-90s and clicks `Get clips in 1 click`.

Everything downstream — processing wait, clip enumeration, AI enhance, scheduler, post-schedule verification — is 100% automated.

## Prerequisites

- Opus Clip web login at `https://clip.opus.pro` in the Claude-in-Chrome browser
- Google Chrome with Claude-in-Chrome extension installed
- Local video file path
- **For automated Drive upload:** one-time setup
  - GCP project `gen-lang-client-0527845499` with Drive API enabled (confirmed, Phase C)
  - OAuth 2.0 Desktop Client ID + Secret in that project
  - An rclone-authorized refresh token (in-memory only per safety rules; re-auth once per session via `rclone authorize drive <client_id> <client_secret>`)
  - Drive folder `opus-clips-automation` (ID in `config.drive_upload.folder_id`) shared with the service account or accessible to your user

## Key files

- `SKILL.md` — this workflow spec
- `config.json` — channels, slots, enhancer settings, clip-length preset, posts_per_day_cap, Drive credentials
- `selectors.json` — DOM selectors (Phase A captured, Phase C/D corrected)
- `drive_upload.py` — resumable chunked Drive upload via our own OAuth client (avoids rclone's shared-project rate limit)
- `upload.sh` — emits the full 8-step ingest runbook (Drive upload → picker handoff → workflow setup → processing trigger)
- `wait_for_clips.sh` — emits polling runbook for `Original clips (N)` counter
- `process_clips.py` — emits per-clip AI-enhance plan including the Remove-pauses subpanel subflow
- `schedule_clips.py` — pure logic; computes (day, slot) per clip with cap enforcement
- `verify_schedule.py` — emits a plan to navigate `/auto-post/calendar`, expand each day cell, count `Scheduled` tokens per time slot, and assert against expected

Each helper supports `--help` and `--dry-run`.

## Workflow

### Phase 0 — Session setup

```
mcp__claude-in-chrome__tabs_context_mcp               # verify MCP tab group
mcp__claude-in-chrome__navigate → /dashboard          # confirm logged in
```

If not logged in, ask the user to sign in manually.

### Phase 1 — Ingest (Drive-first)

```
./upload.sh /path/to/stream.mp4
```

The emitted plan walks the skill through: Drive upload → navigate + click Google Drive → hand off to user for 3 picker clicks → set clip length to 30s-90s on `/workflow` → click `Get clips in 1 click` → extract `projectId` from redirect URL → log to `/tmp/opus-clips-<projectId>.log`.

**Idempotency:** before starting, check if `/tmp/opus-clips-<projectId>.log` exists for this video filename. If yes, offer to resume vs start fresh.

### Phase 2 — Wait for clip generation

```
./wait_for_clips.sh <projectId>
```

Polls `/clip/<projectId>` every 5 min (2 hr ceiling). Matches regex `/Original clips\s*\(\s*(\d+)\s*\)/` on the page body.

### Phase 3 — Clip review gate

Enumerate clip cards. Phase D learned: the clip grid is virtualized; must scroll the overflow container to the bottom before extraction to get ranks #4 and #5.

```js
// Scroll the main scrollable container to the bottom
const s = Array.from(document.querySelectorAll('*')).find(el => {
  const cs = getComputedStyle(el);
  return (cs.overflowY === 'auto' || cs.overflowY === 'scroll') && el.scrollHeight > el.clientHeight + 50;
});
for (let i = 0; i < 6; i++) { s.scrollTop = s.scrollHeight; await new Promise(r => setTimeout(r, 500)); }
```

Parse title, score, start/end from each `#N ... /100 ...` card. Default filter: `viral_score >= config.viral_score_threshold`. Ask user to approve the filtered set.

### Phase 4 — AI enhance per clip

```
./process_clips.py --project-id <projectId>.<firstClipHash> --clip-ranks 1,3,5
```

Per-clip sequence (Phase C/D verified):
1. Click `Edit clip` on card → navigates to `/editor-ux/{projectId}.{clipHash}?clipRank={N}`
2. Wait for `AI enhance` sidebar button → capture initial duration
3. Click `AI enhance` → panel opens
4. Click `Remove filler words` — **direct action**, no confirmation UI. May no-op on clips with no detected fillers (Phase C/D test clips all no-op'd). Skill logs this case but doesn't treat it as error.
5. Click `Remove pauses` → **subpanel opens** with slider + `Remove (N)` button where N = detected pause count
6. Click the `Remove (N)` button (regex `/^Remove\s*\(\d+\)$/`) → commits. Duration shrinks by reported seconds. Phase D evidence: 58s→52s (N=6), 24s→22s (N=2), 50s→46s (N=4).
7. Click `Save changes` → editor navigates back to `/clip/{projectId}` = implicit success
8. Log initial→final duration delta to `/tmp/opus-clips-<projectId>.log`

### Phase 5 — Schedule

Per-clip loop (Phase D flow):

1. Click `Publish on Social` on clip card → schedule modal opens with all 6 channels pre-selected + AI-generated per-platform copy
2. Click `Select time` → date+time popover appears
3. Click the date button (text matches target day-of-month) in the `rdp-*` calendar
4. Click the time combobox (text matches current time like `12:00 AM`) → listbox of 15-min slots opens
5. Click the option matching target time (format: `09:00 AM`, `12:00 PM`, `03:00 PM`)
6. Verify the pill label at the bottom shows `<DD> <Month> 2026 H:MM AM/PM GMT-07:00` then click `Schedule` → modal closes silently

**For large batches (N > ~5 clips):** consider the **Bulk schedule** modal (selectors.json → `bulk_schedule_modal`) which schedules all selected clips at once. Not yet used by this skill — Phase D looped per-clip for clarity on 3 clips.

**Important:** `Schedule` click produces NO toast/confirmation. Verify success via Phase 6.

### Phase 6 — Verify scheduled posts

```
./verify_schedule.py --n-clips N --start-date YYYY-MM-DD
```

Navigates `/auto-post/calendar`, expands each day cell (clicks `See N more`), counts `Scheduled` tokens and grouped times per day. Compares against expected:
- Per day: expected slot-times match wall-clock
- Per slot: count equals `len(config.channels)` (= 6)
- Total across all days: `n_clips * 6`

Phase D evidence (Apr 20, 3 clips): expected `{9:00 AM: 6, 12:00 PM: 6, 3:00 PM: 6}` = 18 posts; actual matched exactly.

## Debugging

- **Log location:** `/tmp/opus-clips-<projectId>.log`. Appended JSON per phase/step with `elapsed_sec`, `result`.
- **Editor not hydrating in Claude-in-Chrome tab:** observed Phase A — needs foreground tab before DOM queries. Fallback: `mcp__claude-in-chrome__computer action=screenshot` to confirm visual state, then retry.
- **AI enhance panel buttons invisible:** panel must be open. Click `AI enhance` first.
- **Remove pauses `Remove (N)` button not appearing:** means Opus detected 0 pauses above threshold. Skill treats as no-op success.
- **Schedule modal close bug:** if `Close` button doesn't dismiss, dispatch Escape: `document.dispatchEvent(new KeyboardEvent('keydown', {key:'Escape',bubbles:true}))`.
- **Clip count stuck in processing:** force-reload the project page once (`location.reload()`) if count hasn't budged in 15 min.
- **Drive upload 403 "Quota exceeded":** means rclone's default shared OAuth client was used. Switch to the user's own OAuth 2.0 client in `gen-lang-client-0527845499` and re-authorize.
- **Drive upload `storageQuotaExceeded` from SA:** expected — service accounts can't own files on personal Gmail. Use user OAuth credentials, not SA.

## Gotchas

- **`clip.opus.pro`, not `app.opus.pro`.**
- **No `data-testid` anywhere** — selectors are `aria-label`, `role`, `data-state`, `textContent`, `href`. Brittle if Opus changes copy.
- **Opus account timezone**, not browser timezone. Scheduler modal shows `America/Los_Angeles` (confirmed 2026-04-19).
- **Remove pauses is NOT one-click** — it opens a subpanel. Actual apply button is `Remove (N)` inside.
- **Remove filler words has no UI confirmation** — may silently no-op. Skill uses duration delta + optional transcript diff to infer outcome.
- **Schedule button success has no toast.** Must verify via `verify_schedule.py`.
- **Service accounts cannot own files on personal Gmail.** Shared Drives are a Workspace-only feature. SA → OAuth user credentials.
- **Google Picker iframe is docs.google.com origin.** Synthetic clicks from CDP do NOT reach it. User gesture required.
- **rclone's default OAuth client hits a global ~99% upload quota.** Always bring your own OAuth 2.0 Client ID from a personal GCP project when uploading >100MB videos.

## Cost

Opus Clip: 3 credits per clip generation (observed Phase D on 3-min test video). Subscription-based, no per-API charges. Runtime: ~15-30 min per stream depending on length, plus ~2-5 min of browser automation.

Drive upload: free within Google Drive storage quota. OAuth token refresh: free.

## Out of scope

- Uploading the livestream to YouTube first
- Captions / branding (set once in Opus Brand Templates)
- Cross-posting to Buffer in parallel (Opus handles scheduling directly)
- X / Twitter (not a connected account)

## Related skills

- `../crosspost-newsletter/SKILL.md` — reference patterns for browser automation, cookie imports, selector-based clicking, handoff-on-failure.
