# voice-corpus

Fetches the author's recent newsletters from beehiiv RSS **and** recent YouTube **livestream** transcripts, caches locally, and prints as JSON for compose-phase prompts in skills that generate **original copy** (not verbatim quotes, not full-article syndication).

Two voice sources, merged into one flat `posts[]` and tagged with `source_type`:
- `newsletter` — beehiiv RSS (the author's *written* register).
- `youtube_live` — transcripts of recent livestreams (`video_type=="live"` in `youtube-analytics/data/videos.json`), the author's *spoken* register. Long-form videos are **excluded** (they're just the author reading the newsletter, already covered by the beehiiv slice) and so are Shorts (clip captions, not authored voice).

The two slices refresh on **independent TTLs** (`stale_days` for newsletters, `youtube_stale_days` for livestream archives, which rarely change). A livestream failure (no captions / private / `youtube_transcript_api` not installed) is per-video and **never fatal** — the newsletter slice the consumer skills depend on always survives.

Pure transport per [PRIMITIVE-TEST.md](../../PRIMITIVE-TEST.md). The judgment about which excerpts to use, how to weight them, or how to interpret the voice belongs in the caller skill's prompt — not here. In particular: transcripts are intentionally passed **raw** (unpunctuated, filler included). Cleaning, summarizing, or down-weighting the spoken register vs the written one is the consumer's call — use the `source_type` tag.

## Build

```bash
cd _shared/voice-corpus && go build .
```

## Usage

```bash
voice-corpus                # fetch if cache stale, print cache JSON to stdout
voice-corpus --refresh      # force fetch, ignore cache age (both sources)
voice-corpus --num 3        # override num_recent (-1 = use config; 0 = all in feed) — newsletters
voice-corpus --print-only   # print existing cache, do not fetch
voice-corpus --youtube      # force-enable livestream transcript ingestion
voice-corpus --no-youtube   # force-disable livestream transcript ingestion
```

## Output shape

```json
{
  "fetched_at": "2026-04-27T...",            // newsletter slice fetch time
  "youtube_fetched_at": "2026-04-27T...",    // livestream slice fetch time (omitted if not ingested)
  "feed_url": "https://rss.beehiiv.com/feeds/9AbhG8CTgD.xml",
  "num_posts": 24,
  "posts": [
    {
      "title": "Tokens From Our Past and The Great Re-Why-ing",
      "url": "https://www.enterprisevibecode.com/p/...",
      "published_at": "2026-04-26",
      "source_type": "newsletter",
      "body_text": "<full plain-text body, capped per max_chars_per_post>"
    },
    {
      "title": "What do we build now? The Bitter Lesson … Enterprise Vibe Code 45",
      "url": "https://www.youtube.com/watch?v=PLl-6JQQVbo",
      "published_at": "2026-05-30",
      "source_type": "youtube_live",
      "body_text": "<raw livestream transcript, capped per youtube_max_chars_per_post>"
    }
  ]
}
```

## Config

`config.json` (committed):
- `feed_url` — beehiiv RSS feed
- `num_recent` — how many recent newsletter posts to cache (default `0` = all items the feed returns; ~14 for an active beehiiv account)
- `max_chars_per_post` — truncate newsletter bodies (default **50000**, effectively full body; bumped from 2000 on 2026-05-17 after a redundant-truncation incident — see Consumers section below)
- `stale_days` — newsletter cache TTL (default 7)
- `cache_path` — relative to binary dir
- `ingest_youtube` — merge recent livestream transcripts (default **true**)
- `youtube_videos_path` — `youtube-analytics` video list (relative to binary dir or absolute)
- `youtube_transcript_script` — path to `generate-transcript.sh` (relative to binary dir or absolute)
- `youtube_num_recent` — most-recent N livestreams to transcribe (default 10)
- `youtube_max_chars_per_post` — per-transcript truncation cap (default 50000)
- `youtube_stale_days` — livestream slice TTL (default 30 — archives don't change)

`config.local.json` (optional, gitignored): override any of the above per-user.

**Dependency:** livestream ingestion shells out to `youtube_transcript_api` (via `youtube-analytics/scripts/generate-transcript.sh`). Install once: `pipx install youtube-transcript-api`. If absent, livestream ingestion degrades gracefully (logged to stderr) and the newsletter slice is unaffected.

## Consumers of this binary — do NOT add a second truncation

**Confirmed rule (2026-05-17, codified here 2026-05-18):** when a caller skill builds a prompt that inlines `body_text` from this binary's output, use `body_text` **as-is**. Do NOT add a second truncation like `body_text[:1500]` or any similar inline cap in the prompt-builder script.

**Why:** the binary already truncates each post body per `max_chars_per_post`. Adding a second truncation in the caller stacks two limits and silently strips signal the binary intended to pass through. Confirmed incident 2026-05-16: a promote-github prompt-builder used `body_text[:1500]` while the binary cap was 2000, so reviewers saw only ~15% of the user's actual voice corpus. Subsequent fix bumped the binary cap to 50000 to pass the full body through — that fix is undermined if any caller re-truncates.

**How to apply:**
- In any Python / JS / Go prompt-assembler that takes `posts: [{body_text: ...}]` from this binary, write `{p["body_text"]}` — never `{p["body_text"][:N]}`.
- If the assembled prompt grows too large for a model's context, raise the issue with the user, don't silently truncate. With current Claude / Codex / Gemini context windows (200K+ tokens), even the full ~180K-char corpus is ~25% of one model's window.
- If a SHORTER excerpt is genuinely needed for a specific use case (e.g., a 200-char hook), extract the FIRST SENTENCE programmatically (via natural sentence boundary) rather than slicing by character count — semantic boundary preserves meaning, char-slice doesn't.

**For the binary's own truncation:** `max_chars_per_post` exists as a runaway-guard only (in case a beehiiv post somehow exceeds the 50K cap). If active truncation matters for your workflow, lower the binary's cap via `config.local.json` — don't paper it over in the caller.

## Caller pattern

See [PATTERNS.md#pattern-voice-grounding-for-original-copy-generation](../../PATTERNS.md#pattern-voice-grounding-for-original-copy-generation).

## Skills using this

- `/tease-newsletter` — Phase 4 (original teaser hooks)
- `/promote-github` — Phase 4 (value/impact framing)
- `/carousel-newsletter` — Phase 2 (slide script — original-copy slides only; quote slides 3/5/7 stay verbatim)
