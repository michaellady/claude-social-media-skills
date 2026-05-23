#!/usr/bin/env bash
# Pull the latest fan-out logic (converge/go/internal/fanout) and llm-provider
# source from the upstream mike-skills repo into this vendored copy. Diffs
# first; prompts before overwriting so unintended drift is visible.
#
# Background: the standalone `adversarial-review` skill was folded into
# `converge` as its `audit` mode. The audit fan-out now lives at
# `converge/go/internal/fanout/` (package fanout). This vendored copy stays
# self-contained: `internal/fanout/` is synced verbatim from upstream, and the
# root `main.go` (a thin `fanout.Run` wrapper) + `go.mod` are vendored-specific
# and NOT synced.
#
# Usage:
#   _shared/adversarial-review/sync.sh             # interactive (prompts)
#   _shared/adversarial-review/sync.sh --check     # exit 1 if out of sync
#   _shared/adversarial-review/sync.sh --apply     # overwrite without prompt
set -euo pipefail

dir=$(cd "$(dirname "$0")" && pwd)
upstream=${MIKE_SKILLS_DIR:-$HOME/dev/mike-skills}

if [[ ! -d "$upstream" ]]; then
  echo "error: mike-skills not found at $upstream" >&2
  echo "       set MIKE_SKILLS_DIR=/path/to/mike-skills if it lives elsewhere" >&2
  exit 2
fi

mode=interactive
case "${1:-}" in
  --check) mode=check ;;
  --apply) mode=apply ;;
  "") mode=interactive ;;
  *) echo "usage: $0 [--check|--apply]" >&2; exit 2 ;;
esac

# Pairs: <upstream-path>::<vendored-path>
# NOTE: go.mod and main.go are deliberately NOT synced — upstream's fanout is an
# internal package of the converge binary; here it's a standalone module whose
# go.mod replaces ./internal/llm-provider and whose main.go wraps fanout.Run.
# Everything else (fan-out logic + provider transport) stays in lockstep.
pairs=(
  "$upstream/converge/go/internal/fanout/fanout.go::$dir/internal/fanout/fanout.go"
  "$upstream/converge/go/internal/fanout/fanout_test.go::$dir/internal/fanout/fanout_test.go"
  "$upstream/llm-provider/go.mod::$dir/internal/llm-provider/go.mod"
  "$upstream/llm-provider/provider/provider.go::$dir/internal/llm-provider/provider/provider.go"
  "$upstream/llm-provider/claude/claude.go::$dir/internal/llm-provider/claude/claude.go"
  "$upstream/llm-provider/codex/codex.go::$dir/internal/llm-provider/codex/codex.go"
  "$upstream/llm-provider/agent/agent.go::$dir/internal/llm-provider/agent/agent.go"
  "$upstream/llm-provider/agy/agy.go::$dir/internal/llm-provider/agy/agy.go"
)

drift=0
for pair in "${pairs[@]}"; do
  src=${pair%%::*}
  dst=${pair##*::}
  if [[ ! -f "$src" ]]; then
    echo "warn: upstream missing: $src" >&2
    continue
  fi
  if [[ ! -f "$dst" ]] || ! diff -q "$src" "$dst" >/dev/null 2>&1; then
    drift=$((drift + 1))
    echo "drift: $dst"
    if [[ "$mode" == "interactive" ]]; then
      diff -u "$dst" "$src" || true
      read -r -p "overwrite $dst with $src? [y/N] " ans
      if [[ "$ans" == "y" || "$ans" == "Y" ]]; then
        mkdir -p "$(dirname "$dst")"
        cp "$src" "$dst"
        echo "  updated"
      else
        echo "  skipped"
      fi
    elif [[ "$mode" == "apply" ]]; then
      mkdir -p "$(dirname "$dst")"
      cp "$src" "$dst"
      echo "  updated"
    fi
  fi
done

if [[ "$mode" == "check" ]]; then
  if [[ "$drift" -gt 0 ]]; then
    echo "$drift file(s) out of sync with upstream" >&2
    exit 1
  fi
  echo "in sync with $upstream"
fi
