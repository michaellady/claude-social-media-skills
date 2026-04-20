#!/usr/bin/env python3
"""Emit the per-clip AI-enhance runbook for Claude to execute via Claude-in-Chrome MCP.

Per-clip sequence (Phase C/D verified 2026-04-19 on project P30420004wHK):
1. Navigate to editor URL
2. Wait for 'AI enhance' sidebar button to render
3. Capture initial duration
4. Click 'AI enhance' → panel opens
5. (optional) Click 'Remove filler words' — direct-action, may no-op silently
6. (optional) Click 'Remove pauses' → SUBPANEL opens with slider + 'Remove (N)'
7.           Click the 'Remove (N)' button in the subpanel to commit pause removal
8. Click 'Save changes' → editor navigates back to /clip/
9. Assert final duration differs from initial (verifies save took effect)

Usage:
  ./process_clips.py --project-id P3041416kZFt --clip-ranks 1,3,5
  ./process_clips.py --project-id P...         --clip-ranks 1        --dry-run
  ./process_clips.py --help

Output schema per step:
  {"step": int, "action": "navigate"|"js"|"mcp"|"assert", "detail": str, "args": {...}}
"""
from __future__ import annotations

import argparse
import json
import pathlib
import sys

HERE = pathlib.Path(__file__).parent


def load_selectors() -> dict:
    return json.loads((HERE / "selectors.json").read_text())


def load_config() -> dict:
    return json.loads((HERE / "config.json").read_text())


def build_plan(project_id: str, clip_ranks: list[int], enhancer: dict, selectors: dict) -> list[dict]:
    base = selectors["_base_url"]
    editor = selectors["clip_editor"]
    save_text = editor["save_button_text"]

    plan: list[dict] = []
    step = 0

    def add(action: str, detail: str, **args):
        nonlocal step
        step += 1
        plan.append({"step": step, "action": action, "detail": detail, "args": args})

    for rank in clip_ranks:
        add(
            "navigate",
            f"[clip #{rank}] Open editor",
            url=f"{base}/editor-ux/{project_id}?clipRank={rank}&editType=normal",
        )
        add(
            "js",
            f"[clip #{rank}] Wait for 'AI enhance' sidebar button + capture initial duration",
            code=(
                "(async () => {"
                "  const start = Date.now();"
                "  while (Date.now() - start < 60000) {"
                "    const b = Array.from(document.querySelectorAll('button'))"
                "      .find(x => x.textContent.trim() === 'AI enhance');"
                "    if (b) {"
                "      const body = document.body.innerText;"
                "      const m = body.match(/(\\d{2}:\\d{2}\\.\\d{2})\\s*\\/\\s*(\\d{2}:\\d{2}\\.\\d{2})/);"
                "      return JSON.stringify({ready: true, initialDuration: m ? m[2] : null});"
                "    }"
                "    await new Promise(r => setTimeout(r, 500));"
                "  }"
                "  return JSON.stringify({ready: false});"
                "})()"
            ),
            save_as="initial_duration",
        )
        add(
            "js",
            f"[clip #{rank}] Click 'AI enhance' to open panel",
            code=(
                "Array.from(document.querySelectorAll('button'))"
                ".find(b => b.textContent.trim() === 'AI enhance').click()"
            ),
        )
        if enhancer.get("remove_filler_words"):
            add(
                "js",
                f"[clip #{rank}] Click 'Remove filler words' (direct-action; may no-op on clips with no detected fillers)",
                code=(
                    "(async () => {"
                    "  const b = Array.from(document.querySelectorAll('button'))"
                    "    .find(x => x.textContent.trim() === 'Remove filler words');"
                    "  if (!b) return 'not_found';"
                    "  b.click();"
                    "  await new Promise(r => setTimeout(r, 2000));"
                    "  return 'clicked';"
                    "})()"
                ),
            )
        if enhancer.get("remove_silences"):
            add(
                "js",
                f"[clip #{rank}] Click 'Remove pauses' → subpanel opens → click 'Remove (N)' to apply",
                code=(
                    "(async () => {"
                    "  const openPauses = Array.from(document.querySelectorAll('button'))"
                    "    .find(x => x.textContent.trim() === 'Remove pauses');"
                    "  if (!openPauses) return JSON.stringify({state: 'pauses_btn_missing'});"
                    "  openPauses.click();"
                    "  await new Promise(r => setTimeout(r, 2000));"
                    "  const removeBtn = Array.from(document.querySelectorAll('button'))"
                    "    .find(x => /^Remove\\s*\\(\\d+\\)$/.test(x.textContent.trim()));"
                    "  if (!removeBtn) return JSON.stringify({state: 'no_pauses_detected'});"
                    "  const count = (removeBtn.textContent.match(/\\d+/) || ['0'])[0];"
                    "  removeBtn.click();"
                    "  await new Promise(r => setTimeout(r, 3000));"
                    "  const body = document.body.innerText;"
                    "  const m = body.match(/(\\d{2}:\\d{2}\\.\\d{2})\\s*\\/\\s*(\\d{2}:\\d{2}\\.\\d{2})/);"
                    "  return JSON.stringify({state: 'applied', count: parseInt(count), newDuration: m ? m[2] : null});"
                    "})()"
                ),
            )
        add(
            "js",
            f"[clip #{rank}] Click '{save_text}' to commit",
            code=(
                f"Array.from(document.querySelectorAll('button'))"
                f".find(b => b.textContent.trim() === {json.dumps(save_text)}).click()"
            ),
        )
        add(
            "js",
            f"[clip #{rank}] Wait for navigation back to /clip/ (save settled)",
            code=(
                "(async () => {"
                "  const start = Date.now();"
                "  while (Date.now() - start < 15000) {"
                "    if (location.pathname.match(/^\\/clip\\/P[A-Za-z0-9]+/) && !location.pathname.includes('editor-ux')) {"
                "      return 'saved';"
                "    }"
                "    await new Promise(r => setTimeout(r, 500));"
                "  }"
                "  return 'save_timeout';"
                "})()"
            ),
        )
    return plan


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser(
        description="Emit per-clip AI-enhance plan for Claude-in-Chrome to execute.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )
    p.add_argument("--project-id", required=True, help="Opus project ID incl. clip hash (e.g. P304....QUAsq9IWyN)")
    p.add_argument("--clip-ranks", required=True, help="Comma-separated 1-based clip ranks (e.g. '1,3,5')")
    p.add_argument("--dry-run", action="store_true",
                   help="Annotate output with 'DRY RUN' tag; stops emitting after first clip for preview")
    args = p.parse_args(argv)

    try:
        ranks = [int(x) for x in args.clip_ranks.split(",") if x.strip()]
    except ValueError:
        print("--clip-ranks must be comma-separated integers", file=sys.stderr)
        return 2
    if not ranks:
        print("--clip-ranks cannot be empty", file=sys.stderr)
        return 2

    cfg = load_config()
    sel = load_selectors()
    enhancer = cfg.get("enhancer_settings", {})
    effective_ranks = ranks[:1] if args.dry_run else ranks
    plan = build_plan(args.project_id, effective_ranks, enhancer, sel)

    out = {
        "project_id": args.project_id,
        "enhancer_settings": enhancer,
        "ranks_requested": ranks,
        "ranks_planned": effective_ranks,
        "dry_run": args.dry_run,
        "steps": plan,
    }
    json.dump(out, sys.stdout, indent=2)
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
