---
name: yt-ab
description: Use when the user wants to A/B test a YouTube video's title or thumbnail using YouTube Studio's native Test and Compare feature. Generates 3 candidate titles from the transcript and drives Studio via Claude-in-Chrome to set up the test. Triggers on "ab test", "a/b test", "test titles", "test thumbnail", "test and compare", "split test", "best title for this".
user_invocable: true
---

# yt-ab

A/B test a YouTube video's title (and optionally thumbnail) using YouTube Studio's **native** Test and Compare feature. Generates candidate titles from the video's transcript, then drives Studio in your logged-in Chrome to kick off the test.

## Cadence — read this before answering ANY question about timing

The user may ask "should I test every hour / 4 hours / half-day?" before we start. Here is the right answer (sourced from YouTube Help, TubeBuddy guides, Descript guides — verified April 2026):

**Native YT Test and Compare (titles + thumbnails — what THIS skill does):**
- Concurrent A/B serving — YouTube distributes impressions across up to 3 variants simultaneously. **There is no cadence to pick.**
- Test runs up to **2 weeks**, then YT picks the winner.
- **Winner is decided by watch time, not CTR.** This is counterintuitive — most "boost CTR" advice points at the wrong metric. Surface this to the user.
- Up to **3 variants per test** (titles, thumbnails, or combinations of both).
- **Can start immediately after publish.** Concurrent serving handles launch-day algorithm skew internally because all variants are exposed during the same impression burst.
- A test result of "inconclusive" means view counts were too low for a decisive call. Higher views = more decisive.

