# opus-clips — improvement backlog

Captured 2026-04-20 after Phase D live run + skill rewrite (commit `ea16938`).
Items here are concrete, each with a rationale from what Phase C/D surfaced and
a rough "how to implement" note. Order within each tier is rough priority.

## Already landed (for reference)
- ✅ Remove-pauses subpanel subflow in `process_clips.py`
- ✅ `verify_schedule.py` — calendar-based post-commit verification
- ✅ Drive-primary ingest (`drive_upload.py` + `upload.sh` handoff runbook)
- ✅ 30-90s clip length preset enforced on `/workflow`
- ✅ `posts_per_day_cap` + day rollover in `schedule_clips.py`
- ✅ User-owned OAuth client path in `gen-lang-client-0527845499` (avoids rclone shared-quota)

---

## High-impact

### 1. Log writer + idempotency check
**What:** Implement the `/tmp/opus-clips-<projectId>.log` writes referenced throughout SKILL.md.
**Why:** Re-running the skill on the same video currently creates duplicate Drive uploads + duplicate Opus projects. No way to resume mid-run after a crash.
**How:** Add `opus-clips/_log.py` helper with `log(phase, step, **kv)` appending JSONL. `upload.sh` + `process_clips.py` + scheduler emit "write log" steps. At start of each skill run, check if `/tmp/opus-clips-<projectId>.log` exists for the target video filename — offer resume.

### 2. Bulk schedule modal flow
**What:** Replace the per-clip `Publish on Social` loop with a single Bulk schedule pass.
**Why:** For 3 clips the loop is fine (~90s). For 20 clips it's 20× slower and 20× more places for a selector regression to bite. Phase A already captured the Bulk schedule modal selectors (`selectors.json → bulk_schedule_modal`).
**How:** New `schedule_clips.py --mode bulk` output. Walks: `Select` → `Select all` → `Bulk schedule` top-bar button → mega-modal. Per row, `Customize time` override using the plan output from this same script. Final `Schedule` button commits all at once.

### 3. Schedule commit verification (tight loop)
**What:** After clicking `Schedule` on each per-clip modal, immediately confirm the post landed before moving to the next clip.
**Why:** Phase D confirmed NO toast on schedule success. A silent failure today would let the skill blow through 3 clips and miss the commit entirely. `verify_schedule.py` runs at the end but doesn't catch per-clip failure mid-flow.
**How:** Poll `/auto-post/calendar` day-cell count OR the clip card's "Scheduled" badge within 5s. Fail loud per clip, stop the batch before scheduling the next one.

### 4. Credit cost pre-flight warning
**What:** Scrape the credit balance from the top-right badge before Phase 1 ingest. Warn user if balance < (estimated credits for this video).
**Why:** Phase D used 3 credits for a 3-min test video. A 2-hour livestream might cost 50-100 credits. Skill shouldn't burn a user's quota silently.
**How:** Pre-ingest: read `button` containing credit badge text (top-right `2,502` number in Phase D screenshots). Warn if within 20% of estimated cost. Estimated-cost table per stream length can live in config.

### 5. Filler-words verification via transcript diff
**What:** Before + after `Remove filler words` click, snapshot the transcript DOM. Diff for removed "um"/"uh"/"like" markers. Log the delta.
**Why:** Phase C/D observed: clicking Remove filler words on 3 clips never changed duration, never showed confirmation UI. Skill can't distinguish "no fillers detected" from "click didn't fire". Transcript diff gives signal.
**How:** Add to `process_clips.py` emit: two `js` steps around the filler-words click that capture `document.querySelector('[class*="transcript"]').innerText`. Skill compares before/after, logs `fillers_removed_count`.

---

## Medium-impact

### 6. Apply pause duration threshold from config
**What:** Honor `config.enhancer_settings.pause_duration_threshold_s` when Remove-pauses subpanel opens.
**Why:** Config has the field (default 0.5s) but `process_clips.py` doesn't set the slider — Opus uses its default of 0s, which over-trims.
**How:** Between "click Remove pauses" and "click Remove (N)", add a JS step that drags the slider (`input[type=range]`) to the configured value. Requires finding the slider selector + computing the pixel position from min/max range. Alternative: fire `input` + `change` events directly on the range input.

### 7. Default viral-score filter in clip review gate
**What:** Auto-filter clips to `>= viral_score_threshold` (default 70) in the review phase; only surface those by default, still allow user to expand.
**Why:** Phase D skipped clip #5 (score 66) manually. Skill should make this the default, not rely on human inspection of all 5.
**How:** In Phase 3 enumeration (the scroll-then-parse snippet), filter the returned list in-memory. Print: "N clips ≥ threshold; M below. Show all? (default: just the N)".

### 8. Lazy-load scroll built into the enumeration snippet
**What:** Bake the clip-grid scroll pattern into a reusable JS snippet so clip #4, #5 aren't missed.
**Why:** Phase D hit this — initial DOM query found 3 of 5 clips; had to scroll the container to load #4 and #5. Currently the snippet is inline in SKILL.md; not in any helper file.
**How:** Add `opus-clips/_js_snippets.py` with `enumerate_clips_js()` returning a ready-to-run async JS string. Referenced from SKILL.md Phase 3.

### 9. Per-platform caption review gate
**What:** Before committing `Schedule`, surface the AI-generated caption for each platform and ask user to approve/regenerate.
**Why:** Opus's AI caption is auto-applied. Phase D noticed clip #1 Instagram got "Trying new things feels scary..." — OK but user might want to tweak tone, add CTA, etc. No review point today.
**How:** In Phase 5 scheduler, before Schedule click, scrape the 6 caption `<input>` values (Opus uses `FACEBOOK_PAGE~...title`, `YOUTUBE~...title`, etc.). Print them. Wait for user approve/edit/regenerate.

