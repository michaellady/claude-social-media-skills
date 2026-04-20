#!/usr/bin/env bash
# wait_for_clips.sh — emit the processing-wait runbook for a given project.
#
# Opus generates clips asynchronously; the clip library shows "Original clips (N)"
# when ready. The skill polls every 5 min up to 2 hr. This script prints the
# polling JS + cadence; the skill runs the actual poll loop via
# mcp__claude-in-chrome__javascript_tool.
#
# Usage:
#   ./wait_for_clips.sh P3041416kZFt
#   ./wait_for_clips.sh P3041416kZFt --dry-run
#   ./wait_for_clips.sh --help

set -euo pipefail

print_help() {
  cat <<EOF
Usage: wait_for_clips.sh PROJECT_ID [--dry-run]

Emits the poll runbook for the skill to execute via Claude-in-Chrome MCP.

Arguments:
  PROJECT_ID    Opus project ID (starts with "P")

Options:
  --dry-run     Emit the plan with a short 1-minute ceiling instead of 2 hours
  -h, --help    Show this help
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  print_help
  exit 0
fi

if [[ $# -lt 1 ]]; then
  echo "error: PROJECT_ID is required" >&2
  print_help
  exit 2
fi

PROJECT_ID="$1"
DRY_RUN="no"
MAX_SEC=7200
POLL_SEC=300
if [[ "${2:-}" == "--dry-run" ]]; then
  DRY_RUN="yes"
  MAX_SEC=60
  POLL_SEC=15
fi

HERE="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
SELECTORS="$HERE/selectors.json"
BASE_URL=$(python3 -c "import json; print(json.load(open('$SELECTORS'))['_base_url'])")
CLIP_COUNT_REGEX=$(python3 -c "import json; print(json.load(open('$SELECTORS'))['clip_library']['original_clips_count_text_regex'])")

cat <<EOF
{
  "dry_run": $( [[ "$DRY_RUN" == "yes" ]] && echo true || echo false ),
  "project_id": "$PROJECT_ID",
  "max_wait_sec": $MAX_SEC,
  "poll_interval_sec": $POLL_SEC,
  "steps": [
    {"step": 1, "action": "navigate", "url": "$BASE_URL/clip/$PROJECT_ID"},
    {"step": 2, "action": "poll_js",
     "code": "(() => { const m = document.body.innerText.match($CLIP_COUNT_REGEX); return m ? parseInt(m[1], 10) : 0; })()",
     "expected": ">= 1",
     "cadence_sec": $POLL_SEC,
     "ceiling_sec": $MAX_SEC,
     "note": "Polls the clip-count badge on the project page. Returns when >=1 clip exists. If ceiling hits, surface to user before continuing."
    },
    {"step": 3, "action": "log", "detail": "Record clip count + elapsed_sec to /tmp/opus-clips-$PROJECT_ID.log"}
  ]
}
EOF
