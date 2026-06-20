---
name: instagram-follower-identification
description: Use when the user wants to identify who is behind a suspicious / anonymous Instagram follower on their main account — "who is this fake follower", "identify this burner account", "deanonymize instagram follower", "find out who this blank account really is". Browser-scrapes the follower modal via claude-in-chrome (NO API), driven from an ALT account so the technique is non-destructive — same interactive pattern as tiktok-stats.
user_invocable: true
---

# instagram-follower-identification

Identify the real person behind a suspicious blank/anonymous account that follows the user's **main** Instagram, by exploiting two Instagram behaviors and diffing the main account's follower list before and after a block.

The whole run is driven from a disposable **alt** account that acts as **both the observer and the blocker**, so the main account's real follow relationships are never touched. The skill **observes and diffs**; the **user performs the block manually** so the irreversible action stays under human control.

## How it works (the two load-bearing behaviors)

1. **Block cascade.** When you block someone, Instagram offers *"Block [user] and other accounts they may have or create."* Choosing it also blocks the accounts Instagram has linked to that person via shared signals — email, device, IP, behavioral patterns. So blocking the burner also blocks the suspect's *real* account, **if** Instagram linked them.
2. **Two-way block invisibility.** A blocked account is hidden not only from the blocked person's view of you, but from **your** (the blocker's) view of **them** — including when you browse a *third party's* follower list. So after the alt blocks the burner + its linked accounts, those accounts vanish from the alt's view of main's follower list.

Diff the alt's view of main's followers (before − after), drop the burner itself, and the remaining vanished accounts are the linked/suspect identity.

## ⚠️ Read before running

- **Run as the ALT, not main.** The observing account and the blocking account must be the *same* account, and that account must be the disposable alt. Blocking from main works too but is **destructive** — blocked followers are dropped permanently (unblock does NOT restore the follow) and the suspect is tipped off. The alt path avoids all of that. This skill refuses to proceed if it detects it's logged in as `main_handle` (`Edge: wrong-account`).
- **The cascade only fires if Instagram linked the accounts.** A careful burner (separate email, separate device, never co-logged-in) may not be linked at all. Then the diff shows only the burner and the result is **inconclusive** — that's the ceiling of the technique, not a bug.
- **Block invisibility in third-party follower lists is app-version-dependent.** Phase 2 validates it empirically with a known account *before* spending the real block, so we never trust the assumption blindly.
- **Privacy / interactive only.** This reads the user's own follower data from their own logged-in session for their own privacy. There is no API/headless path; if a login wall appears, hand off to the user — never log in programmatically.

## Usage

`/instagram-follower-identification` — full run (pre-check → snapshot → guided block → snapshot → diff)
`/instagram-follower-identification --burner <handle>` — name the suspicious account up front (otherwise the skill asks)
`/instagram-follower-identification --skip-precheck` — skip Phase 2 (only if block-invisibility was already validated this session; not recommended)

## 🟢 Happy Path (read first; everything below is edge-case detail)

A full run, ~2–10 min wall-clock (dominated by scrolling a large follower list twice). Each step links to a labeled edge case (`Edge: <name>`) you only read if that step fails.

**Phase 0 — Load config + chrome tools.** Read `config.local.json` if present, else `config.json`. Pull `main_handle`, `main_followers_url`, `alt_handle`, `max_followers_to_scrape`, `scroll_settle_ms`. Load the chrome tools (Phase 0 below). Get the burner handle from `--burner` or ask the user.

**Phase 1 — Browser + login check (as the ALT).** Confirm a connected browser, navigate to `main_followers_url`, read the page. Confirm the session is logged in **as the alt** (not main, not logged out). If main is followed/public, the alt can see its follower modal. Login wall or wrong account → hand off (`Edge: login-wall`, `Edge: wrong-account`).

