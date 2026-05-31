#!/usr/bin/env bash
# End-to-end smoke test for the voice-corpus binary.
#
# Pure-logic Go tests (`go test ./...`) cover loadLiveVideos filtering, the
# sort/limit helper, and the main.go classifier/date helpers — all hermetic,
# no network. They DO NOT exercise the live beehiiv RSS fetch nor the shell-out
# to generate-transcript.sh (which wraps youtube_transcript_api). This script does.
#
# Like _shared/adversarial-review/smoke.sh, it makes real live calls — it hits
# the beehiiv RSS feed over the network. That is acceptable (and intended) for a
# smoke test. It does NOT require any paid/provider tokens.
#
# What it asserts:
#   1. `voice-corpus --refresh --youtube` returns newsletters. If
#      youtube_transcript_api is installed, it ALSO asserts youtube_live posts
#      are present (both source_type values). If the package is absent, the
#      youtube assertion is SKIPPED (not failed) — graceful degradation.
#   2. With a deliberately broken youtube_transcript_script path (via a
#      throwaway config.local.json), newsletters still come back and the exit
#      code is 0 — livestream ingestion failure is never fatal.
#
# It backs up and restores cache.json and never leaves a config.local.json behind.
#
# Usage:
#   _shared/voice-corpus/smoke.sh
set -euo pipefail

dir=$(cd "$(dirname "$0")" && pwd)
bin="$dir/voice-corpus"
local_cfg="$dir/config.local.json"
cache="$dir/cache.json"
cache_bak=""

# --- cleanup: always restore cache, always remove any config.local.json we made ---
made_local_cfg=0
cleanup() {
  if [[ "$made_local_cfg" == "1" && -f "$local_cfg" ]]; then
    rm -f "$local_cfg"
  fi
  if [[ -n "$cache_bak" && -f "$cache_bak" ]]; then
    mv -f "$cache_bak" "$cache"
  fi
}
trap cleanup EXIT

# Guard: never clobber a pre-existing config.local.json (a user's real override).
if [[ -f "$local_cfg" ]]; then
  echo "error: $local_cfg already exists; refusing to overwrite a real local config." >&2
  echo "       Move it aside and re-run." >&2
  exit 2
fi

# Preserve the user's real cache; we restore it on exit.
if [[ -f "$cache" ]]; then
  cache_bak=$(mktemp -t voice-corpus-cache-bak)
  cp -f "$cache" "$cache_bak"
fi

echo "===== build ====="
( cd "$dir" && go build . )
echo "  OK: built $bin"

# jq makes the source_type assertions robust; fall back to grep if it's missing.
have_jq=0
command -v jq >/dev/null 2>&1 && have_jq=1

# Does the transcript dependency exist? Mirrors generate-transcript.sh's own probe.
have_yt_api=0
command -v youtube_transcript_api >/dev/null 2>&1 && have_yt_api=1

count_source() { # count_source <json> <source_type>
  if [[ "$have_jq" == "1" ]]; then
    printf '%s' "$1" | jq --arg s "$2" '[.posts[] | select(.source_type==$s)] | length'
  else
    printf '%s' "$1" | grep -o "\"source_type\": *\"$2\"" | wc -l | tr -d ' '
  fi
}

pass=0
fail=0
fail_msg() { echo "  FAIL: $1" >&2; fail=$((fail + 1)); }
pass_msg() { echo "  OK: $1"; pass=$((pass + 1)); }

# --- Test 1: live fetch, youtube forced on ----------------------------------
echo "===== smoke: --refresh --youtube (live beehiiv + transcript shell-out) ====="
if out=$("$bin" --refresh --youtube 2>/dev/null); then
  n_news=$(count_source "$out" "newsletter")
  n_yt=$(count_source "$out" "youtube_live")
  echo "  newsletters=$n_news  youtube_live=$n_yt"
  if [[ "${n_news:-0}" -gt 0 ]]; then
    pass_msg "newsletters present ($n_news)"
  else
    fail_msg "no newsletter posts returned"
  fi
  if [[ "$have_yt_api" == "1" ]]; then
    if [[ "${n_yt:-0}" -gt 0 ]]; then
      pass_msg "youtube_live present ($n_yt) — both source_type values found"
    else
      fail_msg "youtube_transcript_api is installed but no youtube_live posts returned"
    fi
  else
    echo "  SKIP: youtube_transcript_api not installed — livestream ingestion not asserted"
  fi
else
  fail_msg "binary exited non-zero on --refresh --youtube"
fi

# --- Test 2: broken transcript script path -> graceful degradation ----------
echo "===== smoke: broken youtube_transcript_script -> graceful degradation ====="
cat > "$local_cfg" <<'JSON'
{
  "youtube_transcript_script": "/nonexistent/generate-transcript.sh"
}
JSON
made_local_cfg=1

set +e
out=$("$bin" --refresh --youtube 2>/dev/null)
rc=$?
set -e

# Remove the throwaway override immediately (also covered by the EXIT trap).
rm -f "$local_cfg"
made_local_cfg=0

if [[ "$rc" -eq 0 ]]; then
  pass_msg "exit code 0 despite missing transcript script"
else
  fail_msg "exit code $rc (expected 0 — livestream failure must never be fatal)"
fi

n_news=$(count_source "$out" "newsletter")
if [[ "${n_news:-0}" -gt 0 ]]; then
  pass_msg "newsletters still returned ($n_news) with broken transcript script"
else
  fail_msg "no newsletter posts returned under graceful-degradation path"
fi

echo "---"
echo "smoke: $pass pass / $fail fail"
[[ $fail -eq 0 ]]
