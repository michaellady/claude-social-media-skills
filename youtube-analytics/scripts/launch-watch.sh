#!/usr/bin/env bash
# Daily launch-window monitor for a single video. Lighter than the Sunday
# review — refreshes data and prints the video deep-dive into a dated log.
# No headless claude invocation, no commits. Just visibility during the
# first few days of a launch.
#
# Usage:
#   bash scripts/launch-watch.sh <video-id>
#
# Triggered daily at 9 AM local for ~5 days post-launch via:
#   ~/Library/LaunchAgents/com.mikelady.yt-launch-watch.plist
# (install: make launch-watch-install <id>=H9HR4_Qz-Z0)
#
# After the launch window the LaunchAgent should be uninstalled with
#   make launch-watch-uninstall

set -euo pipefail

VIDEO_ID="${1:-${YT_LAUNCH_VIDEO_ID:-}}"
if [ -z "$VIDEO_ID" ]; then
    echo "Usage: $0 <video-id>   (or set YT_LAUNCH_VIDEO_ID)" >&2
    exit 2
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_DIR"

LOG_DIR="$HOME/Library/Logs/yt-launch-watch/$VIDEO_ID"
mkdir -p "$LOG_DIR"
RUN_DATE="$(date +%Y-%m-%d-%H%M)"
LOG_FILE="$LOG_DIR/$RUN_DATE.log"

{
    echo "=== launch-watch for $VIDEO_ID at $(date '+%Y-%m-%d %H:%M:%S %Z') ==="
    echo "--- fetch (metadata + view counts)"
    go run . fetch
    echo
    echo "--- fetch-analytics --traffic-sources --sub-status"
    # Skip --daily for these short runs: rows balloon quickly and we mostly
    # care about WHERE traffic came from + SUB vs UNSUB split early on.
    go run . fetch-analytics --traffic-sources --sub-status
    echo
    echo "--- video deep-dive"
    go run . video "$VIDEO_ID"
    echo "=== run finished $(date '+%Y-%m-%d %H:%M:%S %Z') ==="
} >> "$LOG_FILE" 2>&1

if command -v osascript >/dev/null; then
    # Pull the headline numbers out of the log for the notification.
    SUMMARY="$(grep -E '^  Views:|^  Subs/1K|^  SUBSCRIBED:|^  UNSUBSCRIBED:|^  Retention:' "$LOG_FILE" | tr '\n' '; ' | head -c 200)"
    osascript -e "display notification \"$SUMMARY\" with title \"Launch watch: $VIDEO_ID\" sound name \"Pop\"" || true
fi
