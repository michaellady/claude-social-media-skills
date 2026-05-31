# insights — the cross-surface hypothesis ledger

This is the repo-level **predict → grade → learn** ledger for the *whole* social presence (Buffer formats, LinkedIn, newsletter, per-source amplification). It is the cross-surface counterpart to `youtube-analytics/data/insights/` (which stays the per-video YouTube ledger). Both share the **same file format and the same tested binary** — the `youtube-analytics insights` command, pointed here with `--ledger-dir`.

`/flywheel` drives this ledger every weekly run (its "Compound" phase): it **grades** last week's predictions against the numbers it just computed (`content_attribution[]`, `format_engagement`, `channel_roi[]`), then **writes** next week's. The dashboard's Learning panel renders this ledger alongside the YouTube one.

## File shape

One `<date>.md` per week, YAML frontmatter + markdown body:

```yaml
---
date: "2026-06-01"
hypotheses:
  - id: h-x-2026-06-01-1
    surface: "format:carousel"          # the cross-surface scope this is about
    cohort: ""                            # (YouTube-only; usually empty here)
    prediction: "Carousels keep out-engaging teasers on LinkedIn (>2x eng/post)."
    metric: eng_rate_pct
    direction: up
    evaluate_after: "2026-06-08"
    outcome: ""                           # filled at grade time
    verdict: ""                           # confirm | refute | inconclusive
---
# Insights — 2026-06-01
```

`surface` values: `cross`, `linkedin`, `instagram`, `format:<name>` (e.g. `format:carousel`), `source/<id>` (a long-form video ID or newsletter slug whose amplification we're tracking).

## Commands (run from `youtube-analytics/`, all flags **before** any positional)

```bash
yt=./youtube-analytics   # or: go run .
LED=../insights

$yt insights list    --ledger-dir "$LED"                     # table
$yt insights list    --ledger-dir "$LED" --json              # structured (dashboard)
$yt insights pending --ledger-dir "$LED" --as-of 2026-06-08  # past-due, ungraded
$yt insights grade   --ledger-dir "$LED" --verdict confirm --outcome "carousel 0.82 vs teaser 0.41" h-x-2026-06-01-1
$yt insights new     --ledger-dir "$LED" 2026-06-08          # scaffold next week
```

> **Flag ordering:** Go's stdlib `flag` stops at the first positional, so `--verdict`/`--outcome`/`--ledger-dir` must come *before* the hypothesis id (or the date for `new`). `grade --verdict X ... <id>` works; `grade <id> --verdict X` silently drops the flags.

A cross-surface hypothesis has **no** `evidence_video_ids`, so `pending` does **not** require a fresh `youtube-analytics/data/videos.json` to grade it — the flywheel run supplies the cross-surface evidence.