**Manual sequential rotation (only relevant for things YT doesn't natively test, like descriptions — NOT what this skill does):**
- Industry standard: 24 hours per variant, switching at midnight Pacific (matches YT Analytics' day boundary).
- Wait 2-3 weeks after publish before starting a manual rotation — early data is biased by YT's new-video promotion bump.
- Hourly / 4-hourly / half-day rotation is measurably worse experimental design (confounds time-of-day with variant effect).

**TL;DR for the user**: "Native YT Test and Compare doesn't take a cadence — it serves all 3 variants concurrently and picks a winner in ~2 weeks based on watch time. So 'every hour vs 4 hours vs half-day' doesn't apply for titles or thumbnails. We'll just kick off the test and let YT run it."

## When this fires

- `/yt-ab <url>` (explicit)
- "A/B test this title"
- "Test some title variants for [video]"
- "Set up Test and Compare for [video]"
- "What's a better title for this?"

## Args

- `$1` — YouTube video URL OR bare 11-char ID (required).
- `--transcript <path>` — optional explicit transcript file path. Defaults to fetching via `scripts/generate-transcript.sh`.

## Steps

### 1. Resolve the video ID

Same regex as `/yt-launch`. Verify the video is on the user's channel (Enterprise Vibe Code, `UCyxYcJWs5WstpM2h8KjabUw`) — `go run . video <id>` will surface its presence in `data/videos.json`.

### 2. Get the transcript

Try in order:

1. If `--transcript <path>` provided, read that file.
2. Else: `bash scripts/generate-transcript.sh <id>` from the repo root. The script wraps the `youtube-transcript-api` CLI (installed via `pipx install youtube-transcript-api`).
3. If the script reports the package isn't installed, surface the install command and offer to fall back to step 4.
4. Last resort: read the video's existing `description` field from `data/videos.json` and warn the user that candidates will be lower-quality without the full transcript.

The transcript is cognition input — not stored on disk by this skill.

### 3. Show baseline numbers

```bash
cd /Users/mikelady/dev/claude-social-media-skills/youtube-analytics
go run . video <id>
```

Read its current title, view count, retention, traffic-source mix. Use these to inform candidate generation (e.g., if SUBSCRIBER traffic is dominant, your test is testing on viewers who already know the channel — calibrate the angle accordingly).

### 4. Generate candidate titles (cognition — your judgment)

You have THREE grounding inputs. Read all three before drafting:

**(a) Transcript** — what the video actually says. Don't lie.

**(b) Channel voice (what's worked HERE):**
```bash
go run . titles --top 10 --by retention --type long-form
```
Mike's channel tone: technical, candid about cost ($200/mo Claude), not-clickbait. Avoid all-caps shouting, em-dash overuse, and AI-slop tells like "REVEALED" / "INSANE".

**(c) Peer-niche signal (what's working ACROSS the AI-coding niche right now):**
```bash
jq -r '.fetched_at, (.channels | to_entries[] | "=== \(.key) (\(.value.angle))===", (.value.videos[] | "  \(.view_count) views: \(.title)"))' data/peer-videos.json
```
Updated weekly by `fetch-peers`. Each peer has a one-line `angle` (Fireship's hyper-tight news, t3dotgg's opinion-led POV, indydevdan's narrative outcome promise, etc). Use the contrast between Mike's voice and peer angles to pick distinct hook archetypes. **Do NOT mimic peer voices wholesale** — they're inspiration for *angle* (outcome-led, surprise-led, identity-led), not for word-for-word style. Mike's voice still wins.

Produce **3 alternative candidates**. (The native test fits **3 variants total**: the original as control plus **2** of your alternatives. Generating 3 gives the user a real choice of which 2 to run.) Constraints:

- **≤60 characters each** (browse-page truncation sweet spot for desktop + mobile).
- **Distinct angles** — don't generate near-duplicates. Cover three of these archetypes (which to pick depends on the video; surface your choice to the user):
  - **Outcome-led**: what the viewer GETS by watching ("Stop AI Slop: Enterprise Rigor for Vibe Coding")
  - **Surprise-led**: hook that interrupts pattern-matching ("Why I Went to Cal Poly to Trash 'Vibe Coding'")
  - **Identity-led**: speaks to the viewer's self-image ("Senior Devs: Your New Job Is Yield Management")
  - **Provocative imperative** (t3dotgg-style): a clear directive that challenges a default ("Delete Your CLAUDE.md")
  - **News-reaction** (Fireship-style): subject + dramatic verb ("Anthropic Killed Tool Calling")
  - **Concrete-deliverable** (AI-Jason-style): "Build X with Y" promise, framework name forward
- **Cite content from the transcript** — title should be honest to what's actually in the video.

Show the original + 3 alternatives to the user with the angle you picked for each AND a one-liner about why. Then use AskUserQuestion (`multiSelect: true`) to have them pick **which 2 alternatives** go into the test alongside the original — the native dialog fits only 3 total. Let them swap wording too. Iterate until they say go.

**If `data/peer-videos.json` is missing or stale** (>14 days old): tell the user `go run . fetch-peers` would refresh peer-niche signal, then proceed with channel-voice-only grounding. Don't block on it.

### 5. Drive YouTube Studio's Test and Compare

This step uses `mcp__claude-in-chrome` against the user's already-logged-in Chrome. Before any browser tool, you MUST first load the chrome tools via ToolSearch (the MCP server's tools are deferred):

```
ToolSearch select:mcp__claude-in-chrome__tabs_context_mcp,mcp__claude-in-chrome__tabs_create_mcp,mcp__claude-in-chrome__navigate,mcp__claude-in-chrome__find,mcp__claude-in-chrome__click,mcp__claude-in-chrome__form_input,mcp__claude-in-chrome__get_page_text,mcp__claude-in-chrome__take_screenshot
```

Then:

1. **Check current tabs** with `tabs_context_mcp` (with `createIfEmpty: true` if no MCP tab group exists yet). Reuse if Studio is already open on this video.
2. **Open Studio** with `tabs_create_mcp` if needed, then `navigate` to `https://studio.youtube.com/video/<id>/edit` — the edit page is the right starting point. Test and Compare is NOT at top-level URLs like `/abtesting` or channel-level `/test_and_compare` (both 404). Don't waste turns trying.
3. **Find the native A/B Testing button.** It lives directly under the Title field on the edit page — labeled "A/B Testing" with a panel/screen icon. **NOT to be confused with TubeBuddy's "A/B Testing" link** (orange "tb" logo) which is the third-party rotation tool. They sit close together on the page. Use `find` with this query to disambiguate: `A/B Testing button under the title field, not a link to tubebuddy`. Available on brand-new videos (verified 2026-04-27 on a 10-hour-old video with 22 views — earlier assumptions about an impressions threshold were wrong).

4. **Open the dialog and pick the test type.** Click the button — a modal opens with three tabs: "Title only", "Thumbnail only", "Title and thumbnail". Default to "Title only" unless the user requested otherwise.

5. **Fill the variant fields.** The dialog shows three input fields:
   - Title 1 is pre-filled with the current title (the original) — don't touch it.
   - Title 2 = the user's first chosen alternative (required).
   - Title 3 = the user's second chosen alternative (optional but recommended — 3 variants > 2 for statistical power).

   The fields are contenteditable divs (NOT standard `<input>`/`<textarea>`), so `form_input` errors with `Element type "DIV" is not a supported form input`. Use `left_click` on the field's ref, then `computer` action `type` with the variant text. After all required fields are filled, the dialog footer flips from "2nd title is required" to a green checkmark + "Title test ready" — that's the gate to the Set test button.

6. **Submit the test.** Click "Set test" (footer of the dialog). The dialog closes and a toast appears: `The test is ready and will start once you save your changes`.

7. **Save the video.** Click "Save" in the top-right (it goes from disabled-grey to filled-dark when there are unsaved changes). A "Changes saved" toast confirms; both Save and Undo go back to disabled. The test is now LIVE.

8. **Screenshot** the post-save state for the experiment record. Don't fake success — if any step fails, surface clearly to the user.

### 6. Record the experiment

Write `data/launch-experiments/<id>.yaml`:

```yaml
video_id: <id>
title: "<original title>"
published_at: <RFC3339 from videos.json>
test_kind: title
mode: native
variants:                          # exactly the 3 the test runs: original + the 2 the user chose
  - kind: original
    text: "<original title>"
  - kind: <angle of chosen alt 1>   # e.g. surprise-led
    text: "<chosen alternative 1>"
  - kind: <angle of chosen alt 2>   # e.g. identity-led
    text: "<chosen alternative 2>"
started_at: <RFC3339 UTC now>
estimated_end_at: <started_at + 14 days>
status: running
winner: ""
notes: "<e.g. which 3rd candidate was generated but not run, and why>"
```

The file is gitignored by default (per `.gitignore`'s `data/launch-experiments/` rule). The Sunday weekly review reads these and surfaces running tests in the report.

### 7. Commit (the experiment YAML is gitignored, so this only commits if other files changed)

If you modified anything tracked (e.g. the SKILL.md), commit + push. Otherwise just confirm to the user that the experiment is local-only.

### 8. Report to user

- ✓ Test running in YT Studio with 3 variants (1 original + 2 alternatives)
- ✓ YT will pick winner by **watch time** in ~2 weeks (mention this — counterintuitive!)
- ✓ Recorded to `data/launch-experiments/<id>.yaml`
- The Sunday weekly review will surface this experiment's status

## What this skill does NOT do

- **Doesn't auto-rotate titles via the Data API.** That fights with the native test. Native test is the single source of truth.
- **Doesn't generate or upload thumbnails.** Image generation is out of scope. The skill describes thumbnail concepts in text if asked but the user makes/uploads images via Studio.
- **Doesn't run description rotations.** That's a different workflow (see cadence section above) and would need a separate skill.
- **Doesn't grade the test.** YT decides the winner. The Sunday review just reads the verdict YT delivers and updates the YAML.

## Reference

- Cadence sources: [YouTube Help: A/B test titles and thumbnails](https://support.google.com/youtube/answer/16391400), [TubeBuddy A/B Testing FAQs](https://support.tubebuddy.com/hc/en-us/articles/21191305824027), [Descript: How to A/B Test on YouTube (2026)](https://www.descript.com/blog/article/how-to-ab-test-on-youtube-for-better-video-performance)
- Primitive Test: `docs/primitive-test.md` — generating candidates is cognition (in this prompt). Driving the browser is transport (Chrome-MCP). Recording the experiment is transport (file write). Grading is YT's job.
- Channel: Enterprise Vibe Code (`UCyxYcJWs5WstpM2h8KjabUw`).
- Setup: `pipx install youtube-transcript-api` once for transcript fetching.

## This skill lives in the repo

Symlinked from `~/.claude/skills/yt-ab` → `/Users/mikelady/dev/claude-social-media-skills/youtube-analytics/skill/yt-ab`. Edit in the repo; symlink picks up changes.
