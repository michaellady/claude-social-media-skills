#!/bin/bash
# content-attribution helpers — the JOIN engine for the unified closed-loop.
# Reads post-manifests + per-platform stats snapshots, emits per-source-content records.
# Source from a consumer skill:  source _shared/content-attribution/content_attribution.sh
# Full contract: see README.md in this directory.
#
# Read-only over snapshots. No mutations. Thin jq wrappers, same Primitive Test
# discipline as _shared/post-manifest/. Missing snapshots are emitted as
# {pending: true, pending_task: "#NNN"} — never as errors.

# ---------- Paths (overridable via env, sane defaults) ----------

: "${CA_YT_VIDEOS:=$HOME/dev/youtube_analytics/data/videos.json}"
: "${CA_OPUS_MANIFESTS_DIR:=$HOME/dev/youtube_analytics/data/opus_clips}"
: "${CA_MANIFESTS_ROOT:=$HOME/dev/youtube_analytics/data}"
: "${CA_BUFFER_CACHE_DIR:=$HOME/dev/claude-social-media-skills/buffer-stats/cache}"
: "${CA_LINKEDIN_CACHE_DIR:=$HOME/dev/claude-social-media-skills/linkedin-stats/cache}"
: "${CA_TIKTOK_CACHE_DIR:=$HOME/dev/claude-social-media-skills/tiktok-stats/cache}"
: "${CA_THREADS_CACHE_DIR:=$HOME/dev/claude-social-media-skills/threads-stats/cache}"

# Pending-task IDs for platforms not yet wired. Update when tasks land.
: "${CA_PENDING_TIKTOK:=#373}"
: "${CA_PENDING_THREADS:=#375}"
: "${CA_PENDING_LINKEDIN_PER_POST:=#370}"
: "${CA_PENDING_BUFFER_FORMAT:=#371}"

# ---------- Private helpers ----------

# Print the newest snapshot file in a directory (lexicographic on snapshot-YYYY-MM-DD.json), or empty.
_ca_newest_snapshot() {
  local dir="$1"
  [[ -d "$dir" ]] || return 0
  find "$dir" -maxdepth 1 -name 'snapshot-*.json' 2>/dev/null | sort | tail -1
}

# Print every post-manifest file. Today this is just opus_clips/; future schedulers
# will add their own directories under CA_MANIFESTS_ROOT (linkedin_pulses/, etc.).
# We filter to files with the manifest shape contract (.clips is an array) — anything
# else under the data root (videos.json, snapshots/, cohorts.yaml, …) is ignored.
_ca_all_manifests() {
  local candidates=()
  if [[ -d "$CA_OPUS_MANIFESTS_DIR" ]]; then
    while IFS= read -r f; do candidates+=("$f"); done < <(find "$CA_OPUS_MANIFESTS_DIR" -maxdepth 1 -name '*.json' 2>/dev/null)
  fi
  # Other scheduler dirs (linkedin_pulses, etc.) — add when they exist.
  local d
  for d in "$CA_MANIFESTS_ROOT"/linkedin_pulses "$CA_MANIFESTS_ROOT"/medium_posts "$CA_MANIFESTS_ROOT"/substack_posts; do
    [[ -d "$d" ]] || continue
    while IFS= read -r f; do candidates+=("$f"); done < <(find "$d" -maxdepth 1 -name '*.json' 2>/dev/null)
  done

  local f
  for f in "${candidates[@]}"; do
    # Shape probe — a real post-manifest has clips[] AND at least one clip carries a
    # scheduled_posts key (the publication-ledger contract from _shared/post-manifest).
    # This excludes compose-phase staging files (e.g. *-proposed-copy.json, *-transcripts.json)
    # that share the directory and also have a clips[] array but no scheduled_posts.
    jq -e 'type == "object" and (.clips | type) == "array" and ([.clips[]? | has("scheduled_posts")] | any)' "$f" >/dev/null 2>&1 && printf '%s\n' "$f"
  done
}

# Emit the canonical "pending" platform record.
_ca_pending() {
  local task="$1"
  jq -nc --arg t "$task" '{engagement: null, pending: true, pending_task: $t}'
}

# Emit the canonical "scheduled but not yet aired" record.
_ca_not_aired() {
  local at="$1"
  jq -nc --arg at "$at" '{engagement: null, scheduled_at_utc: $at, pending: true, reason: "not_yet_aired"}'
}

