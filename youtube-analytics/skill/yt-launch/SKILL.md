---
name: yt-launch
description: Use when the user wants to set up monitoring for a freshly published YouTube video on the Enterprise Vibe Code channel. Tags the video into a cohort, writes a launch hypothesis to the insights ledger, and installs the daily launch-watch LaunchAgent. Triggers on "launch", "track this video", "monitor this drop", "watch the new one", "kick off launch monitoring".
user_invocable: true
---

# yt-launch

Set up a launch-window monitoring stack for a single video. Generalizes the manual sequence used for the Manifesto (`H9HR4_Qz-Z0`) into a one-shot command.

## When this fires

User just published (or is about to publish) a video and wants to track its launch. Triggers include:

- `/yt-launch <url>` (explicit)
- "Set up monitoring for this video"
- "Track this drop / launch / new one"
- "Watch this for the next few days"

If the user wants to A/B test the title or thumbnail, run `/yt-ab` separately AFTER this. They're orthogonal.

## Args

`$1` — YouTube video URL OR bare 11-char ID (required).

## Steps

### 1. Resolve the video ID

Accept any of:
- `H9HR4_Qz-Z0` (bare ID)
- `https://www.youtube.com/watch?v=H9HR4_Qz-Z0` (full URL)
- `https://youtu.be/H9HR4_Qz-Z0` (short URL)
- URLs with extra params (`&t=42s`, `&list=...`)

### 2. Refresh data so the video is captured

```bash
cd /Users/mikelady/dev/claude-social-media-skills/youtube-analytics
go run . fetch          # 1-2 min; idempotent if already current
```

This pulls the new video into `data/videos.json`. If the user just published, the video may not be there yet — `fetch` should pick it up.

### 3. Show the baseline

```bash
go run . video <id>
```

Read the deep-dive output. Capture:
- Title
- Published timestamp
- Initial views/likes/duration
- Whether `--all` analytics dimensions are populated yet (likely sparse for a brand-new video)

### 4. Ask for cohort + hypothesis

Use AskUserQuestion to confirm two things — but propose smart defaults so the user can usually accept:

**Cohort:** default `evergreen-explainers` for substantive long-forms. Offer the existing list:
```bash
go run . cohort list
```
The user can pick an existing one or say "new" and you'll need to add it to `data/cohorts.yaml` first.

**Hypothesis prediction:** default to:
```
<title> launch — predict ≥500 views, ≥30% retention, ≥5 subs/1K UNSUBSCRIBED, ≥10% RELATED_VIDEO traffic by <today + 7 days>.
```
Adjust the numbers based on channel-typical performance you can see from `cohort report` for the chosen cohort.

### 5. Idempotency check

Before making changes, check whether the video is ALREADY set up:
- Is it in `data/cohort_assignments.json` for the chosen cohort? `jq '.\"<id>\"' data/cohort_assignments.json`
- Is there an active hypothesis citing it? `grep -l "<id>" data/insights/*.md`
- Is the launch-watch LaunchAgent already pointing at it? `launchctl list | grep yt-launch-watch` and check the plist

If all three are present and current, report "already monitored" and stop. Don't double-assign.

### 6. Execute the setup

```bash
# Tag into cohort
go run . cohort assign <cohort> <id>

# Append hypothesis to this Monday's insights file
THIS_MONDAY=$(date -v-mon +%Y-%m-%d 2>/dev/null || date -d 'last monday' +%Y-%m-%d)
INSIGHTS_FILE="data/insights/$THIS_MONDAY.md"
[ -f "$INSIGHTS_FILE" ] || go run . insights new "$THIS_MONDAY"
```

Then EDIT the insights file directly (using the Edit tool) to append a new hypothesis to the frontmatter `hypotheses:` list. Use this exact YAML shape:

```yaml
- id: h-<this-monday>-launch-<id-prefix>     # e.g. h-2026-04-27-launch-H9HR4
  cohort: <cohort>
  prediction: "<the prediction text>"
  evidence_video_ids:
    - <id>
  metric: launch_week_composite
  direction: up
  evaluate_after: "<today + 7 days>"
```

And append a markdown section below the frontmatter explaining the hypothesis (cite baseline numbers from step 3).

### 7. Install the launch-watch LaunchAgent

```bash
make launch-watch-install VIDEO=<id>
```

This is the daily 9 AM + 9 PM monitor. It REPLACES any prior launch-watch agent — only one video at a time. If a prior video was being watched, mention this to the user before clobbering.

### 8. Commit + push

```bash
git add data/cohort_assignments.json data/insights/<this-monday>.md
git commit -m "Set up launch monitoring for <id> (<title>)"
git push
```

The pre-commit hook will run `go vet` + `go test`. (No Go files touched, so this is fast.)

### 9. Report to user

Confirm what's running:

- ✓ Cohort: tagged into `<cohort>` (now N members)
- ✓ Insights: hypothesis `h-<date>-launch-<prefix>` written, evaluates `<eval-date>`
- ✓ LaunchAgent: `com.mikelady.yt-launch-watch` watching `<id>`, fires 9 AM + 9 PM local
- Logs: `~/Library/Logs/yt-launch-watch/<id>/`
- Mention: run `/yt-ab <id>` to set up native title testing in YouTube Studio
- Mention: `make launch-watch-uninstall` after the launch window stabilizes (~5 days)

## What this skill does NOT do

- **Doesn't drive YouTube Studio.** Title/thumbnail testing is `/yt-ab`'s job.
- **Doesn't fetch fresh analytics.** Brand-new videos have sparse data anyway; the daily LaunchAgent handles ongoing capture.
- **Doesn't grade hypotheses.** The Sunday weekly review handles grading once `evaluate_after` passes.
- **Doesn't auto-detect launches.** The user invokes this manually when they ship.

## Reference

- Cohort definitions: `data/cohorts.yaml`
- Insights ledger format: `data/insights/<date>.md` (YAML frontmatter, see existing entries)
- Primitive Test: `docs/primitive-test.md` — this skill is pure orchestration of existing CLI primitives; cognition (cohort choice, hypothesis wording) is the model's job.
- Channel: Enterprise Vibe Code (channel ID `UCyxYcJWs5WstpM2h8KjabUw`).

## This skill lives in the repo

Symlinked from `~/.claude/skills/yt-launch` → `/Users/mikelady/dev/claude-social-media-skills/youtube-analytics/skill/yt-launch`. Edit in the repo; symlink picks up changes.
