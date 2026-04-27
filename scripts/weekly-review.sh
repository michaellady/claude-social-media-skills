#!/usr/bin/env bash
# Sunday closed-loop weekly review for claude-social-media-skills.
#
# What this does:
#   1. Pulls latest main
#   2. Invokes `claude -p` headlessly with a prompt that drives the full
#      closed-loop weekly review:
#        - /buffer-stats (engagement attribution + auto recommendations)
#        - /linkedin-stats (per-post deltas, newsletter subs, follower deltas)
#        - /audit-buffer-queue (queue hygiene — bunching, untagged, dead channels)
#        - /flywheel (cross-platform priorities-keyed rollup)
#      and writes a single weekly report to data/reviews/<date>.md
#   3. Pushes any file changes Claude made (e.g., new snapshot caches in
#      buffer-stats/cache/, linkedin-stats/cache/, accepted skill-config
#      recommendations as committed SKILL.md edits)
#   4. Posts a macOS notification when finished
#
# Triggered weekly by ~/Library/LaunchAgents/com.mikelady.csms-weekly-review.plist
# (install with `make schedule-install`).
#
# Manual run: `bash scripts/weekly-review.sh` from the repo root.
#
# Browser-auth note: /buffer-stats and /linkedin-stats use gstack browse with
# imported cookies. If the cookie cache is stale or missing, those skills will
# either skip the engagement scrape (operational-only path) or fail and log
# the auth issue. The cron does NOT require interactive login — if cookies
# are stale, the partial report still ships and surfaces the gap.

set -euo pipefail

# --- locate the repo (script lives in scripts/, repo is one level up)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_DIR"

# --- log destination
LOG_DIR="$HOME/Library/Logs/csms-weekly-review"
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
  echo "=== csms-weekly-review run $(date '+%Y-%m-%d %H:%M:%S %Z') ==="
  echo "Repo: $REPO_DIR"
  echo "This week:  $THIS_MONDAY .. (today)"
  echo "Last week:  $LAST_MONDAY .. $LAST_SUNDAY"
  echo "Prev week:  $PREV_MONDAY .. $PREV_SUNDAY"
  echo "Report:     $REPORT_FILE"
  echo

  # --- pull latest
  echo "--- git pull"
  git pull --ff-only origin main || echo "(pull skipped/failed; continuing with local state)"

  # --- ensure shared Go binaries are built (no-op if already built)
  echo "--- build _shared/ helpers"
  for d in _shared/buffer-post-prep _shared/buffer-queue-check _shared/voice-corpus; do
    if [ -d "$d" ]; then (cd "$d" && go build .) || echo "($d build failed; continuing)"; fi
  done

  echo
  echo "--- invoking claude -p for the weekly review ---"
} >> "$LOG_FILE" 2>&1

# --- build the prompt for claude -p.
# Write the template to a tmpfile (heredoc-to-file avoids the
# command-substitution parsing edge case). Substitute placeholders in place.
PROMPT_FILE="$(mktemp -t csms-weekly-review.XXXXXX)"
trap 'rm -f "$PROMPT_FILE"' EXIT

cat > "$PROMPT_FILE" <<'PROMPT_EOF'
Run the claude-social-media-skills closed-loop weekly review.
Today is __RUN_DATE__.

Your job is the full weekly cognition + analysis step. The goal is a
single weekly report at __REPORT_FILE__ that closes the loop:
measure engagement → surface what worked → recommend skill changes →
flag queue hygiene issues. Write a report a human can scan in ~2 minutes.

Steps (in order — do NOT parallelize; later phases depend on earlier
data being on disk):

1. Run /buffer-stats with default args.
   - This scrapes Buffer Insights + Analyze via gstack browse and writes
     a snapshot under buffer-stats/cache/.
   - If gstack browse fails the auth check (cookie expired), fall back
     to /buffer-stats operational which uses Buffer MCP only and skips
     the browser scrape. Note the degradation in the report.
   - Capture the auto-generated skill recommendations from Phase 5b for
     inclusion in the weekly report.

2. Run /linkedin-stats with default args.
   - Scrapes LinkedIn /dashboard/ + /analytics/creator/* via gstack
     browse and caches under linkedin-stats/cache/.
   - Same fallback pattern: if auth fails, run /linkedin-stats newsletter
     for the fast path.

3. Run /audit-buffer-queue.
   - Surfaces queue health: bunching (gap < 3h), theme over-saturation,
     untagged posts, dead channels, below-threshold channels.
   - Capture findings + recommended actions for the weekly report.
   - Do NOT auto-apply destructive actions (cancel/reschedule/delete)
     in the cron — surface them for the user to review on Monday.

4. Run /flywheel.
   - Cross-platform priorities-keyed rollup using buffer-stats +
     linkedin-stats + YouTube + beehiiv data.
   - This is the synthesis step.

5. Write __REPORT_FILE__ with this structure:

   # CSMS weekly review — __RUN_DATE__
   Last week: __LAST_MONDAY__ .. __LAST_SUNDAY__
   Prev week: __PREV_MONDAY__ .. __PREV_SUNDAY__

   ## TL;DR
   3-5 bullets. What moved the needle this week, what didn't, and
   the single most important action for next week.

   ## Format performance (from /buffer-stats)
   Per-(channel, format) engagement table for last week. Mark which
   formats are above their channel's average and which are below.

   ## Skill-config recommendations (from /buffer-stats Phase 5b)
   Quote each recommendation verbatim. For each, state: the data
   that supports it, and whether to ACCEPT (commit a SKILL.md edit
   now) or DEFER (need more data / human judgment).

   ## LinkedIn deltas (from /linkedin-stats)
   Followers, newsletter subs, top posts WoW.

   ## Queue hygiene (from /audit-buffer-queue)
   Findings count + the top 3 actions the user should take Monday
   (cancel which posts, reschedule which posts, tag which posts).
   Do NOT auto-apply.

   ## Flywheel rollup (from /flywheel)
   Per-priority status. Per-channel ROI. Deprioritization candidates.

   ## What I changed this run
   - List any committed file changes (skill-config edits accepted from
     recommendations, snapshot caches updated, etc.)
   - Or "no changes — surfaced N actions for Monday review"

6. If you accepted recommendations that require SKILL.md edits, make
   them now and commit (git add affected SKILL.md files + this report
   + any new snapshot caches under */cache/). Commit message format:
   "Weekly review __RUN_DATE__: accepted N recs, surfaced M actions".
   Push to origin/main.

7. If no SKILL.md edits were warranted but the report + cache snapshots
   exist, still commit + push the snapshots (so cross-week deltas work
   next time).

Reference docs:
  ARCHITECTURE.md            for the closed-loop design
  PATTERNS.md                for cross-skill cognition patterns
  PRIMITIVE-TEST.md          for transport-vs-cognition framework

Important constraints:
- Do NOT auto-apply queue hygiene destructive actions (delete/reschedule
  Buffer posts). The cron is read-mostly. Surface the actions; human
  approves on Monday.
- Do NOT post anything to social media. /promote-* skills are out of
  scope for the cron.
- If a skill's required browser auth is broken, surface the gap in the
  report's TL;DR and continue with the rest. Don't fail the whole run
  on one skill.
- Save the report to __REPORT_FILE__ before you finish, even if some
  sections are degraded.

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
  osascript -e "display notification \"$MSG\" with title \"CSMS Weekly Review\" sound name \"Glass\"" || true
fi

exit ${EXIT_CODE:-0}
