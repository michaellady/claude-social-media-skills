# adversarial-review (vendored)

Dual-reviewer adversarial-review transport: dispatches the same prompt to both
the Claude and Codex CLIs in parallel, parses each reviewer's JSON verdict,
and emits a merged canonical response.

This directory is **vendored from**
[`mike-skills/adversarial-review`](https://github.com/michaellady/mike-skills/tree/main/adversarial-review)
plus the shared
[`mike-skills/llm-provider`](https://github.com/michaellady/mike-skills/tree/main/llm-provider)
module that powers the CLI dispatch. The vendoring exists so a clone of
`claude-social-media-skills` works end-to-end without a separate `mike-skills`
checkout.

## Layout

```
_shared/adversarial-review/
  SKILL.md                         # canonical skill spec (authoritative copy in mike-skills)
  README.md                        # this file
  go.mod                           # binary's module + replace → ./internal/llm-provider
  main.go                          # parallel dispatch + JSON parse + FAIL-OR merge
  main_test.go                     # unit tests for parse + merge
  sync.sh                          # pull fresh copies from upstream mike-skills
  internal/llm-provider/           # vendored from mike-skills/llm-provider/
    go.mod
    provider/provider.go           # Provider interface + Options + Error
    claude/claude.go               # claude CLI provider
    codex/codex.go                 # codex exec provider
```

## Build

```bash
cd _shared/adversarial-review
go build -o adversarial-review .
```

The compiled binary is gitignored (each user builds locally).

## Usage

```bash
printf '%s' "$ASSEMBLED_PROMPT" | _shared/adversarial-review/adversarial-review
```

Flags:

- `--prompt-file PATH` — read prompt from file instead of stdin
- `--timeout SECONDS` — per-reviewer timeout (default 300)
- `--quiet` — suppress provider heartbeat lines on stderr

See [SKILL.md](SKILL.md) for the contract (input requirements, output JSON
shape, merge rule, when to use, when not to use).

## Sync with upstream

When `mike-skills/adversarial-review` or `mike-skills/llm-provider` changes,
re-vendor:

```bash
_shared/adversarial-review/sync.sh             # interactive (diffs + prompts)
_shared/adversarial-review/sync.sh --check     # exit 1 if drift detected
_shared/adversarial-review/sync.sh --apply     # overwrite without prompt
```

The script honors `MIKE_SKILLS_DIR` if your upstream checkout isn't at
`~/dev/mike-skills`.

## Tests

```bash
go test ./...
```

Tests cover the pure logic (JSON parsing, merge rule, issue dedup); they do
NOT invoke the Claude or Codex CLIs (and so don't burn tokens or require
either CLI to be installed for CI).
