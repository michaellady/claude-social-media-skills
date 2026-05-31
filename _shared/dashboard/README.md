# dashboard — the single pane of glass

A local web app that shows the whole social presence in one view: reach per surface, format ROI, channel-ROI buckets, the per-source amplification leaderboard (click a source → its derivatives across every platform), the YouTube views-vs-retention scatter, voice-corpus freshness, and the cross-surface + YouTube hypothesis ledgers side by side.

Pure transport per [PRIMITIVE-TEST.md](../../PRIMITIVE-TEST.md): it **reads + serves + renders**. Every cross-surface aggregate (ROI bucketing, the amplification clamp, reconciled reach) is computed once upstream by `/flywheel` and read from its snapshot — the dashboard never recomputes a number flywheel already decided. All judgment stays in `/flywheel` and the insights ledger.

## Run

```bash
make dashboard            # from the repo root — builds + serves + opens the browser
# or directly:
cd _shared/dashboard && go build . && ./dashboard serve --open
```

Then open <http://127.0.0.1:7777/>. The server is bound to `127.0.0.1` only (never exposed). It re-reads the **newest** snapshot on every request, so a fresh `/flywheel` run shows up on browser refresh — no restart needed. Chart.js is vendored (`static/chart.umd.min.js`, embedded via `//go:embed`), so the dashboard renders **offline**.

## What it reads (read-only; never writes)

| Source | For |
|---|---|
| `~/dev/flywheel-snapshots/<newest>.json` (the **frozen contract**, flywheel Phase 7) | all aggregates: reach, channel ROI, format ROI, per-source amplification, voice freshness |
| `youtube-analytics/data/videos.json` | YouTube views-vs-retention drill-down |
| `linkedin-stats/cache/snapshot-*.json` | LinkedIn per-post drill-down |
| `buffer-stats/cache/snapshot-*.json` | Buffer channels (aggregate-only; per-post is `available:false`) |
| `_shared/voice-corpus/cache.json` | voice freshness card (fallback when the snapshot predates the contract) |
| `insights/*.md` + `youtube-analytics/data/insights/*.md` | the Learning panel (both ledgers) |

Every path is env-overridable (`DASH_FLYWHEEL_SNAPSHOT`, `DASH_FLYWHEEL_SNAPSHOT_DIR`, `DASH_YT_VIDEOS`, `DASH_BUFFER_CACHE_DIR`, `DASH_LINKEDIN_CACHE_DIR`, `DASH_VOICE_CACHE`, `DASH_XSURFACE_LEDGER`, `DASH_YT_LEDGER`, `DASH_REPO_ROOT`).

## Graceful degradation

- **No snapshot found** → a "run /flywheel first" banner, HTTP 200, no crash.
- **Old-shape snapshot** (pre-`schema_version:"1"`, missing `channel_roi`/`format_engagement`/`reconciled_reach`) → those panels show a "run /flywheel to populate" note; everything present still renders. Decoding is tolerant (raw top-level key map), so a missing key never panics.
- **Missing per-platform snapshot** → that drill-down shows `available:false` with a reason.

## API

`GET /api/overview` · `GET /api/learning` · `GET /api/youtube` · `GET /api/linkedin` · `GET /api/source/<id>` — all JSON. The single-page app in `static/` consumes them.

## Build

Built per-machine (gitignored binary, like `voice-corpus` and `content-attribution`):

```bash
cd _shared/dashboard && go build .
```
