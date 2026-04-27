# voice-corpus

Fetches the author's recent newsletters from beehiiv RSS, caches locally, and prints as JSON for compose-phase prompts in skills that generate **original copy** (not verbatim quotes, not full-article syndication).

Pure transport per [PRIMITIVE-TEST.md](../../PRIMITIVE-TEST.md). The judgment about which excerpts to use, how to weight them, or how to interpret the voice belongs in the caller skill's prompt — not here.

## Build

```bash
cd _shared/voice-corpus && go build .
```

## Usage

```bash
voice-corpus                # fetch if cache stale, print cache JSON to stdout
voice-corpus --refresh      # force fetch, ignore cache age
voice-corpus --num 3        # override num_recent
voice-corpus --print-only   # print existing cache, do not fetch
```

## Output shape

```json
{
  "fetched_at": "2026-04-27T...",
  "feed_url": "https://rss.beehiiv.com/feeds/9AbhG8CTgD.xml",
  "num_posts": 5,
  "posts": [
    {
      "title": "Tokens From Our Past and The Great Re-Why-ing",
      "url": "https://www.enterprisevibecode.com/p/...",
      "published_at": "2026-04-26",
      "body_text": "<first 2000 chars of plain-text body>"
    }
  ]
}
```

## Config

`config.json` (committed):
- `feed_url` — beehiiv RSS feed
- `num_recent` — how many recent posts to cache (default 5)
- `max_chars_per_post` — truncate bodies (default 2000 ≈ 350 words)
- `stale_days` — cache TTL (default 7)
- `cache_path` — relative to binary dir

`config.local.json` (optional, gitignored): override any of the above per-user.

## Caller pattern

See [PATTERNS.md#pattern-voice-grounding-for-original-copy-generation](../../PATTERNS.md#pattern-voice-grounding-for-original-copy-generation).

## Skills using this

- `/tease-newsletter` — Phase 4 (original teaser hooks)
- `/promote-github` — Phase 4 (value/impact framing)
- `/carousel-newsletter` — Phase 2 (slide script — original-copy slides only; quote slides 3/5/7 stay verbatim)
