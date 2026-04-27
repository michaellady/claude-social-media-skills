#!/usr/bin/env bash
# Edit a Buffer channel's posting schedule and/or weekly goal via publish.buffer.com web UI.
#
# Usage:
#   buffer-schedule-edit.sh <channelId> <schedule.json>           # schedule only
#   buffer-schedule-edit.sh <channelId> <schedule.json> <goal>    # schedule + numeric goal
#   buffer-schedule-edit.sh <channelId> --goal-only <goal>        # goal only, leave slots alone
#
# schedule.json shape: {"mon": ["09:30","13:30","18:30"], "tue": [...], ...}
# All times in 24h, channel-local timezone.
set -euo pipefail

CHANNEL_ID="$1"
MODE="schedule"
SCHEDULE_JSON=""
GOAL=""

if [ "${2:-}" = "--goal-only" ]; then
  MODE="goal_only"
  GOAL="${3:-}"
  if [ -z "$GOAL" ]; then echo "missing goal value" >&2; exit 2; fi
else
  SCHEDULE_JSON="$2"
  GOAL="${3:-}"
fi

B="${B:-$HOME/.claude/skills/gstack/browse/dist/browse}"

# JS helpers we'll inject repeatedly
JS_CLICK_FN='
window.__radixClick = (el) => {
  const opts = {bubbles: true, cancelable: true, view: window, button: 0, pointerType: "mouse"};
  el.dispatchEvent(new PointerEvent("pointerdown", opts));
  el.dispatchEvent(new MouseEvent("mousedown", opts));
  el.dispatchEvent(new PointerEvent("pointerup", opts));
  el.dispatchEvent(new MouseEvent("mouseup", opts));
  el.dispatchEvent(new MouseEvent("click", opts));
};
window.__pickMenu = (testid, label) => {
  const btn = document.querySelector("[data-testid=" + testid + "]");
  if (!btn) return "NO_BTN:" + testid;
  window.__radixClick(btn);
  return "OPENED:" + testid;
};
window.__pickItem = (label) => {
  const menus = document.querySelectorAll("[role=menu], [role=listbox]");
  if (menus.length === 0) return "NO_MENU";
  const items = [...menus[menus.length-1].querySelectorAll("[role=menuitem],[role=menuitemradio],[role=option]")];
  const item = items.find(i => i.textContent.trim() === label);
  if (!item) return "NO_ITEM:" + label + ":[" + items.map(i=>i.textContent.trim()).slice(0,12).join(",") + "]";
  window.__radixClick(item);
  return "PICKED:" + label;
};
window.__setGoal = (val) => {
  const heading = [...document.querySelectorAll("h1,h2,h3,h4")].find(h => h.textContent.trim().toLowerCase().includes("posting goal"));
  if (!heading) return "NO_GOAL_HEADING";
  let section = heading.parentElement;
  for (let i = 0; i < 4; i++) { if (section.querySelector("input")) break; section = section.parentElement; }
  const input = section.querySelector("input");
  if (!input) return "NO_GOAL_INPUT";
  const proto = Object.getPrototypeOf(input);
  const setter = Object.getOwnPropertyDescriptor(proto, "value").set;
  setter.call(input, String(val));
  input.dispatchEvent(new Event("input", {bubbles: true}));
  input.dispatchEvent(new Event("change", {bubbles: true}));
  input.blur();
  return "SET:" + input.value;
};
'

run_js() {
  "$B" js "$JS_CLICK_FN $1" 2>/dev/null
}

navigate() {
  "$B" goto "https://publish.buffer.com/channels/${CHANNEL_ID}/settings" >/dev/null 2>&1
  sleep 4
}

clear_all() {
  echo "  → Clear All..."
  run_js 'window.__radixClick(document.querySelector("[data-testid=posting-schedule-clear-all-trigger]"));'
  sleep 1
  run_js 'window.__radixClick(document.querySelector("[data-testid=posting-schedule-clear-all-confirm]"));'
  sleep 2
}

set_goal() {
  local val="$1"
  echo "  → Set posting goal: $val"
  local r=$(run_js "window.__setGoal($val);")
  echo "    $r"
  sleep 2  # auto-save propagation
}

# Convert "09:30" → ("Monday"-style day, "09", "30", "AM"/"PM")
add_slot() {
  local day_label="$1"   # "Monday", "Tuesday", ...
  local time24="$2"      # "09:30"
  local h=$(echo "$time24" | cut -d: -f1 | sed 's/^0//')
  local m=$(echo "$time24" | cut -d: -f2)
  local ampm
  local h12
  if [ "$h" -eq 0 ]; then
    h12="12"; ampm="AM"
  elif [ "$h" -lt 12 ]; then
    h12=$(printf "%02d" "$h"); ampm="AM"
  elif [ "$h" -eq 12 ]; then
    h12="12"; ampm="PM"
  else
    h12=$(printf "%02d" $((h-12))); ampm="PM"
  fi

  # 1. Day picker
  run_js 'window.__pickMenu("postingtime-form-days-selector");'
  sleep 0.5
  local r=$(run_js "window.__pickItem('$day_label');")
  if [[ "$r" != PICKED:* ]]; then echo "    DAY FAIL: $r"; return 1; fi
  sleep 0.4

  # 2. Hours picker
  run_js 'window.__pickMenu("postingtime-form-hours-selector");'
  sleep 0.5
  r=$(run_js "window.__pickItem('$h12');")
  if [[ "$r" != PICKED:* ]]; then echo "    HOUR FAIL: $r"; return 1; fi
  sleep 0.4

  # 3. Minutes
  run_js 'window.__pickMenu("postingtime-form-minutes-selector");'
  sleep 0.5
  r=$(run_js "window.__pickItem('$m');")
  if [[ "$r" != PICKED:* ]]; then echo "    MIN FAIL: $r"; return 1; fi
  sleep 0.4

  # 4. AM/PM
  run_js 'window.__pickMenu("postingtime-form-am-pm-selector");'
  sleep 0.5
  r=$(run_js "window.__pickItem('$ampm');")
  if [[ "$r" != PICKED:* ]]; then echo "    AMPM FAIL: $r"; return 1; fi
  sleep 0.4

  # 5. Submit
  run_js 'window.__radixClick(document.querySelector("[data-testid=postingtime-form-submit-button]"));'
  sleep 0.6
  echo "    + ${day_label} ${h12}:${m} ${ampm}"
}

# Map short day → full day name
day_full() {
  case "$1" in
    mon) echo "Monday";;
    tue) echo "Tuesday";;
    wed) echo "Wednesday";;
    thu) echo "Thursday";;
    fri) echo "Friday";;
    sat) echo "Saturday";;
    sun) echo "Sunday";;
  esac
}

navigate

if [ "$MODE" = "goal_only" ]; then
  set_goal "$GOAL"
  echo "  → Done."
  exit 0
fi

clear_all

for day in mon tue wed thu fri sat sun; do
  TIMES=$(jq -r ".$day[]?" "$SCHEDULE_JSON")
  if [ -z "$TIMES" ]; then continue; fi
  for t in $TIMES; do
    add_slot "$(day_full "$day")" "$t"
  done
done

if [ -n "$GOAL" ]; then
  set_goal "$GOAL"
fi

echo "  → Done. Verifying..."
sleep 2
"$B" js '(() => {
  const wrap = document.querySelector("[data-testid=posting-schedule-wrapper]");
  return JSON.stringify({slotCount: wrap.querySelectorAll("[data-testid=schedule-table-cell-remove-button]").length});
})()' 2>/dev/null
