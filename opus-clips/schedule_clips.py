#!/usr/bin/env python3
"""Compute Opus clip posting schedule from config.json + approved clip count.

Each approved clip fans out to ALL connected channels in a single Opus schedule
action (the per-clip "Schedule post" modal posts to every selected platform at
the same time). So one slot per clip, advancing through daily_time_slots.

Hard cap: `posts_per_day_cap` in config (default 5). User has confirmed
diminishing returns beyond 5 videos/day per channel. The schedule rolls to the
next day once `len(daily_time_slots)` is reached. If `--pre-scheduled N` is
passed (indicating slots already consumed on start-date), clips shift to later
slots to avoid exceeding the cap.

Usage:
  ./schedule_clips.py --n-clips 20
  ./schedule_clips.py --n-clips 20 --start-date 2026-04-20
  ./schedule_clips.py --n-clips 3 --pre-scheduled 2  # day-1 has 2 slots taken already
  ./schedule_clips.py --n-clips 3 --dry-run
  ./schedule_clips.py --help

Output: JSON array of {clip_index, day_offset, time, datetime_local, datetime_iso}.
clip_index is 1-based.
"""
from __future__ import annotations

import argparse
import json
import pathlib
import sys
from datetime import date, datetime, time, timedelta
from zoneinfo import ZoneInfo

HERE = pathlib.Path(__file__).parent


def load_config(path: pathlib.Path) -> dict:
    return json.loads(path.read_text())


def build_schedule(
    n_clips: int,
    slots: list[str],
    tz_name: str,
    start: date,
    pre_scheduled_day0: int = 0,
    cap: int | None = None,
) -> list[dict]:
    """Assign clips to (day, slot) pairs.

    pre_scheduled_day0: slots already taken on start-date (shift day-0 clips later).
    cap: hard per-day cap (defaults to len(slots)). Must equal daily_time_slots length —
         enforcing diminishing-returns policy.
    """
    tz = ZoneInfo(tz_name)
    if cap is None:
        cap = len(slots)
    if cap > len(slots):
        raise ValueError(f"posts_per_day_cap ({cap}) cannot exceed slots/day ({len(slots)})")

    out = []
    # absolute slot index across days, offset by pre_scheduled slots on day 0
    for i in range(n_clips):
        absolute = i + pre_scheduled_day0
        day_offset = absolute // cap
        slot_idx = absolute % cap
        hh, mm = slots[slot_idx].split(":")
        local_dt = datetime.combine(
            start + timedelta(days=day_offset),
            time(int(hh), int(mm)),
            tzinfo=tz,
        )
        out.append({
            "clip_index": i + 1,
            "day_offset": day_offset,
            "time": slots[slot_idx],
            "datetime_local": local_dt.isoformat(),
            "datetime_iso": local_dt.astimezone(ZoneInfo("UTC")).isoformat(),
        })
    return out


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser(
        description="Compute Opus posting schedule from config.json.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )
    p.add_argument("--n-clips", type=int, required=True, help="Number of approved clips to schedule")
    p.add_argument("--start-date", type=str, default=None, help="YYYY-MM-DD; defaults to tomorrow local")
    p.add_argument("--pre-scheduled", type=int, default=0, help="Slots already taken on start-date (shift day-0 clips later)")
    p.add_argument("--config", type=pathlib.Path, default=HERE / "config.json", help="Path to config.json")
    p.add_argument("--dry-run", action="store_true", help="No-op (this script has no side effects)")
    args = p.parse_args(argv)

    if args.n_clips < 1:
        print("--n-clips must be >= 1", file=sys.stderr)
        return 2
    if args.pre_scheduled < 0:
        print("--pre-scheduled must be >= 0", file=sys.stderr)
        return 2

    cfg = load_config(args.config)
    tz_name = cfg.get("timezone", "America/Los_Angeles")
    slots = cfg.get("daily_time_slots", ["09:00", "12:00", "15:00", "18:00", "21:00"])
    cap = cfg.get("posts_per_day_cap", len(slots))

    if args.start_date:
        start = date.fromisoformat(args.start_date)
    else:
        start = datetime.now(ZoneInfo(tz_name)).date() + timedelta(days=1)

    if args.pre_scheduled >= cap:
        print(f"--pre-scheduled ({args.pre_scheduled}) >= cap ({cap}); shift start-date forward", file=sys.stderr)
        return 2

    schedule = build_schedule(args.n_clips, slots, tz_name, start, pre_scheduled_day0=args.pre_scheduled, cap=cap)
    json.dump(schedule, sys.stdout, indent=2)
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