**Phase 2 — Mechanism pre-check (validate block-invisibility).** Ask the user to pick a *known* account that currently follows main (a throwaway or a second test account they control). From the alt, the user blocks it (plain block is fine here), then the skill re-reads main's follower list from the alt and confirms that account **disappeared**. Then the user **unblocks** it. If it disappeared → mechanism confirmed, proceed. If not → **stop**; the alt technique doesn't work on this app version (`Edge: precheck-fails`). Do NOT silently fall back to destructive main-blocking.

**Phase 3 — Snapshot BEFORE.** Scrape main's full follower list from the alt into `cache/followers-before-<timestamp>.json` — an array of `{username, full_name}`. The list is a **virtualized modal**: scroll in modest steps, read after each, dedupe by username, until the set stops growing or `max_followers_to_scrape` is hit (`Edge: virtualized-skips`, `Edge: list-too-large`).

**Phase 4 — Guided block (USER action).** Instruct the user, from the **alt**: open the burner's profile → **⋯** → **Block** → choose **"Block [burner] and other accounts they may have or create"** → confirm. Wait for the user to confirm "done". The skill does not automate this.

**Phase 5 — Snapshot AFTER.** Re-scrape main's follower list from the alt (same recipe as Phase 3) into `cache/followers-after-<timestamp>.json`.

**Phase 6 — Diff + report.** Compute `before − after` (usernames present before, absent after), drop the burner handle itself → candidate linked accounts. Write `cache/diff-<timestamp>.md` and report. Interpret per the rules below.

### Edge labels (jump to these only when you hit the matching failure signal)

| Label | Symptom |
|---|---|
| `Edge: login-wall` | Instagram shows a login / "Log in" screen instead of the profile/followers modal |
| `Edge: wrong-account` | The logged-in account is `main_handle` (or logged out) instead of the alt |
| `Edge: precheck-fails` | The known test account did NOT disappear from main's follower list after the alt blocked it |
| `Edge: virtualized-skips` | Scraped far fewer followers than the profile's stated count, or rows missing from the middle |
| `Edge: list-too-large` | Follower count is huge; the modal won't fully paginate before rate-limiting |
| `Edge: action-block` | Instagram throws "Action Blocked" / "Try Again Later" mid-scroll |
| `Edge: no-diff` | After−before diff is empty (only the burner vanished, or nothing vanished) |
| `Edge: burner-gone` | The burner already isn't in the before-snapshot (unfollowed / deactivated / private) |

Each label corresponds to a heading in **Known issues / robustness notes** below.

## Config

The skill reads config from (in priority order):
1. `~/dev/claude-social-media-skills/instagram-follower-identification/config.local.json` (gitignored — put personal overrides here, including `alt_handle`)
2. `~/dev/claude-social-media-skills/instagram-follower-identification/config.json` (committed defaults)

Fields:
- `main_handle` — `@`-stripped handle of the account being investigated (`mikelady`)
- `main_profile_url` — full public profile URL of main
- `main_followers_url` — the follower modal target (`https://www.instagram.com/<main>/followers/`)
- `alt_handle` — `@`-stripped handle of the disposable account you run this from (set in `config.local.json`; leave `""` in committed config). Used only to confirm you're NOT logged in as main.
- `max_followers_to_scrape` — hard cap on rows scraped (default 5000)
- `scroll_settle_ms` — pause after each scroll so the virtualized list can render (default 1200)

Load config at the start of every run:

```bash
CONFIG_DIR=~/dev/claude-social-media-skills/instagram-follower-identification
if [ -f "$CONFIG_DIR/config.local.json" ]; then CONFIG_FILE="$CONFIG_DIR/config.local.json"; else CONFIG_FILE="$CONFIG_DIR/config.json"; fi
MAIN_HANDLE=$(jq -r .main_handle "$CONFIG_FILE")
FOLLOWERS_URL=$(jq -r .main_followers_url "$CONFIG_FILE")
ALT_HANDLE=$(jq -r '.alt_handle // ""' "$CONFIG_FILE")
MAX_FOLLOWERS=$(jq -r '.max_followers_to_scrape // 5000' "$CONFIG_FILE")
SETTLE_MS=$(jq -r '.scroll_settle_ms // 1200' "$CONFIG_FILE")
```

