#!/usr/bin/env bash
# Wrap youtube-transcript-api to fetch a video's transcript on stdout.
#
# Usage: bash scripts/generate-transcript.sh <video-id>
#
# Used by the /yt-ab skill to ground title candidates in actual content.
# Auto-generated YouTube captions ARE fetchable via this package (it
# scrapes the timedtext endpoint, no auth needed) — unlike the official
# captions.download API which only returns user-uploaded tracks.
#
# Install once:
#   pip3 install --user youtube-transcript-api
# (or pipx install youtube-transcript-api)

set -euo pipefail

if [ "$#" -lt 1 ]; then
    echo "Usage: $0 <video-id>" >&2
    exit 2
fi

VIDEO_ID="$1"

# Strip any URL noise — accept full URLs or bare IDs.
case "$VIDEO_ID" in
    *youtube.com/watch*v=*)
        VIDEO_ID="${VIDEO_ID##*v=}"
        VIDEO_ID="${VIDEO_ID%%&*}"
        ;;
    *youtu.be/*)
        VIDEO_ID="${VIDEO_ID##*youtu.be/}"
        VIDEO_ID="${VIDEO_ID%%[?&]*}"
        ;;
esac

# Prefer the CLI form (works whether the package was installed via pipx,
# pip --user, or a venv). Fall back to a clear install message if missing.
if command -v youtube_transcript_api >/dev/null; then
    if OUT="$(youtube_transcript_api --format text "$VIDEO_ID" 2>&1)"; then
        printf "%s\n" "$OUT"
        exit 0
    fi
    echo "ERROR: failed to fetch transcript for $VIDEO_ID" >&2
    echo "$OUT" >&2
    echo "" >&2
    echo "Common causes: video is unlisted/private, has captions disabled," >&2
    echo "or is region-restricted. Paste the transcript to a file and pass" >&2
    echo "--transcript <path> to /yt-ab instead." >&2
    exit 5
fi

echo "ERROR: youtube-transcript-api CLI not found on PATH." >&2
echo "" >&2
echo "Install (recommended — uses pipx for isolated venv):" >&2
echo "  pipx install youtube-transcript-api" >&2
echo "" >&2
echo "Or with pip in a venv. PEP 668 blocks system-wide pip install on macOS." >&2
echo "" >&2
echo "Alternative: pass --transcript <path> to /yt-ab with a manually-pasted transcript." >&2
exit 4
