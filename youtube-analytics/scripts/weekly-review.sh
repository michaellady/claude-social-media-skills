#!/usr/bin/env bash
# Sunday closed-loop weekly review (Pattern B — local + headless Claude).
#
# What this does:
#   1. Pulls latest main (in case the cloud routine pushed an insights scaffold)
#   2. Refreshes channel data: fetch, fetch-analytics --all, cohort auto,
#      fetch-comments since last Monday, fetch-peers (peer-niche titles)
#   3. Invokes `claude -p` headlessly with a prompt that drives the full
#      closed-loop weekly review — grading past-due hypotheses, proposing
#      new ones, writing the report
#   4. Saves the run log + report under data/reviews/<date>.md and pushes any
#      file changes Claude made (graded insights, new insights/<monday>.md)
#   5. Posts a macOS notification when finished
#
# Triggered weekly by ~/Library/LaunchAgents/com.mikelady.yt-weekly-review.plist
# (install with `make schedule-install`).
#
# Manual run: `bash scripts/weekly-review.sh` from the repo root.

set -euo pipefail

# --- locate the repo (script lives in scripts/, repo is one level up)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_DIR"

# --- log destination
LOG_DIR="$HOME/Library/Logs/yt-weekly-review"
mkdir -p "$LOG_DIR"
RUN_DATE="$(date +%Y-%m-%d)"
LOG_FILE="$LOG_DIR/$RUN_DATE.log"

# --- review report destination (gitignored — generated artifact)
REVIEW_DIR="$REPO_DIR/data/reviews"
mkdir -p "$REVIEW_DIR"
REPORT_FILE="$REVIEW_DIR/$RUN_DATE.md"

# --- compute date anchors for the prompt
THIS_MONDAY="$(date -v-mon +%Y-%m-%d 2>/dev/null || date -d 'last monday' +%Y-%m-%d)"
LAST_MONDAY="$(date -v-mon -v-7d +%Y-%m-%d 2>/dev/null || date -d 'last monday -7 days' +%Y-%m-%d)"
LAST_SUNDAY="$(date -v-mon -v-1d +%Y-%m-%d 2>/dev/null || date -d 'last monday -1 day' +%Y-%m-%d)"
PREV_MONDAY="$(date -v-mon -v-14d +%Y-%m-%d 2>/dev/null || date -d 'last monday -14 days' +%Y-%m-%d)"
PREV_SUNDAY="$(date -v-mon -v-8d +%Y-%m-%d 2>/dev/null || date -d 'last monday -8 days' +%Y-%m-%d)"

{
  echo "=== yt-weekly-review run $(date '+%Y-%m-%d %H:%M:%S %Z') ==="
  echo "Repo: $REPO_DIR"
  echo "This week:  $THIS_MONDAY .. (today)"
  echo "Last week:  $LAST_MONDAY .. $LAST_SUNDAY"
  echo "Prev week:  $PREV_MONDAY .. $PREV_SUNDAY"
  echo "Report:     $REPORT_FILE"
  echo

  # --- pull latest (cloud routine may have committed a scaffold)
  echo "--- git pull"
  git pull --ff-only origin main || echo "(pull skipped/failed; continuing with local state)"

  # --- ensure deps are ready
  echo "--- go mod download"
  go mod download

  # --- data refresh
  echo "--- fetch (YouTube Data API)"
  go run . fetch
  echo "--- fetch-analytics --all (per-day + traffic + sub-status)"
  go run . fetch-analytics --all
  echo "--- cohort auto"
  go run . cohort auto
  echo "--- fetch-comments (since last Monday, with quota guard)"
  go run . fetch-comments --since "$LAST_MONDAY" --limit 10 || echo "(comments fetch had errors; continuing)"
  echo "--- fetch-peers (refresh peer-niche signal for /yt-ab title grounding)"
  go run . fetch-peers || echo "(peer fetch had errors; continuing)"

  echo
  echo "--- invoking claude -p for cognition step ---"
} >> "$LOG_FILE" 2>&1

# --- build the prompt for claude -p.
# Write the template to a tmpfile (heredoc-to-file avoids the
# command-substitution parsing edge case). Substitute placeholders in place.
PROMPT_FILE="$(mktemp -t yt-weekly-review.XXXXXX)"
trap 'rm -f "$PROMPT_FILE"' EXIT