## Process

### Phase 0 — Load config + chrome tools

The chrome tools are MCP tools that must be loaded before use. Load them first:

```
ToolSearch select:mcp__claude-in-chrome__tabs_context_mcp,mcp__claude-in-chrome__navigate,mcp__claude-in-chrome__get_page_text,mcp__claude-in-chrome__computer,mcp__claude-in-chrome__list_connected_browsers
```

**Use `get_page_text`, not screenshots** — the follower modal renders usernames + display names as text; `get_page_text` captures them exactly for diffing, while screenshots would force OCR and lose precision.

Resolve the burner handle: use `--burner <handle>` if given, otherwise ask the user "Which account do you want to identify? Paste its @handle." Normalize to the bare handle (strip `@` and any URL wrapper).

### Phase 1 — Connect browser + verify login (as the ALT)

```
mcp__claude-in-chrome__list_connected_browsers   # confirm a browser is connected; if none, ask the user to open Chrome
mcp__claude-in-chrome__tabs_context_mcp          # see open tabs
mcp__claude-in-chrome__navigate → $FOLLOWERS_URL # go to instagram.com/<main>/followers/
mcp__claude-in-chrome__get_page_text             # read the page
```

Logged-in markers: the followers modal/list with usernames and "Follow"/"Following" buttons; the nav bar avatar. Not-logged-in markers: "Log in", "Sign up", a login form.

**Confirm the account identity.** Read the logged-in account (nav avatar / account menu). It MUST be the alt:
- If it's `$MAIN_HANDLE` → **stop** and warn the user. Running as main makes the block destructive and pollutes the diff with the alt-vs-main visibility you want. Ask them to switch to the alt. (`Edge: wrong-account`.)
- If logged out / login wall → hand off, don't log in programmatically:
  > "Instagram needs you logged in as your alt account in this Chrome window. Please log into the alt at instagram.com, then tell me to continue."
  (`Edge: login-wall`.)
- If `$ALT_HANDLE` is set in config and the logged-in handle doesn't match it either, confirm with the user before proceeding (they may use a different alt than configured).

Also confirm the alt can actually *see* main's follower list (main is public, or the alt follows main). If the modal is empty/locked, ask the user to make sure the alt follows main first.

### Phase 2 — Mechanism pre-check (validate block-invisibility)

This de-risks the load-bearing assumption that a blocked account disappears from a third-party follower list *as the blocker sees it*, on the current app version. **Do not skip** unless `--skip-precheck` is explicitly passed.

1. Ask the user to name a **known control account** that currently follows main and that they're willing to block/unblock — ideally a throwaway or a second account they own. Confirm it appears in the current follower modal read (it must be present for the test to mean anything).
2. Ask the user, from the **alt**, to **block** that control account (a plain block is enough for the test) and confirm done.
3. Re-navigate to `$FOLLOWERS_URL` and re-read with `get_page_text`. Check whether the control account is now **absent** from the alt's view of main's followers.
4. Ask the user to **unblock** the control account (restore state).

- Control account **disappeared** → block-invisibility works here. Proceed to Phase 3.
- Control account **still visible** → `Edge: precheck-fails`. **Stop.** Report that the non-destructive alt technique won't work on this Instagram version and ask the user how to proceed (the only working alternative is the destructive main-account block, which needs explicit opt-in).

### Phase 3 — Snapshot BEFORE (scrape main's follower list from the alt)

The follower modal at `instagram.com/<main>/followers/` is a **virtualized list** — only rows near the viewport exist in the DOM. Same discipline as tiktok-stats' content table:

1. **Scroll in modest steps.** A jump to the bottom skips un-rendered rows. Scroll roughly one modal-height at a time, pausing `$SETTLE_MS` after each scroll so rows render.
2. **Dedupe by username as you accumulate** — the virtualized window overlaps between steps, so the same row reappears.

