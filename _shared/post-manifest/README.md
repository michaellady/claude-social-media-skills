# post-manifest

JSON manifest format for **closed-loop attribution of posts scheduled outside Buffer**. Captures the per-post API IDs returned by whatever scheduler published the content, so a future fetcher can correlate platform-native analytics back to the source content (clip, video, article, PR).

## Why this exists (and when NOT to use it)

The repo's primary closed-loop mechanism is **Buffer's `format:<name>` tag system** — every compose-and-publish skill tags its Buffer posts with one of the 6 canonical format tags (see `ARCHITECTURE.md` + `_shared/format_tags.json`), and `buffer-stats` reads those tags to attribute engagement per (channel, format).

That works because Buffer is the publishing layer. When Buffer **isn't** the publisher — when something else (OpusClip's native scheduler, LinkedIn pulse API, etc.) publishes directly — the `format:` tag never gets applied, and `buffer-stats` has no per-post record to attribute against.

This pattern fills that gap. It's a parallel attribution path, not a replacement.

| Scenario | Attribution path |
|---|---|
| Skill schedules via Buffer (`promote-newsletter`, `carousel-newsletter`, `promote-github`, `tease-newsletter`) | `format:<name>` Buffer tag → `buffer-stats` per-format breakdown. **Don't use this manifest.** |
| Skill schedules via OpusClip native API (`opus-clips`) | This manifest + in-text `[opus:<clip_id>]` tag → future per-platform fetcher. **Use this manifest.** |
| Skill publishes directly to platform native editors (`crosspost-newsletter` — LinkedIn pulse, Medium, Substack) | Platform-native analytics already covers each (LinkedIn → `linkedin-stats`, Medium → no fetcher yet). **Use this manifest** if you need per-publish ID tracking for future correlation. |

## JSON shape

A manifest is a single JSON object keyed by the source content. Path convention: `~/dev/youtube_analytics/data/<scheduler>/<source_id>.json` — keep manifests near related analytics. Examples:

- `~/dev/youtube_analytics/data/opus_clips/P3051823ab0w.json` (OpusClip project)
- `~/dev/youtube_analytics/data/linkedin_pulses/<pulse_slug>.json` (future)

```jsonc
{
  "project_id": "P3051823ab0w",                    // scheduler's project/parent ID
  "source_video": {                                // optional: link back to upstream content
    "id": "uEposKmbFvY",
    "title": "How to Scale Without the Slop",
    "url": "https://www.youtube.com/watch?v=uEposKmbFvY"
  },
  "voice_grounding": {                             // optional: record voice-corpus rules used at compose time
    "corpus_source": "_shared/voice-corpus",
    "key_voice_signals": ["compounding", "agentic", "..."],
    "rules": ["no exclamation", "ground in transcript", "..."]
  },
  "created_at": "2026-05-18T23:36:43Z",
  "clips": [                                       // or "posts", "pieces" — whatever the unit is
    {
      "clip_id": "La4Wghg6IX",                     // scheduler's per-unit ID
      "title": "...",                              // composed title
      "description": "... [opus:La4Wghg6IX]",      // composed description — MUST include the in-text tag
      "score": 99,                                 // optional per-unit metadata
      "duration_sec": 28,
      "scheduled_posts": [                         // one entry per (clip × channel) schedule call
        {
          "label": "FACEBOOK_PAGE Enterprise Vibe Code",
          "account_id": "6946d924a163ebb2222f505b",
          "sub_account_id": "913906768469231",
          "scheduled_at_utc": "2026-05-19T16:00:00Z",
          "api_response": {                        // VERBATIM scheduler response — preserves scheduleId, postId, conflict flags
            "data": {
              "scheduleId": "1779159097402OmCL-FACEBOOK_PAGE",
              "postId": "1779159097402QTRQ-FACEBOOK_PAGE",
              "hasConflict": false,
              "publishAt": "2026-05-19T16:00:00.000Z"
            }
          }
        }
      ]
    }
  ]
}
```

## In-text tag convention

Every scheduled post's description **must** include a footer tag of the form `[<scheme>:<id>]`:

| Scheme | Identifies | Example |
|---|---|---|
| `opus` | OpusClip clip ID | `[opus:La4Wghg6IX]` |
| `lp` | LinkedIn pulse slug | `[lp:scaling-without-slop-essay]` |
| `gh` | GitHub source (PR or commit SHA prefix) | `[gh:bun/30412]` or `[gh:abc1234]` |
| `bh` | Beehiiv post slug | `[bh:fix-forward-solutions]` |

