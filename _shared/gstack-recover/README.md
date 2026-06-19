# gstack-recover

Self-heals a wedged `gstack browse` session. Idempotent — a no-op when healthy.

## The failure (2026-06-19)

gstack's `browse` bun server keeps running but loses its pipe to its
Chrome-for-Testing, so **every** page command (`goto`, `url`, `tabs`, even
`restart`) returns `No active page. Use "browse goto <url>" first.` while
`--help` still works. The server can't recover itself; only a process-level
restart clears it. This wedge silently blocked a whole `/flywheel --refresh`.

## Usage

```bash
go build -o gstack-recover .   # gitignored, built per-machine
./gstack-recover [--probe-url https://example.com]
# exit 0 = healthy (already, or recovered) · 1 = still wedged · 2 = browse binary missing
```

It probes with `goto` (which self-heals a cold server too); on the wedge marker
it hard-kills the gstack bun server + its Chrome-for-Testing — **scoped to the
`.gstack/chromium-profile` instance only, never the user's real Chrome** — waits,
and re-probes so the next `goto` respawns a clean server.

Wired into `_shared/gstack_auth.sh` (runs before the login check) and referenced
from `buffer-stats` / `linkedin-stats` browser-init. Pair with the **dot-prefixed
cookie import** fix (`cookie-import-browser chrome --domain .buffer.com`, not the
bare `buffer.com` which imports 0) — both were the silent walls on 2026-06-19.
