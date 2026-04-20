#!/usr/bin/env python3
"""Emit a verification plan that confirms the expected scheduled posts appear on Opus's Calendar.

Phase D (2026-04-19) proved: after clicking `Schedule` in the per-clip modal,
Opus commits 6 posts silently (no toast). The ONLY way to confirm success is
to navigate `/auto-post/calendar`, expand the day cell, and count the `Scheduled`
tokens + their grouped times.

This script takes the same inputs as `schedule_clips.py` (n-clips + start-date)
and emits a JSON plan the skill runner executes. Compares actual vs expected;
exits non-zero (in the emitted plan semantics) on any mismatch.

Usage:
  ./verify_schedule.py --n-clips 3 --start-date 2026-04-20
  ./verify_schedule.py --n-clips 3 --start-date 2026-04-20 --channels 6

Output schema:
  {
    "expected": {"2026-04-20": {"09:00 AM": 6, "12:00 PM": 6, "03:00 PM": 6}, ...},
    "steps": [ {step, action, detail, args}, ... ]
  }
"""
from __future__ import annotations

import argparse
import json
import pathlib
import sys
from datetime import date, datetime, timedelta
from zoneinfo import ZoneInfo

HERE = pathlib.Path(__file__).parent


def load_config() -> dict:
    return json.loads((HERE / "config.json").read_text())


def load_selectors() -> dict:
    return json.loads((HERE / "selectors.json").read_text())


def to_12h(hhmm: str) -> str:
    """24h '15:00' → 12h '3:00 PM' (match Opus calendar display format)."""
    h, m = (int(x) for x in hhmm.split(":"))
    suffix = "AM" if h < 12 else "PM"
    h12 = h % 12 or 12
    return f"{h12}:{m:02d} {suffix}"


def build_expected(n_clips: int, start: date, slots: list[str], channels: int, cap: int) -> dict[str, dict[str, int]]:
    """Map calendar-day-label → {12h-time → expected_count}."""
    out: dict[str, dict[str, int]] = {}
    for i in range(n_clips):
        day_offset = i // cap
        slot_idx = i % cap
        day = (start + timedelta(days=day_offset))
        # Opus calendar shows "Apr 20" format (no leading zero)
        day_label = f"{day.strftime('%b')} {day.day}"
        time_label = to_12h(slots[slot_idx])
        out.setdefault(day_label, {})
        out[day_label][time_label] = out[day_label].get(time_label, 0) + channels
    return out


def build_plan(expected: dict[str, dict[str, int]], selectors: dict) -> list[dict]:
    base = selectors["_base_url"]
    plan: list[dict] = []
    step = 0

    def add(action: str, detail: str, **args):
        nonlocal step
        step += 1
        plan.append({"step": step, "action": action, "detail": detail, "args": args})

    add("navigate", "Open Calendar", url=f"{base}/auto-post/calendar")
    add(
        "js",
        "Wait for calendar to render",
        code=(
            "(async () => {"
            "  const start = Date.now();"
            "  while (Date.now() - start < 15000) {"
            "    if (/Calendar/.test(document.body.innerText) && /April|May|June/.test(document.body.innerText)) return 'ready';"
            "    await new Promise(r => setTimeout(r, 500));"
            "  }"
            "  return 'timeout';"
            "})()"
        ),
    )
    for day_label, expected_times in expected.items():
        add(
            "js",
            f"Expand '{day_label}' cell if collapsed",
            code=(
                "(() => {"
                f"  const want = {json.dumps(day_label)};"
                "  const cells = Array.from(document.querySelectorAll('div')).filter(d => {"
                "    const t = (d.textContent||'').trim();"
                "    return t.length > 20 && t.length < 5000 && t.startsWith(want) && t.includes('See ') && t.includes(' more');"
                "  }).sort((a,b) => a.textContent.length - b.textContent.length);"
                "  if (!cells.length) return JSON.stringify({state: 'no_collapsed_cell', day: want});"
                "  const cell = cells[0];"
                "  cell.scrollIntoView({block: 'center'});"
                "  const expand = Array.from(cell.querySelectorAll('div')).find(d => /^See \\d+ more$/.test((d.textContent||'').trim()));"
                "  if (!expand) return JSON.stringify({state: 'no_expand_link', day: want});"
                "  expand.click();"
                "  return JSON.stringify({state: 'expanded', day: want});"
                "})()"
            ),
        )
        add(
            "js",
            f"Count Scheduled posts + times in '{day_label}' cell",
            code=(
                "(() => {"
                f"  const want = {json.dumps(day_label)};"
                "  const body = document.body.innerText;"
                "  const startIdx = body.indexOf(want);"
                "  if (startIdx < 0) return JSON.stringify({error: 'day_not_in_body', day: want});"
                "  // Find the next day label to bound the slice"
                "  const monthDayRe = /\\n(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)\\s+\\d{1,2}\\b/g;"
                "  monthDayRe.lastIndex = startIdx + want.length;"
                "  const next = monthDayRe.exec(body);"
                "  const endIdx = next ? next.index : body.length;"
                "  const snippet = body.slice(startIdx, endIdx);"
                "  const scheduled = (snippet.match(/Scheduled/g) || []).length;"
                "  const times = [...snippet.matchAll(/(\\d{1,2}:\\d{2})\\s*(AM|PM)/g)].map(m => m[0]);"
                "  const grouped = {};"
                "  times.forEach(t => { grouped[t] = (grouped[t] || 0) + 1; });"
                "  return JSON.stringify({day: want, scheduled, grouped});"
                "})()"
            ),
            assertion={"day": day_label, "expected_times": expected_times, "expected_total": sum(expected_times.values())},
        )
    return plan


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser(
        description="Emit a schedule-verification plan for Claude-in-Chrome.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )
    p.add_argument("--n-clips", type=int, required=True, help="Number of clips that were scheduled")
    p.add_argument("--start-date", type=str, required=True, help="YYYY-MM-DD — the first day of scheduled posts")
    p.add_argument("--channels", type=int, default=None, help="Expected posts per slot (default: count in config.channels)")
    args = p.parse_args(argv)

    cfg = load_config()
    sel = load_selectors()
    slots = cfg.get("daily_time_slots", ["09:00", "12:00", "15:00", "18:00", "21:00"])
    cap = cfg.get("posts_per_day_cap", len(slots))
    channel_count = args.channels if args.channels is not None else len(cfg.get("channels", []))
    if channel_count < 1:
        print("error: no channels configured and --channels not supplied", file=sys.stderr)
        return 2

    start = date.fromisoformat(args.start_date)
    expected = build_expected(args.n_clips, start, slots, channel_count, cap)
    plan = build_plan(expected, sel)

    out = {
        "n_clips": args.n_clips,
        "start_date": args.start_date,
        "channels_per_slot": channel_count,
        "expected": expected,
        "total_expected_posts": args.n_clips * channel_count,
        "steps": plan,
    }
    json.dump(out, sys.stdout, indent=2)
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
