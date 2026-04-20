#!/usr/bin/env bash
# upload.sh — ingest runbook for Opus Clip.
#
# Phase C finding: local Upload button requires a real HID click to satisfy
# Chrome's file-chooser user-activation guard. Drive picker also requires real
# clicks inside docs.google.com's cross-origin iframe. Conclusion: every ingest
# path needs exactly ONE human gesture.
#
# The chosen path: pre-upload the video to the opus-clips-automation Drive
# folder via drive_upload.py (fully automated), then have the user click
# folder → file → Select in Opus's Drive picker (3 clicks, all real).
#
# This script emits the JSON plan for the skill runner.
#
# Usage:
#   ./upload.sh /path/to/stream.mp4
#   ./upload.sh /path/to/stream.mp4 --dry-run
#   ./upload.sh --help

set -euo pipefail

print_help() {
  cat <<EOF
Usage: upload.sh VIDEO_PATH [--dry-run]

Emits a 4-phase ingest runbook:
  1. Upload video to Drive folder 'opus-clips-automation' via drive_upload.py (automated).
  2. Navigate Opus dashboard + click 'Google Drive' button (automated).
  3. Hand off to user for 3 picker clicks: folder → file → Select.
  4. After Opus redirects to /workflow, set clip length preset to 30s-90s and click 'Get clips in 1 click'.

Arguments:
  VIDEO_PATH    Absolute path to a local video file

Options:
  --dry-run     Print the runbook without verifying file existence
  -h, --help    Show this help
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  print_help
  exit 0
fi

if [[ $# -lt 1 ]]; then
  echo "error: VIDEO_PATH is required" >&2
  print_help
  exit 2
fi

VIDEO_PATH="$1"
DRY_RUN="no"
if [[ "${2:-}" == "--dry-run" ]]; then
  DRY_RUN="yes"
fi

if [[ "$DRY_RUN" != "yes" && ! -f "$VIDEO_PATH" ]]; then
  echo "error: file not found: $VIDEO_PATH" >&2
  exit 3
fi

HERE="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
SELECTORS="$HERE/selectors.json"
CONFIG="$HERE/config.json"
BASE_URL=$(python3 -c "import json; print(json.load(open('$SELECTORS'))['_base_url'])")
DASH_URL=$(python3 -c "import json; print(json.load(open('$SELECTORS'))['dashboard']['url'])")
FOLDER_ID=$(python3 -c "import json; print(json.load(open('$CONFIG'))['drive_upload']['folder_id'])")
FOLDER_NAME=$(python3 -c "import json; print(json.load(open('$CONFIG'))['drive_upload']['folder_name'])")
CLIP_LENGTH=$(python3 -c "import json; print(json.load(open('$CONFIG'))['workflow_setup']['clip_length_preset'])")

VIDEO_BASENAME="$(basename "$VIDEO_PATH")"
VIDEO_JSON=$(python3 -c "import json,sys; print(json.dumps(sys.argv[1]))" "$VIDEO_PATH")
VIDEO_BASENAME_JSON=$(python3 -c "import json,sys; print(json.dumps(sys.argv[1]))" "$VIDEO_BASENAME")

cat <<EOF
{
  "dry_run": $( [[ "$DRY_RUN" == "yes" ]] && echo true || echo false ),
  "video_path": $VIDEO_JSON,
  "video_filename": $VIDEO_BASENAME_JSON,
  "clip_length_preset": "$CLIP_LENGTH",
  "steps": [
    {
      "step": 1, "action": "bash",
      "cmd": ["$HERE/drive_upload.py", $VIDEO_JSON],
      "detail": "Upload video to Drive folder '$FOLDER_NAME' via Python + OAuth client in gen-lang-client-0527845499. Captures resulting fileId."
    },
    {
      "step": 2, "action": "navigate",
      "url": "$DASH_URL",
      "detail": "Open Opus dashboard."
    },
    {
      "step": 3, "action": "js",
      "code": "(() => { const btn = Array.from(document.querySelectorAll('button')).find(b => b.textContent.trim() === 'Google Drive'); if (!btn) return 'not_found'; btn.click(); return 'clicked'; })()",
      "detail": "Click 'Google Drive' button to open Opus's in-page Drive picker."
    },
    {
      "step": 4, "action": "user_handoff",
      "folder_to_click": "$FOLDER_NAME",
      "file_to_click": $VIDEO_BASENAME_JSON,
      "then_click": "Select",
      "detail": "ASK USER to click these three items in the picker. Cannot be automated — Google Picker iframe rejects CDP-dispatched events.",
      "wait_for": {
        "poll_js": "(() => { const p = location.pathname; if (p === '/workflow') return 'on_workflow'; return null; })()",
        "timeout_sec": 120
      }
    },
    {
      "step": 5, "action": "js",
      "code": "(async () => { const start = Date.now(); while (Date.now() - start < 15000) { const btn = Array.from(document.querySelectorAll('button')).find(b => /^(30[\\\\s-]*(to|-)?\\\\s*90\\\\s*s|30-90s|30s-90s)\$/i.test(b.textContent.trim())); if (btn) { btn.click(); return 'selected_30_90'; } await new Promise(r => setTimeout(r, 400)); } return 'preset_not_found_manual_required'; })()",
      "detail": "Set Clip Length preset to 30s-90s on /workflow page. Fall back to manual if preset button text differs — capture actual button text if unfound."
    },
    {
      "step": 6, "action": "js",
      "code": "(() => { const btn = Array.from(document.querySelectorAll('button')).find(b => b.textContent.trim() === 'Get clips in 1 click'); if (!btn) return 'not_found'; btn.click(); return 'clicked'; })()",
      "detail": "Start processing. Opus will redirect to /dashboard?projectId=<P...>."
    },
    {
      "step": 7, "action": "js",
      "code": "(async () => { const start = Date.now(); while (Date.now() - start < 30000) { const pid = new URLSearchParams(location.search).get('projectId'); if (pid) return JSON.stringify({projectId: pid}); await new Promise(r => setTimeout(r, 1000)); } return 'no_redirect'; })()",
      "detail": "Extract projectId from /dashboard?projectId=<P...> redirect URL."
    },
    {
      "step": 8, "action": "log",
      "detail": "Write projectId + video filename + timestamp to /tmp/opus-clips-<projectId>.log for idempotency."
    }
  ]
}
EOF
