---
name: opus-clips-performance
description: Use when user wants engagement metrics for clips scheduled via OpusClip — "how did my clips do", "opus clip performance", "clip engagement report", "which clips landed", "per-clip stats", "opus-clips report". Thin renderer over the shared `_shared/content-attribution` JOIN engine — calls `content-attribution sources` / `join --source-id <video-id>` to get the per-source record, then renders a per-clip rollup from its `derivatives[]` (each derivative is a clip; `.platforms` holds per-platform engagement). The engine does the YouTube join; FB/IG/TikTok/LinkedIn come back `pending` and are filled in by an in-browser native-analytics scrape overlay (claude-in-chrome), matched by the `[opus:]` tag — folded in from the retired `clip-results` skill (2026-05-30).
user_invocable: true
---

# opus-clips-performance

Per-clip engagement report for OpusClip-scheduled posts. A **thin renderer** over the shared [`_shared/content-attribution/`](../_shared/content-attribution/README.md) JOIN engine: it asks the engine for the unified per-source record (`content-attribution join --source-id <video-id>`), whose `derivatives[]` array is exactly the per-clip rollup this report renders (each derivative = one clip; `derivative.platforms` holds the per-platform engagement). This skill emits:

1. A markdown `report-<date>.md` next to the manifest — one section per clip with score, duration, scheduled fan-out, and engagement, rendered from the engine's `derivatives[]`.
2. An additive update to the manifest: each `scheduled_posts[]` entry gains an `engagement` block (or `engagement: null` + `pending_task` pointer for platforms the engine returns as `pending`), folding in any browser-scraped engagement this run captured.

Why this exists: `/opus-clips` writes a publication ledger but no engagement data ("What's NOT in the manifest" — see post-manifest README). The shared JOIN engine does the correlation (`[opus:<clip_id>]` tag / `scheduleId` / ±2h time-window — **there is one correlation implementation now, in the engine, not here**). This skill's remaining job is (a) drive the engine per source, (b) overlay browser-scraped engagement for the platforms the engine can't reach yet, and (c) render + persist. `/flywheel` Phase 4.56 reads the same engine for the cross-project rollup.

## Usage

