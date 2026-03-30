# Claude Social Media Skills

Claude Code skills for promoting your work on social media via [Buffer](https://buffer.com).

## Skills

### `/promote-newsletter`

Extract snippets from a [beehiiv](https://beehiiv.com) newsletter post and schedule platform-specific social media posts. Preserves the author's original words — only trims to fit character limits.

```
/promote-newsletter https://www.example.com/p/my-post
/promote-newsletter latest
```

### `/promote-github`

Fetch your public GitHub contributions (merged PRs, commits, releases) and compose value/impact-framed social media posts. Posts immediately to all connected Buffer channels.

```
/promote-github today
/promote-github this-week
/promote-github 2026-03-01..2026-03-30
/promote-github https://github.com/user/repo/pull/123
```

## Setup

1. Install [Claude Code](https://claude.ai/code)
2. Connect a [Buffer MCP server](https://publish.buffer.com/settings/api) with your API key
3. Symlink each skill directory into `~/.claude/skills/`:
   ```bash
   ln -s /path/to/claude-social-media-skills/promote-newsletter ~/.claude/skills/promote-newsletter
   ln -s /path/to/claude-social-media-skills/promote-github ~/.claude/skills/promote-github
   ```
4. Use the slash commands from any Claude Code session

## How It Works

Each skill follows a multi-phase workflow:

1. **Fetch** content (newsletter via RSS/URL, or GitHub via `gh` CLI)
2. **Identify** the best snippets or contributions to promote
3. **User selects** what to post and how (batch vs. individual)
4. **Compose** platform-specific posts respecting character limits
5. **Review** all posts before publishing
6. **Post** to Buffer across all connected channels

Supported platforms: Twitter/X, Bluesky, LinkedIn, Threads, Facebook, Mastodon, Instagram, Pinterest.