# Emit the canonical "no match found" record.
_ca_no_match() {
  jq -nc '{engagement: null, reason: "no_match"}'
}

# Map a post-manifest scheduled-post .label to a canonical platform key.
# Examples: "YOUTUBE Enterprise Vibe Code" -> "youtube_shorts",
#           "FACEBOOK_PAGE Enterprise Vibe Code" -> "facebook_page".
_ca_label_to_platform() {
  local label="$1"
  case "$label" in
    YOUTUBE*)             echo "youtube_shorts" ;;
    FACEBOOK_PAGE*)       echo "facebook_page" ;;
    INSTAGRAM_BUSINESS*)  echo "instagram_business" ;;
    "LINKEDIN Mike Lady"*) echo "linkedin_personal" ;;
    LINKEDIN*)            echo "linkedin_page" ;;
    TIKTOK_BUSINESS*)     echo "tiktok_business" ;;
    THREADS*)             echo "threads" ;;
    *)                    echo "unknown" ;;
  esac
}

# Look up a YouTube video by clip_id ([opus:<id>] tag), then by time-window fallback.
# $1 = clip_id, $2 = scheduled_at_utc, $3 = target_duration_seconds.
# Echoes a JSON object with the canonical youtube_shorts shape, or empty.
_ca_yt_match() {
  local clip_id="$1" sched_at="$2" target_dur="$3"
  [[ -f "$CA_YT_VIDEOS" ]] || return 0

  # Try tag match first.
  local hit
  hit=$(jq -c --arg cid "$clip_id" '
    (.videos // .items // .)
    | (if type == "array" then . else [] end)
    | map(select(.description? and (.description | test("\\[opus:" + $cid + "\\]"))))
    | .[0] // empty
  ' "$CA_YT_VIDEOS" 2>/dev/null)

  if [[ -n "$hit" && "$hit" != "null" ]]; then
    printf '%s' "$hit" | jq -c '{
      video_id: .id,
      views: (.view_count // 0),
      likes: (.like_count // 0),
      comments: (.comment_count // 0),
      subs_gained: (.subscribers_gained // 0),
      estimated_revenue: (.estimated_revenue // 0),
      join_method: "tag"
    }'
    return 0
  fi

  # Time-window fallback (±2h, tie-break on closest duration).
  [[ -n "$sched_at" ]] || return 0
  hit=$(jq -c --arg at "$sched_at" --argjson dur "${target_dur:-0}" '
    (.videos // .items // .)
    | (if type == "array" then . else [] end)
    | map(select((.video_type? // "") == "short"))
    | map(. + {
        delta_sec: (((.published_at // "1970-01-01T00:00:00Z") | fromdateiso8601) - ($at | fromdateiso8601) | fabs),
        dur_delta: (((.duration_seconds // 0) - $dur) | fabs)
      })
    | map(select(.delta_sec <= 7200))
    | sort_by(.dur_delta, .delta_sec)
    | .[0] // empty
  ' "$CA_YT_VIDEOS" 2>/dev/null)

  if [[ -n "$hit" && "$hit" != "null" ]]; then
    printf '%s' "$hit" | jq -c '{
      video_id: .id,
      views: (.view_count // 0),
      likes: (.like_count // 0),
      comments: (.comment_count // 0),
      subs_gained: (.subscribers_gained // 0),
      estimated_revenue: (.estimated_revenue // 0),
      join_method: "time"
    }'
  fi
}

# Look up a LinkedIn personal post by clip_id tag in profile.recent_posts[].
# $1 = clip_id. Echoes the canonical linkedin_personal record, or empty.
# Gated on #370 Phase 3b — emits pending if recent_posts missing.
_ca_li_personal_match() {
  local clip_id="$1"
  local snap
  snap=$(_ca_newest_snapshot "$CA_LINKEDIN_CACHE_DIR")
  if [[ -z "$snap" ]]; then
    _ca_pending "$CA_PENDING_LINKEDIN_PER_POST"
    return 0
  fi

  local has_posts
  has_posts=$(jq -r '(.profile.recent_posts // []) | length' "$snap" 2>/dev/null)
  if [[ -z "$has_posts" || "$has_posts" == "0" || "$has_posts" == "null" ]]; then
    _ca_pending "$CA_PENDING_LINKEDIN_PER_POST"
    return 0
  fi

  local hit
  hit=$(jq -c --arg cid "$clip_id" '
    [.profile.recent_posts[] | select((.body // .text // "") | test("\\[opus:" + $cid + "\\]"))]
    | .[0] // empty
  ' "$snap" 2>/dev/null)

  if [[ -n "$hit" && "$hit" != "null" ]]; then
    printf '%s' "$hit" | jq -c '{
      urn: (.urn // .id),
      reactions: (.reactions // 0),
      comments: (.comments // 0),
      reposts: (.reposts // 0),
      join_method: "tag"
    }'
  else
    _ca_no_match
  fi
}

# Generic Buffer-routed platform lookup by scheduleId. The buffer-stats snapshot
# doesn't yet carry per-post records keyed on scheduleId (gated on #371).
# When that lands, replace this stub with a real lookup. For now: pending.
_ca_buffer_match() {
  local platform_key="$1"  # e.g. instagram_business
  local schedule_id="$2"
  local snap
  snap=$(_ca_newest_snapshot "$CA_BUFFER_CACHE_DIR")
  if [[ -z "$snap" ]]; then
    _ca_pending "$CA_PENDING_BUFFER_FORMAT"
    return 0
  fi

  # Probe whether the snapshot has per-post records with scheduleId. (#371 will add these.)
  local has_perpost
  has_perpost=$(jq -r '[paths | select(.[-1] == "scheduleId")] | length' "$snap" 2>/dev/null)
  if [[ -z "$has_perpost" || "$has_perpost" == "0" ]]; then
    _ca_pending "$CA_PENDING_BUFFER_FORMAT"
    return 0
  fi

  # Real lookup (forward-compat).
  local hit
  hit=$(jq -c --arg sid "$schedule_id" --arg pk "$platform_key" '
    [.. | objects | select(.scheduleId? == $sid)] | .[0] // empty
  ' "$snap" 2>/dev/null)

  if [[ -n "$hit" && "$hit" != "null" ]]; then
    printf '%s' "$hit" | jq -c --arg pk "$platform_key" '. + {join_method: "schedule_id"}'
  else
    _ca_no_match
  fi
}

# Lookup a TikTok post by clip_id tag. Gated on #373.
_ca_tiktok_match() {
  local snap
  snap=$(_ca_newest_snapshot "$CA_TIKTOK_CACHE_DIR")
  if [[ -z "$snap" ]]; then
    _ca_pending "$CA_PENDING_TIKTOK"
    return 0
  fi
  _ca_pending "$CA_PENDING_TIKTOK"
}

# Lookup a Threads post by clip_id tag. Gated on #375.
_ca_threads_match() {
  local snap
  snap=$(_ca_newest_snapshot "$CA_THREADS_CACHE_DIR")
  if [[ -z "$snap" ]]; then
    _ca_pending "$CA_PENDING_THREADS"
    return 0
  fi
  _ca_pending "$CA_PENDING_THREADS"
}

# Dispatcher: given (platform_key, clip_id, scheduled_at, duration, schedule_id), produce a record.
_ca_platform_lookup() {
  local platform="$1" cid="$2" sched_at="$3" dur="$4" sid="$5"
  local result=""
  case "$platform" in
    youtube_shorts)
      result=$(_ca_yt_match "$cid" "$sched_at" "$dur")
      ;;
    linkedin_personal)
      result=$(_ca_li_personal_match "$cid")
      ;;
    facebook_page|instagram_business|linkedin_page)
      result=$(_ca_buffer_match "$platform" "$sid")
      ;;
    tiktok_business)
      result=$(_ca_tiktok_match)
      ;;
    threads)
      result=$(_ca_threads_match)
      ;;
    *)
      result=$(_ca_no_match)
      ;;
  esac

  # If the platform's scheduled time is still in the future and we got no_match, surface that.
  if [[ -n "$sched_at" && -n "$result" ]]; then
    local now epoch sched_epoch
    now=$(date -u +%Y-%m-%dT%H:%M:%SZ)
    epoch=$(printf '%s' "$now" | jq -R 'fromdateiso8601')
    sched_epoch=$(printf '%s' "$sched_at" | jq -R 'fromdateiso8601' 2>/dev/null)
    if [[ -n "$sched_epoch" && "$sched_epoch" != "null" ]] && (( sched_epoch > epoch )); then
      local reason
      reason=$(printf '%s' "$result" | jq -r '.reason // ""' 2>/dev/null)
      if [[ "$reason" == "no_match" ]]; then
        result=$(_ca_not_aired "$sched_at")
      fi
    fi
  fi

  printf '%s' "$result"
}

# ---------- Public API ----------

# ca_extract_tag <text>
# Pull the FIRST [scheme:id] tag from a text body. Echo {scheme, id} JSON or null.
ca_extract_tag() {
  local text="$*"
  printf '%s' "$text" | jq -Rs '
    (capture("\\[(?<scheme>opus|lp|gh|bh):(?<id>[^\\]]+)\\]") // null)
  '
}

# ca_find_source <source_id>
# Locate a source content by ID. Tries (in order):
#   1. YouTube videos.json by .id
#   2. Post-manifests by .source_video.id
# Echoes {type, id, title, url, published_at, duration_seconds, manifest_path?} or empty.
ca_find_source() {
  local sid="$1"
  [[ -n "$sid" ]] || return 0

  # 1) YouTube videos.json.
  if [[ -f "$CA_YT_VIDEOS" ]]; then
    local hit
    hit=$(jq -c --arg id "$sid" '
      (.videos // .items // .)
      | (if type == "array" then . else [] end)
      | map(select(.id == $id)) | .[0] // empty
    ' "$CA_YT_VIDEOS" 2>/dev/null)
    if [[ -n "$hit" && "$hit" != "null" ]]; then
      printf '%s' "$hit" | jq -c '{
        type: (if (.video_type // "") == "short" then "short" else "long_form" end),
        id: .id,
        title: .title,
        url: ("https://www.youtube.com/watch?v=" + .id),
        published_at: .published_at,
        duration_seconds: (.duration_seconds // null)
      }'
      return 0
    fi
  fi

  # 2) Walk manifests for matching .source_video.id.
  local m
  while IFS= read -r m; do
    [[ -z "$m" ]] && continue
    local svid
    svid=$(jq -r '.source_video.id // empty' "$m" 2>/dev/null)
    if [[ "$svid" == "$sid" ]]; then
      jq -c --arg path "$m" '{
        type: "long_form",
        id: .source_video.id,
        title: (.source_video.title // null),
        url: (.source_video.url // null),
        published_at: null,
        duration_seconds: null,
        manifest_path: $path
      }' "$m"
      return 0
    fi
  done < <(_ca_all_manifests)
}

# ca_join_engagement <source_id>
# The JOIN. Emit the unified per-source-content record.
ca_join_engagement() {
  # Accept either positional (`ca_join_engagement <source_id>`) or flag form
  # (`ca_join_engagement --source-id X [--source-type T] [--manifest P]`). The flag
  # form is what /flywheel Phase 4.56 calls; --source-type and --manifest are accepted
  # as hints but not required — the source is re-resolved from the snapshot universe.
  local sid="" _stype="" _manifest=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --source-id)   sid="$2"; shift 2 ;;
      --source-type) _stype="$2"; shift 2 ;;
      --manifest)    _manifest="$2"; shift 2 ;;
      --*)           shift ;;  # ignore unknown flags forward-compat
      *)             [[ -z "$sid" ]] && sid="$1"; shift ;;
    esac
  done
  [[ -n "$sid" ]] || { echo "ca_join_engagement: source_id required (positional or --source-id)" >&2; return 2; }

  # Resolve source.
  local source_json
  source_json=$(ca_find_source "$sid")
  if [[ -z "$source_json" ]]; then
    jq -nc --arg id "$sid" '{
      source: {id: $id, type: "unknown"},
      derivatives: [],
      source_engagement: null,
      derived_engagement: {reach: 0, reactions: 0, comments: 0, subs_gained: 0, estimated_revenue: 0},
      amplification_ratio: null,
      pending: true,
      reason: "source_not_found"
    }'
    return 0
  fi

  # Source engagement (YouTube-side, if we have it).
  local source_eng="null"
  if [[ -f "$CA_YT_VIDEOS" ]]; then
    source_eng=$(jq -c --arg id "$sid" '
      (.videos // .items // .)
      | (if type == "array" then . else [] end)
      | map(select(.id == $id)) | .[0]
      | if . then {
          views: (.view_count // 0),
          likes: (.like_count // 0),
          comments: (.comment_count // 0),
          subs_gained: (.subscribers_gained // 0),
          estimated_revenue: (.estimated_revenue // 0)
        } else null end
    ' "$CA_YT_VIDEOS" 2>/dev/null)
    [[ -z "$source_eng" ]] && source_eng="null"
  fi

  # Find manifests pointing at this source.
  local manifests=()
  local m
  while IFS= read -r m; do
    [[ -z "$m" ]] && continue
    local svid
    svid=$(jq -r '.source_video.id // empty' "$m" 2>/dev/null)
    [[ "$svid" == "$sid" ]] && manifests+=("$m")
  done < <(_ca_all_manifests)

  # Walk each manifest, each clip, each scheduled_posts entry; build derivative records.
  local derivatives_json="[]"
  for m in "${manifests[@]}"; do
    local n_clips
    n_clips=$(jq -r '.clips | length' "$m" 2>/dev/null)
    [[ -z "$n_clips" || "$n_clips" == "0" ]] && continue

    local i=0
    while (( i < n_clips )); do
      local clip
      clip=$(jq -c --argjson i "$i" '.clips[$i]' "$m")
      local cid title score dur
      cid=$(printf '%s' "$clip" | jq -r '.clip_id')
      title=$(printf '%s' "$clip" | jq -r '.title // ""')
      score=$(printf '%s' "$clip" | jq -r '.score // "null"')
      dur=$(printf '%s' "$clip" | jq -r '.duration_sec // 0')

      # Build platforms object by walking scheduled_posts.
      local platforms_json="{}"
      local n_posts
      n_posts=$(printf '%s' "$clip" | jq -r '.scheduled_posts | length')

      local j=0
      while (( j < n_posts )); do
        local sp
        sp=$(printf '%s' "$clip" | jq -c --argjson j "$j" '.scheduled_posts[$j]')
        local label sched_at sid_post
        label=$(printf '%s' "$sp" | jq -r '.label')
        sched_at=$(printf '%s' "$sp" | jq -r '.scheduled_at_utc // ""')
        sid_post=$(printf '%s' "$sp" | jq -r '.api_response.data.scheduleId // ""')
        local pkey
        pkey=$(_ca_label_to_platform "$label")

        local record
        record=$(_ca_platform_lookup "$pkey" "$cid" "$sched_at" "$dur" "$sid_post")
        [[ -z "$record" ]] && record=$(_ca_no_match)

        platforms_json=$(printf '%s' "$platforms_json" | jq -c --arg k "$pkey" --argjson v "$record" '. + {($k): $v}')
        j=$((j+1))
      done

      # Derivative total: sum live numeric metrics across platforms.
      local deriv_total
      deriv_total=$(printf '%s' "$platforms_json" | jq -c '
        [to_entries[].value]
        | map(select(.engagement != null or .views? or .impressions? or .reactions? or .likes?))
        | reduce .[] as $p ({reach: 0, reactions: 0, comments: 0};
            .reach     += (($p.views // 0) + ($p.impressions // 0)) |
            .reactions += (($p.likes // 0) + ($p.reactions // 0)) |
            .comments  += ($p.comments // 0)
          )
      ')

      local deriv_record
      deriv_record=$(jq -nc \
        --arg cid "$cid" \
        --arg title "$title" \
        --argjson score "${score:-null}" \
        --argjson dur "${dur:-0}" \
        --argjson platforms "$platforms_json" \
        --argjson total "$deriv_total" '{
          type: "opus_clip",
          clip_id: $cid,
          title: $title,
          score: $score,
          duration_seconds: $dur,
          platforms: $platforms,
          derivative_engagement_total: $total
        }')

      derivatives_json=$(printf '%s' "$derivatives_json" | jq -c --argjson d "$deriv_record" '. + [$d]')
      i=$((i+1))
    done
  done

  # Sum derived engagement across all derivatives.
  local derived
  derived=$(printf '%s' "$derivatives_json" | jq -c '
    reduce .[] as $d ({reach: 0, reactions: 0, comments: 0, subs_gained: 0, estimated_revenue: 0};
      .reach     += ($d.derivative_engagement_total.reach // 0) |
      .reactions += ($d.derivative_engagement_total.reactions // 0) |
      .comments  += ($d.derivative_engagement_total.comments // 0) |
      .subs_gained += ([$d.platforms | to_entries[].value.subs_gained // 0] | add // 0) |
      .estimated_revenue += ([$d.platforms | to_entries[].value.estimated_revenue // 0] | add // 0)
    )
  ')

  # Amplification ratio.
  local amp="null"
  local src_views
  src_views=$(printf '%s' "$source_eng" | jq -r '.views // 0' 2>/dev/null)
  local derived_reach
  derived_reach=$(printf '%s' "$derived" | jq -r '.reach // 0')
  if [[ -n "$src_views" && "$src_views" != "0" && "$src_views" != "null" ]]; then
    amp=$(jq -nc --argjson r "${derived_reach:-0}" --argjson v "${src_views:-1}" '($r / $v) | . * 10 | round / 10')
  fi

  jq -nc \
    --argjson source "$source_json" \
    --argjson derivatives "$derivatives_json" \
    --argjson src_eng "$source_eng" \
    --argjson derived "$derived" \
    --argjson amp "$amp" '{
      source: $source,
      derivatives: $derivatives,
      source_engagement: $src_eng,
      derived_engagement: $derived,
      amplification_ratio: $amp
    }'
}

# ca_list_sources
# Enumerate all source content with at least one derivative across snapshots.
# Today: every distinct .source_video.id across post-manifests.
ca_list_sources() {
  local m
  local out="[]"
  while IFS= read -r m; do
    [[ -z "$m" ]] && continue
    local svid stitle surl
    svid=$(jq -r '.source_video.id // empty' "$m" 2>/dev/null)
    [[ -z "$svid" ]] && continue
    stitle=$(jq -r '.source_video.title // ""' "$m" 2>/dev/null)
    surl=$(jq -r '.source_video.url // ""' "$m" 2>/dev/null)
    local n_clips n_posts
    n_clips=$(jq -r '.clips | length' "$m" 2>/dev/null)
    n_posts=$(jq -r '[.clips[]?.scheduled_posts | length] | add // 0' "$m" 2>/dev/null)
    out=$(printf '%s' "$out" | jq -c \
      --arg id "$svid" --arg t "$stitle" --arg u "$surl" --arg path "$m" \
      --argjson nc "${n_clips:-0}" --argjson np "${n_posts:-0}" '
      . + [{id: $id, title: $t, url: $u, manifest_path: $path, n_derivatives: $nc, n_scheduled_posts: $np}]
    ')
  done < <(_ca_all_manifests)
  printf '%s\n' "$out" | jq -c 'unique_by(.id)'
}

# ca_render_report <source_id> [--format md|json]
ca_render_report() {
  local sid="$1"; shift
  local fmt="md"
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --format) fmt="$2"; shift 2 ;;
      *) echo "ca_render_report: unknown flag '$1'" >&2; return 2 ;;
    esac
  done
  [[ -n "$sid" ]] || { echo "ca_render_report: source_id required" >&2; return 2; }

  local rec
  rec=$(ca_join_engagement "$sid")

  if [[ "$fmt" == "json" ]]; then
    printf '%s' "$rec" | jq .
    return 0
  fi

  printf '%s' "$rec" | jq -r '
    "# Source-content closed-loop: \(.source.title // .source.id)",
    "",
    "- ID: `\(.source.id)`  (\(.source.type))",
    "- URL: \(.source.url // "n/a")",
    "- Published: \(.source.published_at // "n/a")",
    "- Duration: \(.source.duration_seconds // "n/a")s",
    "",
    "## Source engagement",
    (if .source_engagement then
      "- Views: \(.source_engagement.views) | Likes: \(.source_engagement.likes) | Comments: \(.source_engagement.comments) | Subs gained: \(.source_engagement.subs_gained) | Est. revenue: $\(.source_engagement.estimated_revenue)"
    else "- (no source engagement available)" end),
    "",
    "## Derived engagement (sum across derivatives)",
    "- Reach: \(.derived_engagement.reach) | Reactions: \(.derived_engagement.reactions) | Comments: \(.derived_engagement.comments) | Subs gained: \(.derived_engagement.subs_gained) | Est. revenue: $\(.derived_engagement.estimated_revenue)",
    "- Amplification ratio: \(.amplification_ratio // "n/a")",
    "",
    "## Derivatives (\(.derivatives | length))",
    "",
    (
      .derivatives[]
      | "### \(.title // .clip_id)  (score \(.score // "?"), \(.duration_seconds // "?")s)",
        "",
        (
          .platforms | to_entries[]
          | "- **\(.key)**: " +
            (if .value.pending then "_pending \(.value.pending_task // .value.reason // "")_"
             elif .value.reason == "no_match" then "_no_match_"
             elif .value.reason == "not_yet_aired" then "_scheduled \(.value.scheduled_at_utc)_"
             else "join=\(.value.join_method // "?") | " +
                  ([.value | to_entries[] | select(.key | IN("join_method","video_id","post_id","urn") | not) | "\(.key)=\(.value)"] | join(" "))
             end)
        ),
        ""
    )
  '
}
