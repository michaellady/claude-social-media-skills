# _shared

Pure-transport helpers used by the skills in this repo. **No cognition** — these helpers refuse invalid input and never make decisions about content. The cognition stays in the skill prompts.

See [PATTERNS.md](../PATTERNS.md) for cross-skill workflows that DO involve cognition (adversarial review, when-to-handoff, queue-overlap interpretation).

## Files

| File | Purpose | Language |
|---|---|---|
| `cta.sh` | Print the canonical "Comment newsletter" CTA for a beehiiv article title | Shell |
| `format_tags.json` | Authoritative list of `format:<name>` Buffer post tag values | JSON data |
| `gstack_auth.sh` | Verify gstack browse is logged in; attempt cookie import once if not | Shell |
| `buffer-post-prep/` | Validate + shape arguments for `mcp__buffer__create_post` | Go |
| `buffer-queue-check/` | Filter Buffer posts (queued + sent) by distinctive phrases | Go |

## Build

The Go binaries need to be built before first use:

```bash
cd _shared/buffer-post-prep && go build .
cd _shared/buffer-queue-check && go build .
```

Built binaries are gitignored — each user builds locally. The Go source is the source of truth.

## Why these are pure transport

Each helper passes [The Primitive Test](https://github.com/michaellady/claude-social-media-skills/blob/main/PATTERNS.md#why-this-isnt-code):

- **Atomicity:** stateless; safe to call concurrently.
- **Bitter Lesson:** a smarter model still needs validated JSON shapes and deterministic queue scans.
- **ZFC:** no `if stuck then X` branches anywhere.

If a helper grows a cognition branch (an `if` that requires judgment), refactor it: move the cognition into the calling skill's prompt; keep only the transport here.
