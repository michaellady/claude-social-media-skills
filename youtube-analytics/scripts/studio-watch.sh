#!/usr/bin/env bash
# Daily Studio-screenshot pass for a single video.
#
# Complements scripts/launch-watch.sh: launch-watch covers Data + Analytics
# API metrics; studio-watch covers what's ONLY in Studio's UI — realtime
# 48h views, Studio's labeled traffic surfaces (Suggested videos / Browse
# features / Notifications / Channel pages), "X more than usual" comparisons,
# audience-retention curve, and the A/B test verdict modal.
#
# Mechanism: invokes `claude -p` headlessly with a prompt that drives
# claude-in-chrome through the user's already-logged-in Chrome. Requires:
#   - Chrome running with the claude-in-chrome extension connected
#   - User logged into YouTube Studio in that Chrome
#
# Output: ~/Library/Logs/yt-studio-watch/<video>/<YYYY-MM-DD>/
#   - overview.png       Analytics → Overview tab screenshot
#   - ab-report.png      A/B test report screenshot (if Results ready)
#   - summary.md         Brief text summary Claude wrote from the screenshots
#
# Usage:
#   bash scripts/studio-watch.sh <video-id>
#
# Triggered daily at 9 AM local via:
#   ~/Library/LaunchAgents/com.mikelady.yt-studio-watch.plist
#   (install: make studio-watch-install VIDEO=<video-id>)
#
# After the launch window:
#   make studio-watch-uninstall

set -euo pipefail

VIDEO_ID="${1:-${YT_STUDIO_VIDEO_ID:-}}"
if [ -z "$VIDEO_ID" ]; then
    echo "Usage: $0 <video-id>   (or set YT_STUDIO_VIDEO_ID)" >&2
    exit 2
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_DIR"

RUN_DATE="$(date +%Y-%m-%d)"
RUN_TIME="$(date '+%Y-%m-%d %H:%M:%S %Z')"
OUT_DIR="$HOME/Library/Logs/yt-studio-watch/$VIDEO_ID/$RUN_DATE"
mkdir -p "$OUT_DIR"
LOG_FILE="$OUT_DIR/run.log"

PROMPT_FILE="$(mktemp -t yt-studio-watch.XXXXXX)"
trap 'rm -f "$PROMPT_FILE"' EXIT

cat > "$PROMPT_FILE" <<'PROMPT_EOF'
Daily Studio-screenshot check on YouTube video __VIDEO_ID__.
Today is __RUN_DATE__. Output directory: __OUT_DIR__

Drive Chrome via mcp__claude-in-chrome. Load the tools you need via
ToolSearch first (the chrome MCP tools are deferred).

Steps:

1. tabs_context_mcp (createIfEmpty: true) — get a tab ID. Reuse or create.

2. navigate to https://studio.youtube.com/video/__VIDEO_ID__/analytics/tab-overview/period-default
   wait 4 seconds.

3. If a TubeBuddy upgrade modal is overlaying the page, click its close X
   (top-right of modal). Then wait 1 second.

4. Take a screenshot with save_to_disk: true. Move/rename the saved file
   to __OUT_DIR__/overview.png (use the Bash tool, NOT a screenshot move
   tool that doesn't exist).

5. navigate to https://studio.youtube.com/video/__VIDEO_ID__/edit
   wait 3 seconds.

6. Look at the page text — is there an "A/B Testing titles: Results ready"
   indicator above the title? If YES:
     - Click "View test report"
     - Wait 3 seconds
     - Screenshot with save_to_disk: true; move to __OUT_DIR__/ab-report.png
     - Close the dialog (X in top-right of modal)
   If NO results ready (test still running, or not started):
     - Do NOT screenshot ab-report.png.
     - Note this in the summary.

7. Write __OUT_DIR__/summary.md with the following structure:

   # Studio watch __VIDEO_ID__ — __RUN_DATE__

   - **Views (since published):** <number from the headline "This video
     has gotten N views since it was published">
   - **Watch time:** <hours from the Watch time card>
   - **Subscribers (attributed):** <+N from the Subscribers card>
   - **Realtime (last 48h):** <number from the Realtime panel>
   - **Top traffic sources (Realtime):** <each source: pct>
   - **Top traffic sources (since published):** <each source: pct, views>
   - **Audience retention:** <"No data yet" OR briefly characterize the curve>
   - **A/B test status:** <"Results ready: <winner+shares>" OR "Running, started <date>" OR "No test">

   Pull all numbers from the screenshots you just took. Do NOT invent
   numbers. If you cannot read a number cleanly from the screenshot, write
   "(unreadable)" rather than guessing.

8. End with one sentence about what changed vs the prior day's summary at
   __OUT_DIR__/../__YESTERDAY__/summary.md if it exists.

When done, output a one-line confirmation to stdout: "studio-watch
__VIDEO_ID__ __RUN_DATE__: OK <views> views <subs> subs".

Do NOT commit, push, or modify any files outside __OUT_DIR__.
PROMPT_EOF

YESTERDAY="$(date -v-1d +%Y-%m-%d 2>/dev/null || date -d 'yesterday' +%Y-%m-%d)"

sed -i '' \
    -e "s|__VIDEO_ID__|$VIDEO_ID|g" \
    -e "s|__RUN_DATE__|$RUN_DATE|g" \
    -e "s|__OUT_DIR__|$OUT_DIR|g" \
    -e "s|__YESTERDAY__|$YESTERDAY|g" \
    "$PROMPT_FILE"

PROMPT="$(cat "$PROMPT_FILE")"

{
    echo "=== studio-watch for $VIDEO_ID at $RUN_TIME ==="
    echo "Output dir: $OUT_DIR"
    echo
    echo "--- invoking claude -p for Studio screenshot pass ---"
    if claude -p --permission-mode bypassPermissions "$PROMPT" 2>&1; then
        EXIT_CODE=0
    else
        EXIT_CODE=$?
    fi
    echo
    echo "--- claude exit code: $EXIT_CODE"
    echo "=== run finished $(date '+%Y-%m-%d %H:%M:%S %Z') ==="
} >> "$LOG_FILE" 2>&1

if command -v osascript >/dev/null; then
    if [ -f "$OUT_DIR/summary.md" ]; then
        HEADLINE="$(grep -E '^\- \*\*Views' "$OUT_DIR/summary.md" | head -1 | sed 's/.*: //')"
        MSG="Studio: $HEADLINE — see $OUT_DIR"
    else
        MSG="studio-watch failed — check $LOG_FILE (Chrome + MCP connected?)"
    fi
    osascript -e "display notification \"$MSG\" with title \"Studio watch: $VIDEO_ID\" sound name \"Pop\"" || true
fi

exit ${EXIT_CODE:-0}
