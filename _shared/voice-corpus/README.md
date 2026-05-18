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
voice-corpus --num 3        # override num_recent (-1 = use config; 0 = all in feed)
voice-corpus --print-only   # print existing cache, do not fetch
```

## Output shape

```json
{
  "fetched_at": "2026-04-27T...",
  "feed_url": "https://rss.beehiiv.com/feeds/9AbhG8CTgD.xml",
  "num_posts": 12,
  "posts": [
    {
      "title": "Tokens From Our Past and The Great Re-Why-ing",
      "url": "https://www.enterprisevibecode.com/p/...",
      "published_at": "2026-04-26",
      "body_text": "<full plain-text body, capped per max_chars_per_post — currently 50000 chars, effectively full body>"
    }
  ]
}
```

## Config

`config.json` (committed):
- `feed_url` — beehiiv RSS feed
- `num_recent` — how many recent posts to cache (default `0` = all items the feed returns; ~12 for an active beehiiv account)
- `max_chars_per_post` — truncate bodies (default **50000**, effectively full body since the largest beehiiv post in this corpus is ~23K chars; bumped from 2000 on 2026-05-17 after a redundant-truncation incident — see Consumers section below)
- `stale_days` — cache TTL (default 7)
- `cache_path` — relative to binary dir

`config.local.json` (optional, gitignored): override any of the above per-user.

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
