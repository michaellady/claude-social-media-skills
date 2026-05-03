#!/usr/bin/env bash
# Pull the latest SKILL.md and llm-provider source from the upstream
# mike-skills repo into this vendored copy. Diffs first; prompts before
# overwriting so unintended drift is visible.
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
pairs=(
  "$upstream/adversarial-review/SKILL.md::$dir/SKILL.md"
  "$upstream/llm-provider/go.mod::$dir/internal/llm-provider/go.mod"
  "$upstream/llm-provider/provider/provider.go::$dir/internal/llm-provider/provider/provider.go"
  "$upstream/llm-provider/claude/claude.go::$dir/internal/llm-provider/claude/claude.go"
  "$upstream/llm-provider/codex/codex.go::$dir/internal/llm-provider/codex/codex.go"
  "$upstream/llm-provider/agent/agent.go::$dir/internal/llm-provider/agent/agent.go"
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
