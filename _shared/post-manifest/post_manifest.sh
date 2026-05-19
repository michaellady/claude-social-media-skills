#!/bin/bash
# post-manifest helpers — thin jq wrappers encoding the closed-loop manifest contract.
# Source from a scheduling skill:  source _shared/post-manifest/post_manifest.sh
# Full contract: see README.md in this directory.
#
# All helpers operate on a manifest file path passed as $1. Mutations write
# atomically (tmp + mv). Errors return non-zero and print to stderr.

# Internal: atomic write — preserve original on failure
_pm_atomic_write() {
  local file="$1"
  local content
  content=$(cat)
  printf '%s' "$content" > "$file.tmp" && mv "$file.tmp" "$file"
}

# pm_init <manifest_path> [--project ID] [--source-video ID] [--source-title TITLE] [--source-url URL] [--force]
# Idempotent: if manifest exists, no-op unless --force.
pm_init() {
  local manifest="$1"; shift
  local project="" svid="" stitle="" surl="" force=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --project)       project="$2"; shift 2 ;;
      --source-video)  svid="$2"; shift 2 ;;
      --source-title)  stitle="$2"; shift 2 ;;
      --source-url)    surl="$2"; shift 2 ;;
      --force)         force="1"; shift ;;
      *) echo "pm_init: unknown flag '$1'" >&2; return 2 ;;
    esac
  done
  [[ -n "$manifest" ]] || { echo "pm_init: manifest path required" >&2; return 2; }

  if [[ -f "$manifest" && -z "$force" ]]; then
    return 0
  fi

  mkdir -p "$(dirname "$manifest")"
  jq -n \
    --arg pid "$project" \
    --arg svid "$svid" \
    --arg stitle "$stitle" \
    --arg surl "$surl" \
    --arg created "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    '{
      project_id: ($pid | select(. != "") // null),
      source_video: (
        if $svid != "" or $stitle != "" or $surl != "" then
          {
            id: ($svid | select(. != "") // null),
            title: ($stitle | select(. != "") // null),
            url: ($surl | select(. != "") // null)
          }
        else null end
      ),
      created_at: $created,
      clips: []
    } | del(.. | nulls? | select(. == null))' > "$manifest"
}

# pm_ensure_clip <manifest_path> --clip-id ID [--title T] [--description D] [--score N] [--duration-sec N] [--theme S]
# Idempotent: if clip_id already exists, no-op. Otherwise append a new entry with scheduled_posts=[].
pm_ensure_clip() {
  local manifest="$1"; shift
  local cid="" title="" desc="" score="null" dur="null" theme=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --clip-id)       cid="$2"; shift 2 ;;
      --title)         title="$2"; shift 2 ;;
      --description)   desc="$2"; shift 2 ;;
      --score)         score="$2"; shift 2 ;;
      --duration-sec)  dur="$2"; shift 2 ;;
      --theme)         theme="$2"; shift 2 ;;
      *) echo "pm_ensure_clip: unknown flag '$1'" >&2; return 2 ;;
    esac
  done
  [[ -n "$manifest" && -f "$manifest" ]] || { echo "pm_ensure_clip: manifest not found: $manifest" >&2; return 2; }
  [[ -n "$cid" ]] || { echo "pm_ensure_clip: --clip-id required" >&2; return 2; }

  local has
  has=$(jq --arg cid "$cid" '[.clips[] | select(.clip_id == $cid)] | length' "$manifest")
  if [[ "$has" -gt 0 ]]; then
    return 0
  fi

  jq --arg cid "$cid" --arg t "$title" --arg d "$desc" --argjson s "$score" --argjson dur "$dur" --arg theme "$theme" \
    '.clips += [{
      clip_id: $cid,
      title: $t,
      description: $d,
      score: $s,
      duration_sec: $dur,
      theme: ($theme | select(. != "") // null),
      scheduled_posts: []
    } | del(.. | nulls? | select(. == null))]' \
    "$manifest" | _pm_atomic_write "$manifest"
}

