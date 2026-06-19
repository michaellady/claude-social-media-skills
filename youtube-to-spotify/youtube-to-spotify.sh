#!/usr/bin/env bash
# youtube-to-spotify — pure-transport helper for the /youtube-to-spotify skill.
#
# Owns the deterministic, non-interactive mechanics only:
#   - resolve a YouTube URL/ID to a bare video ID
#   - download the video (yt-dlp) and produce two Spotify-compliant artifacts:
#       episode-spotify.mp4  (H.264 / AAC, ≤1080p, +faststart)  — the native video
#       episode.mp3          (libmp3lame 192k 44.1k)            — the RSS-syndicated audio
#   - probe an artifact's codecs
#   - set a React-controlled form field via gstack browse (proven native-setter pattern)
#
# Cognition (ownership judgment, show-note generation, adversarial review, the
# file-picker / publish HANDOFFS) stays in SKILL.md — this script never decides
# whether to publish and never talks to Spotify's editor beyond a single field set.
#
# Usage:
#   youtube-to-spotify.sh resolve  <url-or-id>
#   youtube-to-spotify.sh fetch    <url-or-id>          # download + transcode + probe
#   youtube-to-spotify.sh probe    <media-file>
#   youtube-to-spotify.sh setfield <css-selector> <value>   # React-safe value set via gstack
#
# Spotify for Creators reqs encoded here: video MP4 H.264(avc1)/AAC 16:9 ≤1080p,
# audio MP3. https://support.spotify.com/us/creators/article/publishing-videos/
set -euo pipefail

B="${B:-$HOME/.claude/skills/gstack/browse/dist/browse}"
WORKROOT="${YT2SP_WORKROOT:-/tmp/yt2sp}"

die() { echo "youtube-to-spotify: $*" >&2; exit 1; }

need() { command -v "$1" >/dev/null 2>&1 || die "missing dependency: $1 (see SKILL.md Prerequisites)"; }