**Why a free-form text tag in addition to the manifest?** The manifest is machine-readable but lives on the user's disk. The in-text tag travels with the post itself — so a future fetcher (or the user, or even a 3rd party) can grep platform-native search and recover the source-content link without the manifest. Defense in depth.

**Placement:** last line of the description, separated by a blank line from the body. Don't put it in the title (eats title char budget on some platforms).

## Helper functions (sourceable bash)

```bash
source ~/dev/claude-social-media-skills/_shared/post-manifest/post_manifest.sh
```

Then in your scheduling skill:

```bash
MANIFEST=~/dev/youtube_analytics/data/opus_clips/P3051823ab0w.json

# 1. Initialize (idempotent — if file exists, leave alone unless --force)
pm_init "$MANIFEST" \
  --project P3051823ab0w \
  --source-video uEposKmbFvY \
  --source-title "How to Scale Without the Slop"

# 2. Ensure a clip entry exists (idempotent)
pm_ensure_clip "$MANIFEST" \
  --clip-id La4Wghg6IX \
  --title "Your next 50% productivity gain isn't a new AI tool" \
  --description "Hormozi's math: ... [opus:La4Wghg6IX]" \
  --score 99 --duration-sec 28

# 3. After firing a schedule call, record the response
pm_append_post "$MANIFEST" \
  --clip-id La4Wghg6IX \
  --label "FACEBOOK_PAGE Enterprise Vibe Code" \
  --account-id 6946d924a163ebb2222f505b \
  --sub-account-id 913906768469231 \
  --scheduled-at-utc 2026-05-19T16:00:00Z \
  --api-response "$RAW_API_JSON"

# 4. Query (read-side)
pm_count_scheduled "$MANIFEST"                  # total (clip × channel) schedule entries
pm_list_by_channel "$MANIFEST" "FACEBOOK_PAGE"  # all posts to a label substring
pm_find_clip "$MANIFEST" --clip-id La4Wghg6IX   # one clip's full record
pm_conflicts "$MANIFEST"                        # entries where api_response.data.hasConflict == true
```

The helpers are intentionally thin jq wrappers — they encode the **shape contract**, nothing more. They don't talk to any scheduler; the scheduling skill calls its own scheduler and passes the response.

## Consumers

- **`opus-clips`** — primary user. Schedules OpusClip projects → manifest per project at `~/dev/youtube_analytics/data/opus_clips/<project_id>.json`.
- **`flywheel`** — future input: read manifests from any scheduler directory under `~/dev/youtube_analytics/data/` to count posts toward Priority 1 throughput + measure derivative-format reach.
- **`crosspost-newsletter`** — could write a manifest per article to record LinkedIn pulse / Medium / Substack publish IDs for cross-surface correlation (not wired today; left as future work).
- **Future** `/opus-clips-performance` (or similar) — would walk the manifest's `scheduled_posts[].api_response.data.scheduleId`s and poll each platform's native analytics API to fill in per-post engagement metrics.

## Closed-loop integration with the existing `format:` tag system

This pattern is **additive**, not replacement. A complete picture of "what got published where":

```
Buffer-published posts → format:<name> tag → buffer-stats engagement attribution
                                                      ↓
                                              per-(channel, format) ROI scores
                                                      ↓
                                                   /flywheel
                                                      ↑
                                              per-(channel, source) reach counts
                                                      ↑
Non-Buffer-published posts → post-manifest + [scheme:id] tag → future native-platform fetcher
```

`/flywheel` is the joining point. Today it reads `buffer-stats` snapshots; tomorrow it will also read post-manifests to count throughput from non-Buffer surfaces.

## What's NOT in the manifest

- **Engagement metrics.** The manifest is a publication ledger, not an analytics store. Engagement comes from platform-native APIs and gets merged downstream (in the fetcher, not here).
- **Source content body.** Just metadata + IDs. The source video / article / clip lives where it always lived.
- **Secrets.** No API keys, no OAuth tokens. The manifest is meant to be diff-able and (in principle) shareable — though in practice `~/dev/youtube_analytics/data/` is gitignored.

## Versioning

Schema-version-naïve today (no `version:` field). If the shape changes meaningfully, add `"schema_version": "2"` at the top level and have consumers branch on it. For now: append-only new fields are fine; renaming or removing fields is a breaking change that needs migration.
