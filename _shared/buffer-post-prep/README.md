# buffer-post-prep

Validates + shapes arguments for `mcp__buffer__create_post`. Pure transport per [PRIMITIVE-TEST.md](../../PRIMITIVE-TEST.md). Skills running inside the Claude harness call this binary, then pass its JSON output as arguments to the actual MCP call.

## Build

```bash
cd _shared/buffer-post-prep && go build .
```

## Tag IDs (one-time setup)

**Closed-loop attribution requires Buffer Tag IDs**, not tag names. Buffer's `CreatePostInput` schema has a `tagIds: [TagId!]` field (24-char hex MongoDB ObjectIds), NOT a `tags` field. Tag *names* like `format:teaser` are silently dropped if you send them as `tags`.

Buffer's public GraphQL API does not expose a `createTag` mutation, so tags must be created in Buffer's web UI before you can attribute posts to them. One-time setup:

### 1. Create the 5 format tags in Buffer's web UI

Open Buffer → settings or post composer → Tag picker → create one tag per row in the table below. Tag *names* must match exactly (case-insensitive in Buffer's UI, but the underlying string is what's stored):

| Tag name | Produced by |
|---|---|
| `format:verbatim-quote` | `/promote-newsletter` |
| `format:teaser` | `/tease-newsletter` |
| `format:carousel` | `/carousel-newsletter` |
| `format:link-share` | `/promote-github` (individual mode) |
| `format:batch-summary` | `/promote-github` (batch mode) |

(Skip `format:long-form-pulse` — that key is reserved for a future Buffer-companion-post skill that doesn't exist yet.)

### 2. Capture the Tag IDs

After creating each tag in the UI, attach it to a single test post (any post — the post itself doesn't matter). Then list them via Buffer's GraphQL:

```
mcp__buffer__execute_query summary:"List all format:* Tag IDs after one-time UI setup" query:'
query {
  posts(input: {organizationId: "<your-org-id>"}, first: 50) {
    edges { node { tags { id name } } }
  }
}
'
```

Each tag's `id` is a 24-char hex string.

### 3. Write `tag-ids.local.json`

**Path:** `<repo-root>/_shared/buffer-post-prep/tag-ids.local.json` (gitignored — IDs are per-organization). Whenever a SKILL.md mentions `_shared/buffer-post-prep/tag-ids.local.json`, that path is **always relative to the repo root**, never to `~/.claude/skills/` (skills are symlinked from the repo, but the data file lives in the repo, not the skill cache). Use `git rev-parse --show-toplevel` if you need to resolve it programmatically.

Copy `tag-ids.example.json` to `tag-ids.local.json` and replace the placeholders with real IDs:

```json
{
  "verbatim_quote": "abc123...",
  "teaser":         "def456...",
  "carousel":       "ghi789...",
  "link_share":     "jkl012...",
  "batch_summary":  "mno345..."
}
```

### 4. Verify

Run the binary against any channel and check that the emitted JSON includes `"tagIds": ["<id>"]`:

```bash
./buffer-post-prep --channel-id <id> --service threads --text "test" --format-tag teaser | jq .tagIds
```

## Graceful degradation

If `tag-ids.local.json` is missing, malformed, or the format key isn't in it, the binary **still emits the post args** — just without `tagIds`. It warns on stderr so the caller can surface it. This means closed-loop attribution silently degrades instead of blocking publication. `audit-buffer-queue` will catch the resulting untagged posts at the next weekly review.

## Dead-channel deny-list

The skill-level `min_followers_to_promote = 50` guard skips channels that have *too few followers*. It does **not** catch a channel that HAS followers but produces *zero engagement* — a channel that's actively posted-to but dead. `buffer-post-prep` enforces that case at the transport layer with a **deny-list**.

When the target channel matches the deny-list, the binary **skips the post**: it prints a structured JSON object to stdout and exits `75` (distinct from `0`=ok, `64`/`65`=caller bug, `70`=internal):

```json
{
  "skipped": true,
  "reason": "skipped: dead channel (0 reactions/0 comments over 64 posts in 30d; …)",
  "channelId": "6935604f29ea336fd65bacf8",
  "service": "threads",
  "handle": "enterprisevibecode"
}
```

Callers should parse stdout and branch on the `"skipped"` key (or on exit code `75`): treat it as "intentionally not posted, keep going" — NOT as an error. The `reason` is formatted like the skills' other skip reasons, so an existing publish loop already handles it.

### How matching works

A channel matches a deny-list row if **either**:
- its `channel_id` equals the row's `channel_id` (preferred — stable, unambiguous), **or**
- its `service` + `--handle` both equal the row's `service` + `handle` (resilient if Buffer ever reissues the channel id). Pass `--handle` so this path is available; matching is case-insensitive and ignores a leading `@`.

### Config files

- **`dead-channels.json`** — committed seed/default. Empty list (or absent) = the mechanism is **OFF**; only explicitly-listed channels are ever skipped. Seeded with the Threads `enterprisevibecode` account (`6935604f29ea336fd65bacf8`) per `audits/threads-evc-dead-channel.md`.
- **`dead-channels.local.json`** — optional, gitignored operator override. If present, it's read **instead of** `dead-channels.json` (it does not merge — copy the committed rows in if you want to keep them). Use it to extend the list without a commit. Copy `dead-channels.example.json` to start.

A missing or malformed deny-list never blocks a post — the binary warns on stderr and treats it as empty. The mechanism only ever blocks channels that are explicitly and parseably listed.

### Adding / removing a channel

- **Add:** append a row to `dead-channels.json` with at least `channel_id` (preferred) and a `reason` that cites the audit doc, then `go build .` is not required (the file is read at runtime). First write/update the investigation at `audits/<channel>-dead-channel.md`.
- **Remove (re-enable a channel):** delete its row and revisit the audit doc's re-assessment criteria.