### 10. OAuth token persistence (user-directed)
**What:** Persist the rclone refresh token once with explicit user approval so the Drive upload doesn't need a re-auth every session.
**Why:** Today: every new session runs `rclone authorize drive <client_id> <client_secret>` which opens browser + OAuth flow. Fine for a one-time test but adds ~30s/run for real use.
**How:** After first auth, prompt: "Save token to `~/.config/rclone/rclone.conf` for future runs? [y/N]". On yes, write the standard rclone config (with the token embedded). Safety system will allow this because user explicitly directed the path.

### 11. Timezone auto-detect
**What:** Read user's Opus account timezone from a known location (e.g., Settings page, or the scheduler modal's timezone pill) instead of hardcoding `America/Los_Angeles`.
**Why:** Cross-timezone users or a future switch would silently schedule posts for the wrong local time. Phase A confirmed Opus uses account tz, not browser.
**How:** One-time fetch at Phase 0 setup: navigate Settings or read the scheduler-modal's `GMT-07` pill, parse into IANA tz, cache in `config.local.json`.

### 12. Processing-time estimator
**What:** Predict expected clip-generation wait time based on video duration.
**Why:** Phase D: 3-min video → ~5 min wait. 2-hour livestream could be 30-60 min. User should know before walking away.
**How:** Read video duration from `drive_upload.py` upload (`ffprobe` or similar) OR from `/workflow` page's displayed duration. Print ETA.

---

## Low-impact / polish

### 13. Schedule modal Close-button fallback
**What:** Codify the Escape-dispatch fallback for when `Close` doesn't dismiss cleanly.
**Why:** Phase A flagged this as intermittent. Phase D didn't hit it but it's lurking.
**How:** Wrap any "close modal" step in try-`Close`-then-`Escape`. Already documented in SKILL.md Debugging.

### 14. Auto-screenshot at critical state transitions
**What:** Emit `mcp__claude-in-chrome__computer action=screenshot` at: post-upload, post-processing, post-enhance per clip, post-schedule commit.
**Why:** When something goes wrong in a long unattended run, the user has no visual timeline to debug from.
**How:** Each helper emits an optional `screenshot` action step. Gate behind `--screenshot` flag to avoid bloat in normal runs.

### 15. Rewrite `wait_for_clips.sh` in Python
**What:** Consolidate the shell wrapper into Python so it can share helpers with `process_clips.py` / `schedule_clips.py`.
**Why:** Two bash + three Python helpers is inconsistent. Shell version was quick scaffolding; Python lets us share the JSON-plan builder + selector loader.
**How:** `wait_for_clips.py`. Same JSON output, same CLI surface.

### 16. Unit tests for schedule math
**What:** Add `test_schedule_clips.py` covering day rollover, pre-scheduled offset, tz edge cases (DST boundaries), cap enforcement.
**Why:** Zero tests today. Easy to regress on a timezone change or cap refactor.
**How:** Pytest-style. Core logic is `build_schedule()` — already pure. Test: `n=5, start=DST-fall-back day`, `n=10, pre_scheduled=3`, `cap > slots raises`, etc.

### 17. Opus UI drift detection harness
**What:** A nightly-runnable script that opens Opus dashboard, navigates to a known project, and asserts key selectors still exist. Alert if any fail.
**Why:** `selectors.json` is hand-captured. When Opus changes their UI, the skill breaks silently until a real run fails. An eval harness catches drift before a skill invocation.
**How:** `check_selectors.py` loads selectors.json, navigates each documented page, queries each selector, reports green/red. Run via cron or skill-health-check command.

### 18. Editor hydration wait fallback
**What:** If editor page body text stays <200 chars after 10s, force a reload or foreground-tab attention trigger.
**Why:** Phase A hit this — the editor sometimes didn't fully hydrate in background Claude-in-Chrome tabs. Currently no handler.
**How:** Add a wait-with-recovery step to `process_clips.py` Phase 4 step 2. If `body.innerText.length < 200` after 10s, try `location.reload()` once.

---

## Out-of-scope / deferred

### Opus API direct integration
Opus does not publicly expose a clip-processing API. We verified during Phase A that upload/schedule is browser-only. Would require enterprise deal.

### Shared Drive / Workspace
Would unlock Service Account upload (SA can own files in Shared Drives). Costs $5-10/mo for a personal Workspace. Current OAuth-user path works; revisit only if OAuth refresh becomes a pain.

### Automated Drive picker selection
Phase C proved `docs.google.com/picker` iframe rejects CDP-dispatched clicks (Google isTrusted guard). Not fixable from outside the browser. User's 3-click handoff stands.

### Direct file-chooser automation
Chrome's user-activation guard on `<input type=file>` cannot be bypassed via CDP mouse OR keyboard events. Confirmed Phase C. Would require restarting Chrome with `--remote-debugging-port=9222` + `--user-data-dir=<fresh>`, which loses the user's session. Not worth the trade.

---

## Implementation tips

- Each improvement should be self-contained — merge-able in isolation.
- Preserve the "emit JSON runbook, skill executes" pattern. Don't sneak in direct MCP calls from helper scripts (they run outside Claude's tool surface).
- Match existing testing style: each helper `./script.py --help` + `--dry-run`.
- When touching `selectors.json`, bump `_captured_at` and note what Phase/commit introduced the change.
- All improvements touching scheduling math must honor `posts_per_day_cap` (user-confirmed diminishing returns above 5/day).
