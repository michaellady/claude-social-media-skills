#!/usr/bin/env bash
# Print the canonical "Comment newsletter" CTA for a beehiiv article title.
# This string is the trigger word for the Manychat / Comment-to-DM automation —
# the EXACT text matters; do not edit ad-hoc.
#
# Usage: cta.sh "<article title>"
# Output: Comment "newsletter" to get my latest post, "<title>"
#
# Pure transport. No cognition. Used by promote-newsletter, tease-newsletter,
# carousel-newsletter, crosspost-newsletter LinkedIn pulse accompanying post.

set -euo pipefail

if [ "$#" -ne 1 ] || [ -z "$1" ]; then
  echo "usage: $0 \"<article title>\"" >&2
  exit 64  # EX_USAGE
fi

printf 'Comment "newsletter" to get my latest post, "%s"' "$1"