# pm_append_post <manifest_path> --clip-id ID --label L --account-id A [--sub-account-id S] --scheduled-at-utc T --api-response RAW_JSON
# Appends one (clip × channel) schedule record under the matching clip entry.
pm_append_post() {
  local manifest="$1"; shift
  local cid="" label="" aid="" sub="" at="" resp=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --clip-id)            cid="$2"; shift 2 ;;
      --label)              label="$2"; shift 2 ;;
      --account-id)         aid="$2"; shift 2 ;;
      --sub-account-id)     sub="$2"; shift 2 ;;
      --scheduled-at-utc)   at="$2"; shift 2 ;;
      --api-response)       resp="$2"; shift 2 ;;
      *) echo "pm_append_post: unknown flag '$1'" >&2; return 2 ;;
    esac
  done
  [[ -n "$manifest" && -f "$manifest" ]] || { echo "pm_append_post: manifest not found: $manifest" >&2; return 2; }
  [[ -n "$cid" && -n "$label" && -n "$at" ]] || { echo "pm_append_post: --clip-id, --label, --scheduled-at-utc all required" >&2; return 2; }

  # Validate api-response is parseable JSON; if not, wrap it as a raw string
  local resp_json
  if printf '%s' "$resp" | jq -e . >/dev/null 2>&1; then
    resp_json="$resp"
  else
    resp_json=$(jq -n --arg raw "$resp" '{raw: $raw}')
  fi

  local post_json
  post_json=$(jq -nc --arg label "$label" --arg aid "$aid" --arg sub "$sub" --arg at "$at" --argjson r "$resp_json" '{
    label: $label,
    account_id: ($aid | select(. != "") // null),
    sub_account_id: ($sub | select(. != "") // null),
    scheduled_at_utc: $at,
    api_response: $r
  } | del(.. | nulls? | select(. == null))')

  jq --arg cid "$cid" --argjson post "$post_json" \
    '.clips |= map(if .clip_id == $cid then .scheduled_posts += [$post] else . end)' \
    "$manifest" | _pm_atomic_write "$manifest"
}

# pm_count_scheduled <manifest_path>
# Print total (clip × channel) schedule entries across all clips.
pm_count_scheduled() {
  local manifest="$1"
  [[ -n "$manifest" && -f "$manifest" ]] || { echo "0"; return 0; }
  jq '[.clips[]?.scheduled_posts | length] | add // 0' "$manifest"
}

# pm_list_by_channel <manifest_path> <label_substring>
# Print all scheduled posts whose .label contains the substring.
pm_list_by_channel() {
  local manifest="$1"
  local needle="$2"
  jq --arg n "$needle" '
    [.clips[]? as $c | $c.scheduled_posts[]? | select(.label | test($n; "i")) |
      . + {clip_id: $c.clip_id, clip_title: $c.title}]
  ' "$manifest"
}

# pm_find_clip <manifest_path> --clip-id ID
# Print the full entry for one clip.
pm_find_clip() {
  local manifest="$1"; shift
  local cid=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --clip-id) cid="$2"; shift 2 ;;
      *) echo "pm_find_clip: unknown flag '$1'" >&2; return 2 ;;
    esac
  done
  jq --arg cid "$cid" '.clips[] | select(.clip_id == $cid)' "$manifest"
}

# pm_conflicts <manifest_path>
# Print every scheduled post where api_response.data.hasConflict == true.
# (Matches OpusClip's flag shape; harmless on other schedulers — just returns empty.)
pm_conflicts() {
  local manifest="$1"
  jq '[.clips[]? as $c | $c.scheduled_posts[]? | select(.api_response.data.hasConflict == true) |
    . + {clip_id: $c.clip_id}]' "$manifest"
}

# pm_schedule_ids <manifest_path>
# Print every scheduleId. Useful for batch-cancel scripts: while-read | xargs cancel.
pm_schedule_ids() {
  local manifest="$1"
  jq -r '.clips[]?.scheduled_posts[]?.api_response.data.scheduleId // empty' "$manifest"
}
