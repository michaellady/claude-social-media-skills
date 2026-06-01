# publish-constraints

Pre-publish validation gate for the **non-Buffer** publish path (OpusClip native
scheduling today; any future direct-publish skill). The unbypassable analog of
`_shared/buffer-post-prep` for the Buffer path: the caller pipes the batch it is
about to schedule through this binary and **reads the exit code**.

## Why it exists

OpusClip's `opusclip post schedule --title` is the post **caption** on
FB/IG/LinkedIn/TikTok but the **video title** on YouTube (100-char hard cap). On
2026-05-31 a 25-clip batch passed the long caption as `--title` for every
platform — it posted fine on four and failed silently on YouTube only (25/25,
`"The video title is invalid"`). This gate makes that whole class of bug —
*wrong field, wrong cap, silent platform-specific failure* — impossible to ship.

The load-bearing fact is one cell of the table: `youtube_shorts.title_source ==
"separate"` — post-text goes to `description`, and `title` is a separate,
100-char-capped field, *not* the caption. On every other platform the post-text
IS the title.

## Usage

```bash
go build -o publish-constraints .   # gitignored, built per-machine

echo '[
  {"clip_id":"X","label":"YOUTUBE Enterprise Vibe Code","title":"Short title","description":"the full caption…"},
  {"clip_id":"X","label":"FACEBOOK_PAGE Enterprise Vibe Code","title":"the full caption…","description":""}
]' | ./publish-constraints validate
```

- **Input:** a JSON array on stdin, one cell per `(clip × platform)` schedule the
  caller is about to fire, carrying the *exact* `title`/`description` args.
- **Output:** nothing on success; one JSON verdict object per failing cell.
- **Exit codes:** `0` all valid · `64` usage error · `65` ≥1 cell invalid.

A verdict:

```json
{"clip_id":"X","platform":"youtube_shorts","field":"title",
 "reason":"title length 214 exceeds youtube_shorts cap 100",
 "fix_hint":"youtube_shorts maps post-text to \"description\"; supply a separate <=100-char title, don't reuse the caption"}
```

## The data (`constraints.json`)

The committed, embedded source of truth for **platform-native** per-field caps —
the *ceiling*. A downstream router may impose a **tighter policy** cap (Buffer
caps Facebook at 500 even though Facebook allows far more); those policy caps
live in the router and must stay `<=` these ceilings. Add a platform or change a
cap here (and in `testdata/label-platform.golden.json` if a new label maps to
it) — never in skill prose.

| platform | title cap | description cap | post-text field | title source |
|---|---:|---:|---|---|
| `youtube_shorts` | **100** | 5000 | `description` | **separate** |
| `facebook_page` | 2200 | — | `title` | post_text |
| `instagram_business` | 2200 | — | `title` | post_text |
| `tiktok_business` | 2200 | — | `title` | post_text |
| `linkedin_personal` / `linkedin_page` | 3000 | — | `title` | post_text |
| `threads` | 500 | — | `title` | post_text |

`labelToPlatform` is the canonical copy of the OpusClip-label→platform mapping
(`_shared/content-attribution/main.go` has an identical copy for the JOIN); both
are pinned to `testdata/label-platform.golden.json` by tests in each module.
Validation **fails closed** — a label no row covers is a hard error, so a new
connected account can't slip through unvalidated.

Consumed by: `opus-clips` Phase 5.7 (pre-publish gate). See
[PATTERNS.md → Pre-publish constraint gate](../../PATTERNS.md#pattern-pre-publish-constraint-gate-per-platform-per-field).
