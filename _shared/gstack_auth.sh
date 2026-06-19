#!/usr/bin/env bash
# Verify gstack browse is logged into a platform; attempt cookie import once if not.
# Returns:
#   exit 0 — logged in (caller proceeds)
#   exit 1 — not logged in after cookie import; caller decides whether to handoff or skip
#   exit 2 — bad usage
#
# Usage: gstack_auth.sh <domain> <login-check-url>
# Examples:
#   gstack_auth.sh linkedin.com https://www.linkedin.com/feed/
#   gstack_auth.sh reddit.com    https://www.reddit.com/
#   gstack_auth.sh buffer.com    https://analyze.buffer.com
#
# Pure transport. The HANDOFF decision is cognition — caller's prompt decides
# whether to $B handoff (interactive) or skip the platform (non-interactive).

set -euo pipefail

if [ "$#" -ne 2 ]; then
  echo "usage: $0 <domain> <login-check-url>" >&2
  exit 2
fi

DOMAIN="$1"
URL="$2"
B="${B:-$HOME/.claude/skills/gstack/browse/dist/browse}"

if [ ! -x "$B" ]; then
  echo "gstack browse not found at $B" >&2
  exit 2
fi

# Self-heal a wedged gstack session BEFORE anything else. The bun server can keep
# running but lose its Chrome pipe, so every page command returns "No active page"
# and can't recover itself (confirmed 2026-06-19). gstack-recover hard-restarts it.
RECOVER_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/gstack-recover"
if [ ! -x "$RECOVER_DIR/gstack-recover" ] && command -v go >/dev/null 2>&1; then
  ( cd "$RECOVER_DIR" && go build -o gstack-recover . ) >/dev/null 2>&1 || true
fi
[ -x "$RECOVER_DIR/gstack-recover" ] && B="$B" "$RECOVER_DIR/gstack-recover" --probe-url "$URL" >&2 || true

# Reddit needs a UA spoof set proactively (otherwise 403 on first nav)
if [[ "$DOMAIN" == "reddit.com" ]]; then
  "$B" useragent "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36" >/dev/null 2>&1
fi

is_logged_in() {
  "$B" goto "$URL" >/dev/null 2>&1
  sleep 3
  # Generic check: if the URL path starts with /login, /sign-in, /uas/login, or
  # contains session_redirect=, we're not logged in.
  # Use anchored path matching to avoid false positives on URLs like ?ref=login_modal.
  CURRENT_URL=$("$B" url 2>/dev/null | head -1)
  if echo "$CURRENT_URL" | grep -qE '://[^/]+/(login|sign-in|uas/login)([/?#]|$)|[?&]session_redirect='; then
    return 1
  fi
  return 0
}

# First check
if is_logged_in; then
  exit 0
fi

# Direct, NON-INTERACTIVE cookie import from Chrome's store. Use the DOT-PREFIXED
# domain: Buffer/LinkedIn store their session cookie under ".buffer.com" /
# ".linkedin.com" (cross-subdomain), so the bare domain imports 0 — the silent
# auth wall hit on 2026-06-19. `--domain` reads the cookie DB directly (no picker,
# no handoff), so we try the dot-prefixed, bare, and www forms and sum the count.
imported=0
for d in ".$DOMAIN" "$DOMAIN" "www.$DOMAIN"; do
  # Each step `|| true` so a no-match grep can't trip set -e/pipefail (some domain
  # variants print no "Imported N" line at all).
  out=$("$B" cookie-import-browser chrome --domain "$d" 2>&1 || true)
  n=$(printf '%s' "$out" | grep -oE 'Imported [0-9]+' | grep -oE '[0-9]+' | head -1 || true)
  imported=$((imported + ${n:-0}))
done
echo "gstack_auth: imported $imported cookies for $DOMAIN (dot-prefixed direct read)" >&2

# Re-check auth
if is_logged_in; then
  exit 0
fi

# Still not logged in — caller decides what to do
exit 1