```
# Pseudocode — you drive the tool calls; the engine is your accumulating set:
seen = {}                       # key: username → value: {username, full_name}
prev_count = -1
while len(seen) < MAX_FOLLOWERS:
    text = get_page_text()      # read currently-rendered rows
    for each follower row parsed from `text`:
        if row.username not in seen: seen[row.username] = {username, full_name}
    if len(seen) == prev_count: break    # no new rows two reads running → bottom reached
    prev_count = len(seen)
    computer(action="scroll", direction="down", amount=modest)   # scroll the MODAL, ~one modal-height
    # wait scroll_settle_ms before the next get_page_text
```

Scroll the **modal's** scroll container (not the page body) — the follower list scrolls independently. Parse each row's `username` (the `@handle`) and `full_name` (display name, may be empty). Write the accumulated set to:

```bash
CACHE_DIR=~/dev/claude-social-media-skills/instagram-follower-identification/cache
mkdir -p "$CACHE_DIR"
TS=$(date -u +%Y-%m-%dT%H-%M-%SZ)
# write cache/followers-before-$TS.json  (see schema below)
```

Sanity-check the captured count against the profile's stated follower number (read from the profile header). A large shortfall means you scrolled too fast or hit a wall — see `Edge: virtualized-skips` / `Edge: action-block`. Confirm the **burner is present** in this before-snapshot; if not, see `Edge: burner-gone`.

### Phase 4 — Guided block (USER performs it)

Print explicit instructions and wait — the skill never automates the block:

> From your **alt** account, in the Instagram app or web:
> 1. Open the burner's profile: `instagram.com/<burner>/`
> 2. Tap **⋯** (top right) → **Block**
> 3. Choose **"Block [burner] and other accounts they may have or create"** — this is the option that triggers the cascade. (If only a plain "Block" is offered, Instagram has no linked accounts on file for this user — the result will likely be inconclusive.)
> 4. Confirm.
> Tell me when it's done.

Note to the user: this block lives on the disposable alt, so nothing on main is affected; you can unblock from the alt afterward with no impact on main's followers.

### Phase 5 — Snapshot AFTER

Re-navigate to `$FOLLOWERS_URL` and repeat the exact Phase 3 scrape into `cache/followers-after-$TS.json`. Use the **same scroll discipline** — an under-scrolled after-snapshot creates false "disappearances". If the two snapshots' total counts differ by far more than expected (≈1 burner + a few linked accounts), suspect an incomplete scrape and re-run Phase 5 before trusting the diff.

### Phase 6 — Diff + report

Compute the set difference and drop the burner itself:

```bash
BEFORE=cache/followers-before-$TS.json
AFTER=cache/followers-after-$TS.json
BURNER="<burner-handle>"

# usernames present BEFORE but absent AFTER, excluding the burner = candidate linked accounts
jq -n --slurpfile b "$BEFORE" --slurpfile a "$AFTER" --arg burner "$BURNER" '
  ($a[0].followers | map(.username)) as $after_names
  | $b[0].followers
  | map(select(.username as $u | ($after_names | index($u)) | not))
  | map(select(.username != $burner))
' > cache/diff-$TS.json
```

Render a short report and write `cache/diff-$TS.md`:

```
Instagram follower identification — <main> (timestamp)

Burner investigated: @<burner>
Block invisibility pre-check: PASSED
Followers scraped: N before / M after

Candidate linked accounts (vanished from main's followers after the alt blocked the burner, excluding the burner itself):
  @candidate1   "Display Name"
  @candidate2   "Display Name"
  …

Read this as: <interpretation per the rules below>
```

