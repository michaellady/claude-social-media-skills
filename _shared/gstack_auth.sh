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

# Try cookie import once
"$B" cookie-import-browser chrome "$DOMAIN" >/dev/null 2>&1 &
PICKER_PID=$!

# Cookie picker is interactive — write picker URL to stderr so caller can prompt user
echo "Cookie picker opened at http://127.0.0.1:11297/cookie-picker — select $DOMAIN, then close" >&2
echo "Waiting up to 60s for picker to close..." >&2

# Wait for picker process to end (user closes the picker tab)
for i in $(seq 1 60); do
  if ! kill -0 "$PICKER_PID" 2>/dev/null; then
    break
  fi
  sleep 1
done

# Reap the picker process so we don't leave a zombie if it exited normally.
# `wait` returns the picker's exit code but we don't use it — the cookie picker
# always exits 0 on close.
wait "$PICKER_PID" 2>/dev/null || true

# Re-check auth
if is_logged_in; then
  exit 0
fi

# Still not logged in — caller decides what to do
exit 1
