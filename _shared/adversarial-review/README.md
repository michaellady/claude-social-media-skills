# adversarial-review (vendored)

Multi-reviewer adversarial-review transport: dispatches the same prompt to
every selected reviewer CLI in parallel (default
`claude,muse,codex,agy,composer-2.5,grok-build,kimi,glm`; bare `agent`
is opt-in), parses each reviewer's JSON verdict, and emits a merged
canonical response.

The standalone `adversarial-review` skill was **folded into `converge` as its
`audit` mode**. The fan-out logic now lives upstream at
[`mike-skills/converge/go/internal/fanout`](https://github.com/michaellady/mike-skills/tree/main/converge/go/internal/fanout)
and is **vendored here** (plus the shared
[`mike-skills/llm-provider`](https://github.com/michaellady/mike-skills/tree/main/llm-provider)
module that powers the CLI dispatch) so a clone of `claude-social-media-skills`
works end-to-end without a separate `mike-skills` checkout. Invoking this
vendored binary is equivalent to running `converge audit`.

## Layout

Muse uses your existing Muse Code subscription login, with no separate API
key. The verified model defaults on 2026-11-03 are Fable, Muse Wonder 1.0,
GPT-6 Astra Max, Gemini 4 Argon High, Composer 2.5, Grok 4.7 Extra High,
Kimi K3 Max, and GLM 5.2 Max. Wonder Full returned an access
error on the tested account; it is not selected. Explicit provider model
options and `CONVERGE_*_MODEL` overrides remain available.

All reviews use the most advanced verified model in each family by default.
Claude Fable 5.1 and Muse Wonder use maximum reasoning; Codex uses GPT-6 Astra
with maximum reasoning. The optional generic Cursor `agent` reviewer uses
`claude-fable-5-1-thinking-max` instead of Auto. An explicit user request can
still override model or effort.

The vendored build retains upstream's Go module namespace so its internal
fanout and ledger imports compile unchanged. The local provider replacement
keeps it self-contained. Sync includes run persistence, ledger, platform
process cleanup, and Muse sources/tests, not only the original fanout file.

```
_shared/adversarial-review/
  README.md                        # this file
  go.mod                           # binary's module + replace → ./internal/llm-provider  (NOT synced)
  main.go                          # thin wrapper → fanout.Run (vendored-specific, NOT synced)
  sync.sh                          # pull fresh copies from upstream mike-skills
  smoke.sh                         # end-to-end provider smoke
  internal/fanout/                 # vendored from mike-skills/converge/go/internal/fanout/
    fanout.go                      # parallel dispatch + JSON parse + FAIL-OR merge + clustering
    fanout_test.go                 # unit tests for parse + merge + dedup
  internal/llm-provider/           # vendored from mike-skills/llm-provider/
    go.mod
    provider/provider.go           # Provider interface + Options + Error
    claude/claude.go               # claude CLI provider
    codex/codex.go                 # codex exec provider
    agent/agent.go                 # Cursor agent CLI provider (opt-in)
    agy/agy.go                     # agy CLI provider (default; replaced gemini)
    muse/                          # Muse Code subscription provider + tests
  internal/ledger/                 # audit history dependency from upstream
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

- `--reviewers <csv>` — which reviewers to dispatch (nine by default; bare `agent` is opt-in)
- `--prompt-file PATH` — read prompt from file instead of stdin
- `--timeout SECONDS` — per-reviewer timeout (default 300)
- `--quiet` — suppress provider heartbeat lines on stderr

To add the opt-in `agent` reviewer (needs Cursor quota):

```bash
# Select a smaller panel including Muse and the opt-in Cursor auto model
printf '%s' "$ASSEMBLED_PROMPT" | _shared/adversarial-review/adversarial-review \
  --reviewers claude,muse,codex,agy,agent

# Or scope down to a specific subset
printf '%s' "$ASSEMBLED_PROMPT" | _shared/adversarial-review/adversarial-review \
  --reviewers claude,codex
```

For the full contract (input requirements, output JSON shape, merge rule, when
to use / when not to use) see the **`audit` mode** in
[`mike-skills/converge/SKILL.md`](https://github.com/michaellady/mike-skills/blob/main/converge/SKILL.md).

## Sync with upstream

When `mike-skills/converge` (the `internal/fanout` package) or
`mike-skills/llm-provider` changes, re-vendor:

```bash
_shared/adversarial-review/sync.sh             # interactive (diffs + prompts)
_shared/adversarial-review/sync.sh --check     # exit 1 if drift detected
_shared/adversarial-review/sync.sh --apply     # overwrite without prompt
```

`main.go` and `go.mod` are vendored-specific and intentionally not synced. The
script honors `MIKE_SKILLS_DIR` if your upstream checkout isn't at
`~/dev/mike-skills`.

## Tests

```bash
go test ./...                         # pure logic only — fast, no CLIs invoked
./smoke.sh                            # end-to-end against every CLI provider (burns a few cents)
./smoke.sh claude codex               # specific N-way combo
```

Pure-logic Go tests cover JSON parsing, merge rule, and issue dedup. They
do NOT invoke the actual `claude` / `codex` / `agent` / `agy` CLIs
(so they don't burn tokens or require any CLI installed for CI). End-to-end
provider smoke is a separate `./smoke.sh` — required after every change to
provider code or after upgrading a provider CLI.