`/opus-clips-performance` — process every manifest under `~/dev/claude-social-media-skills/youtube-analytics/data/opus_clips/*.json` whose newest scheduled post is older than 24 h (gives platforms time to ingest analytics).
`/opus-clips-performance <project_id>` — process one manifest (`P3051823ab0w` or full path).
`/opus-clips-performance --since YYYY-MM-DD` — only manifests with scheduled posts on/after this date.
`/opus-clips-performance --dry-run` — render the report to stdout, don't mutate manifests or write files.
`/opus-clips-performance --platforms youtube` — restrict to one platform (today only `youtube` is wired; the flag is for forward-compat with #370/#371/#373).

## 🟢 Happy Path

For one fully-fanned-out OpusClip project (23 clips × 6 channels = 138 scheduled posts, the live `P3051823ab0w` shape) once 24 h have elapsed. ~30 sec wall-clock today (only the engine's YouTube join hits disk; other platforms short-circuit to `pending` unless the interactive browser-scrape overlay runs).

**Phase 0 — Build the JOIN engine if missing.** The engine is a gitignored Go binary (built per-machine). Build it on demand; degrade gracefully if `go` is absent (see Phase 0 below).

**Phase 1 — Discover sources.** Ask the engine: `content-attribution sources` enumerates every source video with ≥1 derivative across all manifests (`{id, title, manifest_path, n_derivatives, n_scheduled_posts}`). If `<project_id>` was passed, resolve it to a single manifest and read its `source_video.id` to pin one source.

**Phase 2 — JOIN (engine does the correlation).** For each source video ID, call `content-attribution join --source-id <video-id>`. The engine returns the unified record (`source`, `derivatives[]`, `source_engagement`, `derived_engagement`, `amplification_ratio`). Each `derivative` is a clip; `derivative.platforms.youtube_shorts.engagement` is the live YouTube join (the engine did the `[opus:]` tag → ±2h time-window correlation — **this skill no longer reimplements it**). The other platform keys come back `{engagement: null, pending: true, pending_task: "#NNN"}`.

**Phase 3 — Overlay browser-scraped engagement (interactive).** For the platforms the engine returns as `pending` (`facebook_page`, `instagram_business`, `linkedin_page`, `linkedin_personal`, `tiktok_business`), read each platform's **native analytics in the user's logged-in browser** via `claude-in-chrome`, match each post to a clip by its `[opus:<clip_id>]` tag, and overlay the result onto that clip's platform record (replacing the engine's `pending` placeholder for this run + the persisted manifest). The engine has no browser path — this overlay is the only home for these metrics today.

**Phase 4 — Render report.** One markdown section per clip, sorted by `derivative.score` desc, rendered directly from the engine's `derivatives[]` (+ the Phase 3 overlay). Include: clip_id, title, score, duration, per-platform engagement (live, scraped, or `pending #NNN`). Lead with the engine's `derived_engagement` + `amplification_ratio` as the aggregate. Write to `report-<YYYY-MM-DD>.md` next to the manifest.

**Phase 5 — Persist back to manifest.** Atomic write (tmp + mv, same pattern as `_pm_atomic_write`): add the `engagement` (or `engagement: null` + `pending_task`) field to each `scheduled_posts[]` entry, keyed by `scheduleId`. Additive only — never remove or rename existing fields (schema versioning rule from post-manifest README). The engine is read-only over snapshots; this skill owns the manifest write-back.

## Phases

### Phase 0 — Build the JOIN engine if missing

The correlation lives entirely in the shared engine `_shared/content-attribution/`, a gitignored Go binary (built per-machine like `voice-corpus`). Build it on demand — **same guard pattern as `/flywheel` Phase 4.56**:

```bash
CA_DIR=~/dev/claude-social-media-skills/_shared/content-attribution
CA_BIN="$CA_DIR/content-attribution"
# Build the Go binary if it's not on disk (gitignored — built per-machine).
if [ ! -x "$CA_BIN" ] && [ -f "$CA_DIR/main.go" ]; then
  ( cd "$CA_DIR" && go build -o content-attribution . ) 2>/dev/null
fi
if [ ! -x "$CA_BIN" ]; then
  CA_AVAILABLE=0
  CA_SKIP_REASON="_shared/content-attribution/ binary missing and build failed (need Go) — task #381/#383"
else
  CA_AVAILABLE=1
fi
```

`content-attribution` is a **Go binary** — it runs identically under any shell, so call it directly (no `bash -c`, no sourcing). **Graceful degradation:** if `CA_AVAILABLE=0`, fall back to the legacy in-skill discovery (`pm_count_scheduled` walk below) for a publication-count-only report and tell the user the engine is missing so YouTube engagement and `amplification_ratio` are absent this run. Don't abort.

### Phase 1 — Discover sources

Ask the engine which sources have derivatives:

```bash
# {id, title, url, manifest_path, n_derivatives, n_scheduled_posts} per source video
SOURCES=$("$CA_BIN" sources)   # JSON array
```

If `<project_id>` was passed, resolve it to a single manifest under `opus_clips/` and read `source_video.id` to pin one source ID (the engine's `--source-id`):

```bash
SRC_ID=$(jq -r '.source_video.id' "$MANIFEST")
```

For the `--since` filter, drop manifests whose newest `scheduled_at_utc` is older than the cutoff:

```bash
NEWEST=$(jq -r '[.clips[].scheduled_posts[].scheduled_at_utc] | max' "$MANIFEST")
```

**Degraded fallback (engine missing).** Discover manifests in-skill, publication-count only:

```bash
source ~/dev/claude-social-media-skills/_shared/post-manifest/post_manifest.sh
DATA_ROOT=~/dev/claude-social-media-skills/youtube-analytics/data
MANIFESTS=()
for f in "$DATA_ROOT"/*/*.json; do
  COUNT=$(pm_count_scheduled "$f" 2>/dev/null)
  [ "${COUNT:-0}" -gt 0 ] && MANIFESTS+=("$f")
done
```

### Phase 2 — JOIN (the engine does the correlation)

```bash
# One unified record per source video. derivatives[] IS the per-clip rollup.
REC=$("$CA_BIN" join --source-id "$SRC_ID")
```

The record's shape (see [`_shared/content-attribution/README.md`](../_shared/content-attribution/README.md#output-shape)):

```jsonc
{
  "source": { "id": "uEposKmbFvY", "type": "long_form", "title": "…", "url": "…" },
  "derivatives": [
    {
      "type": "opus_clip", "clip_id": "La4Wghg6IX", "title": "…", "score": 99, "duration_seconds": 28,
      "platforms": {
        "youtube_shorts":     { "engagement": { "video_id": "9nOxvAhZyWo", "views": 539, "likes": 2, "comments": 0, "subs_gained": 0, "estimated_revenue": 0.023, "join_method": "tag" } },
        "facebook_page":      { "engagement": null, "pending": true, "pending_task": "#371" },
        "instagram_business": { "engagement": null, "pending": true, "pending_task": "#371" },
        "linkedin_page":      { "engagement": null, "pending": true, "pending_task": "#371" },
        "linkedin_personal":  { "engagement": null, "pending": true, "pending_task": "#370" },
        "tiktok_business":    { "engagement": null, "pending": true, "pending_task": "#373" }
      },
      "derivative_engagement_total": { "reach": 539, "reactions": 2, "comments": 0 }
    }
    // … one derivative per clip
  ],
  "source_engagement": { "views": …, "likes": …, "comments": …, "subs_gained": …, "estimated_revenue": … },
  "derived_engagement": { "reach": …, "reactions": …, "comments": …, "subs_gained": …, "estimated_revenue": … },
  "amplification_ratio": 42.9
}
```

**Read it, don't recompute it.** The engine already did the `[opus:<clip_id>]` tag match (and the ±2h time-window fallback for re-titled Shorts) — there is now ONE correlation implementation, in the engine. Each clip is `REC.derivatives[]`; per-platform engagement is `derivative.platforms.<key>.engagement` (non-null = matched; check `.engagement != null` first, then read `.engagement.*`). `score`/`duration_seconds`/`title`/`clip_id` are on the derivative. Pull the YouTube number with e.g. `jq '.derivatives[] | {clip_id, yt: .platforms.youtube_shorts.engagement}'`.

> **Pretty-print shortcut.** `content-attribution report --source-id <id>` renders a default markdown view of the same record (add `--format json` for the raw record). Phase 4 below produces the per-clip-table UX this skill's users expect; `report` is the quick ad-hoc path.

### Phase 3 — Overlay browser-scraped engagement (interactive)

The engine returns the non-YouTube platforms as `{engagement: null, pending: true, pending_task: "#371"}` (FB/IG/LinkedIn-page, via Buffer `scheduleId` — the underlying Buffer snapshot doesn't yet carry per-post records), `"#370"` (LinkedIn-personal, gated on the linkedin-stats per-post snapshot), and `"#373"` (TikTok). **These task IDs are emitted by the engine itself** (`_shared/content-attribution/main.go`) — don't invent your own. This phase **overlays real numbers onto those `pending` records** by reading each platform's native analytics — the only path that exists today, since none of these platforms expose the per-post engagement the engine reads from a cached snapshot.

Read each platform's **native analytics in the user's logged-in browser** via `claude-in-chrome` — the proven method (folded in from the retired `clip-results` skill; first validated on `P3051823ab0w`, 2026-05-30, total ~31.5K reach across 6 platforms). **Interactive only** — needs an open, logged-in session; the daily launchd reminder prompts the user to run it, it can't go headless. If the session is non-interactive (e.g. `/flywheel`'s unattended run), skip this phase: the report shows those platforms as `pending` and a later interactive run fills them in.

Load chrome tools first: `ToolSearch select:mcp__claude-in-chrome__tabs_context_mcp,mcp__claude-in-chrome__navigate,mcp__claude-in-chrome__get_page_text,mcp__claude-in-chrome__browser_batch,mcp__claude-in-chrome__computer`.
**Use `get_page_text`, not screenshots** — it returns full captions (with the `[opus:CLIPID]` tag for exact
matching) plus the metric columns. Match each post to a clip by its `[opus:]` tag (the SAME tag key the engine
joins on), overlay the result onto that clip's `derivative.platforms.<key>.engagement` for the report, and
write it into the matching `scheduled_posts[].engagement` block in Phase 5 (the post-manifest contract that the engine + `/flywheel` read).

| Label prefix | Where (native analytics) | Notes |
|---|---|---|
| `FACEBOOK_PAGE` + `INSTAGRAM_BUSINESS` | **Meta Business Suite** → `business.facebook.com/latest/posts/published_posts/` (Content) | Covers BOTH. Widen window ~1920; Columns → add **Views** + **Follows**, drop Reach/Shares. IG captions carry the `[opus:]` tag; FB shows a short title only — match FB by the same publish time-slot as the tagged IG post. FB ≫ IG on views. NB: FB EVC/Sandbox/Mike share one `postAccountId`. |
| `TIKTOK_BUSINESS` | **TikTok Studio** → `tiktok.com/tiktokstudio/content` | Views/Likes/Comments columns; captions carry the `[opus:]` tag. **Virtualized list** — scroll in modest steps and dedupe by tag (big jumps skip rows). Highest engagement of any platform. |
| `LINKEDIN` (org) | `linkedin.com/company/<ORG_ID>/admin/analytics/updates/` → Content engagement table | Impressions per post; "Show: 20" + paginate. Usually negligible (org page ~tiny). |
| `LINKEDIN` (personal) | Me → Posts & Activity → `linkedin.com/in/<vanity>/recent-activity/all/` | "N impressions" per post; scroll-load. The personal profile reaches far more than the org page. |

If a platform nav is permission-denied, ask the user before retrying; don't loop. For any clip a sweep
couldn't reach, leave the engine's `{engagement: null, pending: true, pending_task: "#NNN"}` placeholder
in place (don't overwrite it) so the report still shows `pending` and a later run can fill it.

### Phase 4 — Render report

Render directly from the engine's record (`REC` from Phase 2) plus any Phase 3 overlay. Sort clips by `derivative.score` desc; per platform, render `engagement.*` when `engagement != null`, else `pending (<pending_task>)`.

```
# OpusClip performance — <project_id>
Source: <REC.source.title> (<REC.source.id>)
Clips: <REC.derivatives | length> · Derived reach: <REC.derived_engagement.reach> · Amplification: <REC.amplification_ratio>×
Generated: <now>

## Top clips by score

### [99] La4Wghg6IX — "Your next 50% productivity gain..." (28s)
| Platform            | Engagement                                  |
|---------------------|---------------------------------------------|
| youtube_shorts      | 539 views · 2 likes · 0 cm · +0 subs        |
| facebook_page       | pending (#371)                              |
| instagram_business  | pending (#371)                              |
| linkedin_page       | pending (#371)                              |
| linkedin_personal   | pending (#370)                              |
| tiktok_business     | pending (#373)                              |

(repeat per clip, descending score; replace `pending` rows with scraped numbers when Phase 3 ran)

## Aggregate (from the engine's derived_engagement)
Derived reach: <reach> · Reactions: <reactions> · Comments: <comments> · Subs gained: <subs_gained> · Est. revenue: $<estimated_revenue>
Source: <source_engagement.views> views · Amplification ratio: <amplification_ratio>×
```

Write to `~/dev/claude-social-media-skills/youtube-analytics/data/opus_clips/report-<YYYY-MM-DD>-<project_id>.md`.

### Phase 5 — Persist back to manifest

Build the patch from the engine's per-platform records (`REC.derivatives[].platforms.<key>`) plus the Phase 3 browser overlay, keyed by `scheduleId`. The engine is read-only over snapshots — this skill owns the manifest write-back. Use the `_pm_atomic_write` pattern from `post_manifest.sh` (tmp + mv). Update only the `engagement` and optional `pending_task` keys on each `scheduled_posts[]` entry. Do not touch any other field.

Map each `scheduled_posts[]` entry to its engine/overlay record: resolve the post's platform via its `label` (same `label → platform` mapping the engine uses: `YOUTUBE→youtube_shorts`, `FACEBOOK_PAGE→facebook_page`, `INSTAGRAM_BUSINESS→instagram_business`, `LINKEDIN Mike Lady→linkedin_personal`, other `LINKEDIN→linkedin_page`, `TIKTOK_BUSINESS→tiktok_business`), then copy that clip's `platforms.<key>` (`.engagement` or `.pending_task`) into the patch keyed by the post's `scheduleId`.

```bash
jq --argjson updates "$ENGAGEMENT_PATCH_JSON" '
  .clips |= map(
    .scheduled_posts |= map(
      . as $p
      | ($updates[$p.api_response.data.scheduleId] // null) as $u
      | if $u then . + {engagement: $u.engagement, pending_task: $u.pending_task} else . end
    )
  )
' "$MANIFEST" > "$MANIFEST.tmp" && mv "$MANIFEST.tmp" "$MANIFEST"
```

`scheduleId` is globally unique within a manifest; the engine doesn't surface it per-derivative (see "engine mismatches" below), so derive it from the manifest's own `scheduled_posts[].api_response.data.scheduleId` (already open in this phase).

## Out of scope

- **The correlation logic.** Owned by `_shared/content-attribution/` (the `[opus:<clip_id>]` tag match, the ±2h time-window fallback, `scheduleId` JOIN). This skill calls the engine and never reimplements it — there is one correlation implementation. If a JOIN key is wrong, fix it in the engine, not here.
- **Snapshot-backed FB / IG / LinkedIn / TikTok engagement.** When the underlying per-post snapshots land, the engine stops returning those platforms as `pending` and the Phase 3 browser overlay becomes redundant. The engine's pending pointers today: **#371** (FB/IG/LinkedIn-page — Buffer per-post snapshot keyed on `scheduleId`), **#370** (LinkedIn-personal per-post snapshot), **#373** (TikTok). These IDs come from the engine; this skill mirrors them.
- **Mutating the source content.** This is read-side only; no re-renders or re-scheduling. For that, see `/opus-clips`.
- **Aggregating across projects.** One source video = one report. Cross-project rollups belong in `/flywheel` (which reads the same engine).
- **Recomputing OpusClip's `score`.** That field is from OpusClip's viral-score model at curation time and is preserved verbatim (the engine passes it through on each derivative).
- **Backfilling missing `[opus:<clip_id>]` footers.** If a user edited a post and stripped the tag, the engine falls back to time-matching; neither it nor this skill re-writes the post.

## Relationship to the JOIN engine (could this skill be deprecated?)

After this refactor, opus-clips-performance is **a thin renderer over `_shared/content-attribution/`** for everything except one thing: the **interactive browser-scrape overlay** in Phase 3. The engine owns discovery (`sources`), the per-clip skeleton + the YouTube join (`join`), the aggregate (`derived_engagement` + `amplification_ratio`), and even a default markdown view (`report`). What this skill still uniquely provides:

1. **Phase 3 browser overlay** — fills the FB/IG/LinkedIn/TikTok engagement the engine returns as `pending` (no snapshot exists, no API). This has no home in the engine (which is pure transport over cached snapshots, never a browser driver).
2. **Manifest write-back** (Phase 5) — the engine is read-only over snapshots; this skill owns persisting `engagement` back into `scheduled_posts[]`.
3. **The per-clip-table report UX** users expect (vs the engine's source-centric `report` view).

**For the integrator:** once the per-post snapshots land (engine pending tasks #370/#371/#373) so the engine joins all platforms itself, items (1) collapses and this skill becomes a near-pure rendering shim over `content-attribution report` + a manifest write-back — at which point it's a candidate to deprecate/merge into `/flywheel`'s per-source rendering (the open question in CLOSED-LOOP-UNIFICATION-PLAN.md §"Open questions" #3). Until then, keep it: the interactive browser overlay is real, load-bearing work the engine can't do.

## Schema gaps / engine mismatches noticed

- **Engine emits no per-`scheduled_post` `scheduleId` on its derivative platform records.** This skill's Phase 5 write-back keys the manifest patch by `scheduleId`; the engine's `join` output identifies a platform record by its `<key>` (e.g. `linkedin_page`) and the live record's `urn`/`video_id`/`post_id`, not the originating `scheduleId`. For YouTube the `video_id` is enough; for the browser-overlay platforms this skill re-derives the `scheduleId` from the manifest itself (it already has the manifest open) rather than from the engine output. Not a blocker, but a future engine field `derivative.platforms.<key>.schedule_id` would let the write-back key purely off engine output.
- **`source_video.duration_seconds` absent in the manifest.** The engine's ±2h time-fallback would bias better toward clips near the source duration if it had it; additive, non-breaking, lives in the manifest schema (owned by `/opus-clips`), not here.
- **No `schema_version`** on manifest or engine output (the engine README acknowledges this for its own output). Both `/flywheel` and this skill now read the engine; an explicit `schema_version` would give consumers a check-and-warn anchor.

## Related skills

- **`_shared/content-attribution/`** — the JOIN engine this skill now wraps; owns all correlation. See its [README](../_shared/content-attribution/README.md).
- **`/opus-clips`** — upstream; writes the manifests the engine reads.
- **`_shared/post-manifest/`** — the manifest shape contract; legacy helper functions (`pm_count_scheduled`, `pm_schedule_ids`, `pm_find_clip`) used in the degraded fallback + Phase 5 write-back.
- **`/flywheel`** — sibling consumer; Phase 4.56 reads the SAME engine (`content-attribution join`) to credit clip output toward Priority 1. Same build-if-missing guard.
- **`/yt-analytics`** — owns `~/dev/claude-social-media-skills/youtube-analytics/data/videos.json` (the engine's YouTube source); refresh it (`go run . fetch-analytics --all`) before running if YouTube data is stale.
- **`/buffer-stats`** — owns the Buffer snapshot the engine reads for FB/IG/LinkedIn-page once #371 lands; does NOT directly cover these posts (they bypass Buffer's normal queue).
