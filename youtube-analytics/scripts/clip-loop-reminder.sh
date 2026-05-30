#!/usr/bin/env bash
# Daily reminder to run the FULL cross-platform clip closed loop INTERACTIVELY.
#
# Why this only reminds (doesn't measure): the cross-platform half of the loop
# (Facebook / Instagram / TikTok / LinkedIn) must read each platform's native
# analytics through a logged-in browser via claude-in-chrome — which cannot run
# headless/unattended (cf. the studio-watch routine, blocked since 2026-05-18).
# So this job just pings you; you run `/clip-results` with Claude while at the
# machine, and Claude drives the browser sweep + the Go aggregator.
#
# Fires daily at 9:30 AM local via
#   ~/Library/LaunchAgents/com.mikelady.yt-clip-loop-reminder.plist
#   (install: make clip-loop-reminder-install)
# Uninstall once the ~2-3 week measurement window closes:
#   make clip-loop-reminder-uninstall
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_DIR" || exit 0

LOG_DIR="$HOME/Library/Logs/yt-clip-loop-reminder"
mkdir -p "$LOG_DIR"
TS="$(date '+%Y-%m-%d %H:%M:%S %Z')"

PROJECTS="P3053020lhb4 P3053020lij1 P3053020ljFQ"
MSG="Run /opus-clips-performance + /flywheel on the batch ($PROJECTS) with Claude — the cross-platform sweep needs your logged-in browser."

# macOS notification (LaunchAgents run in the user GUI session, so this shows).
# Never fail the job if notifications are unavailable.
osascript -e "display notification \"$MSG\" with title \"📊 Clip closed loop\" sound name \"Glass\"" 2>/dev/null || true

echo "$TS  reminder fired" >> "$LOG_DIR/reminder.log"