cat > "$PROMPT_FILE" <<'PROMPT_EOF'
Run the yt-analytics closed-loop weekly review for the youtube_analytics
project. Today is __RUN_DATE__. The data refresh has already happened
(fetch + fetch-analytics --all + cohort auto + fetch-comments) so do
NOT re-run those.

Your job is the cognition step.

1. Read past-due hypotheses with: go run . insights pending
   For each pending hypothesis:
     - Read the prediction and the cited videos current metrics in the
       command output.
     - Decide a verdict: confirm, refute, or inconclusive.
     - Persist with: go run . insights grade --verdict V --outcome "WHY" ID
       Flags MUST come before the positional id (Go stdlib quirk).

2. Pull comparison numbers:
     go run . cohort report --since __LAST_MONDAY__
     go run . analyze --compare-periods __PREV_MONDAY__..__PREV_SUNDAY__ __LAST_MONDAY__..__LAST_SUNDAY__

3. Surface running A/B experiments. List any data/launch-experiments/*.yaml
   files where status: running. For each: report the variants, started_at,
   estimated_end_at, and a one-liner suggestion to check Studio's Test and
   Compare view for the verdict (the agent does NOT navigate Studio in
   this Sunday run — that's the /yt-ab skill's job, invoked manually).
   When a running test passes its estimated_end_at, flag it for human
   verification.

4. Write the weekly review report to __REPORT_FILE__ with this structure:
     - Lead with cohort movement: per-cohort table comparing this week
       vs prior week. Cohorts are the unit of intentional decision-making.
     - WoW totals from the analyze --compare-periods call.
     - Verdicts on hypotheses you graded this cycle, with reasoning.
     - Running A/B experiments: status from data/launch-experiments/*.yaml
       (variant titles, started_at, estimated_end_at).
     - 1-3 NEW hypotheses for next week, each with:
         id like h-__THIS_MONDAY__-N, cohort, prediction,
         evidence_video_ids with cited current metrics, metric,
         direction, and evaluate_after typically 7-14 days out.

5. Persist new hypotheses by running go run . insights new __THIS_MONDAY__
   to create the scaffold (it may already exist from the cloud routine;
   that is fine), then edit data/insights/__THIS_MONDAY__.md directly to
   append hypotheses with proper YAML frontmatter.

6. Commit anything you changed (graded insights/*.md plus the new
   insights file) with message: Weekly review __RUN_DATE__: graded N,
   proposed M. Push to origin/main. The pre-commit hook runs go vet
   plus go test.

Reference docs:
  skill/yt-analytics/SKILL.md  for workflow conventions
  docs/primitive-test.md       for Go=transport, prompt=cognition
  data/cohorts.yaml            for cohort definitions

Do not invent metrics. Quote the actual numbers from CLI output. If no
hypotheses are pending, say so explicitly and propose new ones based on
cohort movement alone.

Save the report to __REPORT_FILE__ before you finish.
PROMPT_EOF

# In-place placeholder substitution.
sed -i '' \
    -e "s|__RUN_DATE__|$RUN_DATE|g" \
    -e "s|__THIS_MONDAY__|$THIS_MONDAY|g" \
    -e "s|__LAST_MONDAY__|$LAST_MONDAY|g" \
    -e "s|__LAST_SUNDAY__|$LAST_SUNDAY|g" \
    -e "s|__PREV_MONDAY__|$PREV_MONDAY|g" \
    -e "s|__PREV_SUNDAY__|$PREV_SUNDAY|g" \
    -e "s|__REPORT_FILE__|$REPORT_FILE|g" \
    "$PROMPT_FILE"

PROMPT="$(cat "$PROMPT_FILE")"

# --- run claude headlessly. --print returns the final assistant text on stdout.
# Append both Claude's output and any tool errors to the log.
{
  if claude -p --permission-mode bypassPermissions "$PROMPT" 2>&1; then
    EXIT_CODE=0
  else
    EXIT_CODE=$?
  fi
  echo
  echo "--- claude exit code: $EXIT_CODE"
  echo "=== run finished $(date '+%Y-%m-%d %H:%M:%S %Z') ==="
} >> "$LOG_FILE" 2>&1

# --- macOS notification
if command -v osascript >/dev/null; then
  if [ -f "$REPORT_FILE" ]; then
    MSG="Report at $REPORT_FILE"
  else
    MSG="Run finished — check log $LOG_FILE"
  fi
  osascript -e "display notification \"$MSG\" with title \"YT Weekly Review\" sound name \"Glass\"" || true
fi

exit ${EXIT_CODE:-0}
