#!/usr/bin/env bash
# Render a filled EVC slide HTML → 1080x1350 PNG via headless Chrome.
#
# Usage:
#   render.sh <input.html> <output.png>
#
# The input HTML must include the marker <!--SVG_SPRITE--> somewhere in <body>;
# this script replaces that marker with the contents of illustrations.svg (inlined,
# display:none) before rendering. This is necessary because Chrome's headless
# `file://` loader won't resolve cross-document <use href="illustrations.svg#id">
# references reliably.

set -euo pipefail

IN="${1:?usage: render.sh <input.html> <output.png>}"
OUT="${2:?usage: render.sh <input.html> <output.png>}"

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
SPRITE="${SCRIPT_DIR}/illustrations.svg"
CHROME="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"

[[ -f "$IN" ]] || { echo "input not found: $IN" >&2; exit 1; }
[[ -f "$SPRITE" ]] || { echo "sprite not found: $SPRITE" >&2; exit 1; }
[[ -x "$CHROME" ]] || { echo "Chrome not found at: $CHROME" >&2; exit 1; }

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

# Copy the CSS alongside so relative link resolves
cp "${SCRIPT_DIR}/shared.css" "${WORK}/shared.css"

# Inline the sprite in place of the marker (using Python for reliable multi-line handling).
# Falls back to injecting right after <body> if marker is absent.
TMP_HTML="${WORK}/$(basename "$IN")"
SPRITE="$SPRITE" IN="$IN" OUT="$TMP_HTML" python3 <<'PY'
import os, re
with open(os.environ['SPRITE'], 'r') as f:
    sprite = f.read()
with open(os.environ['IN'], 'r') as f:
    html = f.read()
if '<!--SVG_SPRITE-->' in html:
    html = html.replace('<!--SVG_SPRITE-->', sprite, 1)
else:
    html = re.sub(r'(<body[^>]*>)', r'\1\n' + sprite, html, count=1)
with open(os.environ['OUT'], 'w') as f:
    f.write(html)
PY

# Chrome's headless viewport is ~87px shorter than --window-size (reserved for
# window chrome). Over-size the window by 100px so the 1350px body fits, then
# crop the resulting screenshot to exactly 1080x1350 from the top-left.
RAW="${WORK}/raw.png"
"$CHROME" \
  --headless=new \
  --disable-gpu \
  --hide-scrollbars \
  --force-device-scale-factor=1 \
  --window-size=1080,1450 \
  --default-background-color=00000000 \
  --screenshot="$RAW" \
  "file://${TMP_HTML}" \
  >/dev/null 2>&1

[[ -f "$RAW" ]] || { echo "render failed: no output produced" >&2; exit 2; }

# Crop to 1080x1350 top-left using sips (native macOS, no deps)
sips --cropToHeightWidth 1350 1080 "$RAW" --out "$OUT" >/dev/null 2>&1

[[ -f "$OUT" ]] || { echo "render failed: crop step failed" >&2; exit 3; }

DIM="$(file "$OUT" | grep -oE '[0-9]+ x [0-9]+' | head -1)"
if [[ "$DIM" != "1080 x 1350" ]]; then
  echo "warning: expected 1080 x 1350, got $DIM" >&2
fi

echo "$OUT ($DIM)"
