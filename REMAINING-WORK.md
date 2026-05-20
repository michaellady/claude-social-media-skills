# Remaining work — social media closed-loop

**Captured 2026-05-20** at the end of the closed-loop build session, so the task list can be cleared. Everything below is either pending implementation, a strategy follow-up the flywheel surfaced, or tech debt. Pick up from here.

## Pending implementation

### 1. threads-stats — per-post insights fetcher (was task #375)
Investigation (done) confirmed FEASIBLE: Meta Threads API exposes `GET /v1.0/{threads-media-id}/insights` (views, likes, replies, reposts, quotes, shares). Auth: OAuth 2.0 `threads_basic + threads_manage_insights`; dev-mode skips App Review (add mikelady + enterprisevibecode as testers). Design in `threads-stats/INVESTIGATION.md`.
- **Write it in Go** (per `~/dev/AGENTS.md` directive — no new bash modules).
- Gated on user completing OAuth setup (Meta dev app + tokens).
- Output: `threads-stats/cache/snapshot-<date>.json`, same shape as linkedin-stats, with `[opus:<id>]` extraction for the closed-loop join.
- Wire into `_shared/content-attribution` (the `threads` matcher is currently a `pending #375` stub).

### 2. post-manifest — schema fields + Go rewrite (was task #376)
Add `schema_version: "1"`, `source_video.duration_seconds`, `clips[].published_url` (surfaced by the opus-clips-performance scaffold). 
- **Do it as the Go rewrite of `post_manifest.sh`** — it's the last remaining bash module (`content-attribution` already migrated). Same precedent as voice-corpus / buffer-post-prep / content-attribution. Subcommands: `init`, `ensure-clip`, `append-post`, `count-scheduled`, `find-clip`, `conflicts`, `schedule-ids`, `list-by-channel`. Then update consumers (opus-clips, flywheel Phase 4.55) to call the binary.

### 3. tiktok-stats — activate (was task #373, scaffolded)
Scaffold exists (`tiktok-stats/SKILL.md`, NOT-YET-FUNCTIONAL banner). Needs user OAuth: register TikTok for Developers app for mikelady, capture `TIKTOK_ACCESS_TOKEN` in `~/.zshrc`. Then implement the `/v2/video/list/` fetch + snapshot in Go. Wire into content-attribution (currently `pending #373` stub).

### 4. opus-clips-performance — refactor onto the content-attribution binary (was task #372)
Scaffolded with its own YouTube-join logic. Now that `_shared/content-attribution` is a working Go binary, thin opus-clips-performance down to call `content-attribution join` instead of duplicating the JOIN. May be deprecate-able if `/flywheel` Phase 4.56 covers the per-clip rollup.

## Strategy follow-ups (surfaced by the 2026-05-20 flywheel)

### 5. Prune dead channels from fan-out
Both **Threads EVC** (19 followers, 0 reactions / 64 posts — `audits/threads-evc-dead-channel.md`) and **Facebook EVC** (0 reactions / 5 posts this week) score 🔴 dead-candidate. Drop them from the default fan-out set in the promote-* / opus-clips channel config. Concentrate the freed slots on the two 🟢 workhorses (Instagram + LinkedIn personal).
- **First do #6 (FB investigation)** before fully pruning FB — it had an 18-reaction breakout last week, so it may be recoverable, unlike Threads EVC.

### 6. Investigate Facebook EVC going dark
FB EVC had a breakout post last week (18 reactions) but 0 this week across 5 posts. Determine: algorithm change, content mismatch, or noise. Decide reduce-vs-pause. Pattern after `audits/threads-evc-dead-channel.md`.

### 7. CTA hypothesis re-check (~2026-05-22, after Day 2-3 clips fire)
The conversion fix (newsletter CTA + UTM links on 48 pending clips) is a **hypothesis test**: we believe missing-CTA caused 0% YouTube→newsletter conversion. After the Day 2-3 clips fire (~48h), run `/flywheel` and check `beehiiv_attribution`:
- **Success signal:** non-zero `utm_source=youtube` / `utm_source=linkedin` signups under `utm_campaign=opus_uEposKmbFvY`, traceable per-clip via `utm_content`.
- **Failure signal:** still 0% even with the CTA → problem is deeper (link friction, audience mismatch, beehiiv not parsing UTMs) → investigate further.
- Note: the 36 Day-1 clips fired CTA-less (can't edit live posts) — exclude them from the test; only the 48 backfilled + future clips carry the CTA.

## Tech debt / notes

- **The two `_shared` bash modules:** `content-attribution` is now Go. `post_manifest.sh` is the last bash module → fold its Go rewrite into #2.
- **IG/TikTok conversions are UTM-invisible** by design (comment-to-DM, no trackable link). They land in beehiiv's coarse `organic`/`referral` buckets. Accepted tradeoff — not the high-converting channels.
- **The core unresolved business problem:** amplification works (11.2× clip reach off one long-form) but conversion was 0%. The CTA fix is the first attempt to close that. If it works, the flywheel finally spins: long-form → clips → reach → newsletter signups → consulting leads. If it doesn't, the bottleneck is elsewhere and needs a different intervention.

## Done this session (for context — don't redo)
linkedin-stats Phase 3b per-post scrape (#370) · buffer-stats real tagId JOIN + all-6-channel ROI (#371, #377) · ARCHITECTURE.md closed-loop docs (#378) · Threads EVC dead-channel audit (#379) · flywheel Phase 4.55/4.56 unified JOIN (#380) · content-attribution JOIN engine, built then migrated to Go (#381, #383) · live flywheel + buffer-stats + LinkedIn-vs-beehiiv newsletter comparison (#382) wired into Priority 3 with auto-refresh · opus-clips Go rewrite + CTA fix (forward + 48-post backfill, #384). All committed to origin/main through `85b69f8` + the CTA backfill manifest updates.
