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

Copy `tag-ids.example.json` to `tag-ids.local.json` (gitignored — IDs are per-organization) and replace the placeholders with real IDs:

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