**Interpretation rules:**
- **Exactly one candidate** → strongest signal: that account is very likely the same person as the burner (Instagram linked them). Present it as a strong lead, not proof.
- **Multiple candidates** → all are linked to the burner by Instagram; the person may run several. Present all; the user recognizes the real one.
- **Zero candidates** (`Edge: no-diff`) → inconclusive. Either Instagram never linked the burner to a real account (careful burner), or the cascade had nothing to cascade to. Not a failure of the run — the ceiling of the technique.
- Always caveat: a vanished account *could* be coincidental (someone who deactivated, went private, or blocked the alt during the run). The tighter the before/after window, the less likely.

## Snapshot schema

`cache/followers-before-*.json` and `cache/followers-after-*.json`:

```jsonc
{
  "fetched_at": "2026-06-20T19:00:00Z",   // ISO-8601 UTC when scraped
  "platform": "instagram",
  "observed_from": "alt",                  // always the alt (the observer = the blocker)
  "main_handle": "mikelady",
  "source": "instagram_followers_modal_scrape",
  "stated_follower_count": 2800,           // from the profile header; null if not read
  "scraped_count": 2798,                   // how many distinct rows we actually captured
  "followers": [
    { "username": "someone", "full_name": "Some One" },
    { "username": "blank_acct_2020", "full_name": "" }
    // … one record per distinct follower
  ]
}
```

`cache/diff-*.json` is the array of candidate `{username, full_name}` records (before − after − burner).

## Known issues / robustness notes

- **Wrong account.** Running as main makes the block destructive and breaks the logic (the diff relies on *alt*-side block invisibility). The skill stops if it detects `main_handle` logged in. Always run as the alt.
  *Label: `Edge: wrong-account`*
- **Login wall.** Instagram requires the user's own logged-in (alt) session and shows a login form otherwise. Never log in programmatically — hand off to the user and resume.
  *Label: `Edge: login-wall`*
- **Pre-check fails.** If the control account doesn't disappear after the alt blocks it, block-invisibility doesn't apply in third-party follower lists on this app version, and the whole non-destructive technique is void. Stop and report; only proceed to destructive main-blocking with explicit user opt-in.
  *Label: `Edge: precheck-fails`*
- **Virtualized-list skips.** The modal renders only near-viewport rows; big scroll jumps skip rows. Scroll in modest steps, settle, dedupe by username. If `scraped_count` is far below `stated_follower_count`, you jumped too far — reset to the top of the modal and scroll gently.
  *Label: `Edge: virtualized-skips`*
- **List too large.** Very large follower counts may not fully paginate before Instagram rate-limits. Increase `scroll_settle_ms`, scrape in one sitting, and if you can't reach the bottom, say so — a partial scrape can still surface the candidate IF the burner's linked account was captured in the scraped portion, but flag the result as partial.
  *Label: `Edge: list-too-large`*
- **Action block.** Aggressive scrolling can trigger "Action Blocked / Try Again Later". Back off: stop scraping, wait, raise `scroll_settle_ms`, and resume later. Never hammer through it.
  *Label: `Edge: action-block`*
- **No diff.** An empty candidate set means inconclusive (careful burner / no linkage), not a bug. Report it as such. Re-confirm both snapshots fully paginated before concluding.
  *Label: `Edge: no-diff`*
- **Burner already gone.** If the burner isn't in the before-snapshot, it may have unfollowed, deactivated, or gone private. Confirm the handle and that the alt can see it before blocking.
  *Label: `Edge: burner-gone`*
- **Coincidental disappearances.** Followers who deactivate, go private, or block the alt during the run also vanish from the diff. Keep the before/after window tight to minimize this, and present candidates as leads to verify, never as proof.

## Why this skill exists

A blank account created in 2020 followed the user's main, with the user as its only follow — a classic "someone I know made a burner to watch me" pattern. This skill turns Instagram's own anti-abuse machinery (linked-account block cascade) plus a quirk of block visibility into a non-destructive identification probe: observe from a throwaway alt, let the cascade do the linking, and read the answer off the follower-list diff — without ever losing a real follower or tipping off the suspect.