# Strip URL noise — accept full watch URLs, youtu.be, shorts/embed, or a bare ID.
resolve_id() {
  local v="$1"
  case "$v" in
    *youtube.com/watch*v=*) v="${v##*v=}"; v="${v%%&*}" ;;
    *youtu.be/*)            v="${v##*youtu.be/}"; v="${v%%[?&]*}" ;;
    *youtube.com/shorts/*)  v="${v##*shorts/}"; v="${v%%[?&/]*}" ;;
    *youtube.com/embed/*)   v="${v##*embed/}"; v="${v%%[?&/]*}" ;;
  esac
  [ -n "$v" ] || die "could not resolve a video id from: $1"
  printf '%s' "$v"
}

# Print "<video_codec> <audio_codec>" for a media file.
probe_codecs() {
  need ffprobe
  local f="$1"
  local vc ac
  vc=$(ffprobe -v error -select_streams v:0 -show_entries stream=codec_name -of csv=p=0 "$f" 2>/dev/null | head -1)
  ac=$(ffprobe -v error -select_streams a:0 -show_entries stream=codec_name -of csv=p=0 "$f" 2>/dev/null | head -1)
  printf '%s %s' "${vc:-none}" "${ac:-none}"
}

cmd_resolve() {
  [ $# -ge 1 ] || die "usage: resolve <url-or-id>"
  resolve_id "$1"; echo
}

cmd_probe() {
  [ $# -ge 1 ] || die "usage: probe <media-file>"
  [ -f "$1" ] || die "no such file: $1"
  local cc; cc=$(probe_codecs "$1")
  local dur; dur=$(ffprobe -v error -show_entries format=duration -of csv=p=0 "$1" 2>/dev/null | head -1)
  echo "file=$1 codecs=$cc duration_sec=${dur%.*}"
}

cmd_fetch() {
  [ $# -ge 1 ] || die "usage: fetch <url-or-id>"
  need yt-dlp; need ffmpeg; need ffprobe
  local url="$1"
  local id; id=$(resolve_id "$url")
  # If we were handed a bare ID, build a canonical watch URL for yt-dlp.
  case "$url" in http*) : ;; *) url="https://www.youtube.com/watch?v=$id" ;; esac

  local wd="$WORKROOT/$id"
  mkdir -p "$wd"
  local raw="$wd/episode.mp4"
  local mp4="$wd/episode-spotify.mp4"
  local mp3="$wd/episode.mp3"

  if [ ! -f "$raw" ]; then
    echo "→ downloading $url" >&2
    yt-dlp \
      -f "bv*[height<=1080][vcodec^=avc1]+ba[acodec^=mp4a]/b[ext=mp4]/bv*+ba/b" \
      --merge-output-format mp4 \
      -o "$wd/episode.%(ext)s" "$url" >&2 || die "yt-dlp failed (private/members-only/age-gated? see Edge: private-or-members-only)"
  else
    echo "→ reusing existing download $raw" >&2
  fi
  [ -f "$raw" ] || die "expected $raw after download but it is missing"

  # Video: only transcode if not already H.264/AAC — saves minutes on long essays.
  local cc; cc=$(probe_codecs "$raw")
  case "$cc" in
    "h264 aac")
      echo "→ already H.264/AAC; using download as Spotify MP4" >&2
      cp -f "$raw" "$mp4" 2>/dev/null || ln -sf "$raw" "$mp4"
      ;;
    *)
      echo "→ transcoding to H.264/AAC ($cc → h264 aac)" >&2
      ffmpeg -y -i "$raw" \
        -c:v libx264 -profile:v high -pix_fmt yuv420p \
        -vf "scale='min(1920,iw)':-2,setsar=1" \
        -c:a aac -b:a 192k -movflags +faststart \
        "$mp4" >&2 || die "ffmpeg video transcode failed"
      ;;
  esac

  # Audio: the RSS-syndicated file. Extract from the local MP4 (no second pull).
  echo "→ extracting MP3 audio" >&2
  ffmpeg -y -i "$raw" -vn -c:a libmp3lame -b:a 192k -ar 44100 "$mp3" >&2 || die "ffmpeg mp3 extract failed"

  # Machine-readable summary on stdout for the skill to parse/verify.
  echo "VIDEO_ID=$id"
  echo "WORKDIR=$wd"
  echo "MP4=$mp4 ($(probe_codecs "$mp4"))"
  echo "MP3=$mp3 ($(probe_codecs "$mp3"))"
}

# Set a React-controlled <input>/<textarea> value safely, then verify.
# Uses the native value setter so React's controlled-input intercept doesn't
# revert it (PATTERNS.md → React form input setter; __setGoal in buffer-schedule-edit.sh).
cmd_setfield() {
  [ $# -ge 2 ] || die "usage: setfield <css-selector> <value>"
  [ -x "$B" ] || die "gstack browse not found at $B"
  need jq
  local sel="$1" val="$2"
  local sel_json val_json js
  sel_json=$(jq -nc --arg s "$sel" '$s')   # JSON-encode for safe JS embedding
  val_json=$(jq -nc --arg v "$val" '$v')
  js=$(cat <<JS
(() => {
  const el = document.querySelector($sel_json);
  if (!el) return 'NO_EL:' + $sel_json;
  const tag = el.tagName === 'TEXTAREA' ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
  const setter = Object.getOwnPropertyDescriptor(tag, 'value').set;
  setter.call(el, $val_json);
  el.dispatchEvent(new Event('input', {bubbles: true}));
  el.dispatchEvent(new Event('change', {bubbles: true}));
  el.blur();
  return 'SET:' + String(el.value).slice(0, 80);
})()
JS
)
  "$B" js "$js" 2>/dev/null
}

[ $# -ge 1 ] || die "usage: $0 {resolve|fetch|probe|setfield} ..."
sub="$1"; shift
case "$sub" in
  resolve)  cmd_resolve  "$@" ;;
  fetch)    cmd_fetch    "$@" ;;
  probe)    cmd_probe    "$@" ;;
  setfield) cmd_setfield "$@" ;;
  *) die "unknown subcommand: $sub (resolve|fetch|probe|setfield)" ;;
esac
