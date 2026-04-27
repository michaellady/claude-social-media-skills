# Claude Social Media Skills

Claude Code skills for promoting and syndicating your work on social media.

These skills are designed as a **closed loop** — every post is tagged at compose time so engagement can be attributed back, and the analytics skills auto-generate skill-config recommendations from each week's data. See **[ARCHITECTURE.md](ARCHITECTURE.md)** for the design philosophy and how the skills connect.

## Skills

### `/promote-newsletter`

Extract snippets from a [beehiiv](https://beehiiv.com) newsletter post and schedule platform-specific social media posts. Preserves the author's original words — only trims to fit character limits.

```
/promote-newsletter https://www.example.com/p/my-post
/promote-newsletter latest
```

### `/tease-newsletter`

Sibling to `/promote-newsletter`. Instead of pulling verbatim snippets, write short original teaser hooks per channel that summarize the article without spoiling the punchline. Same `Comment "newsletter"…` CTA so the same DM automation works.

```
/tease-newsletter https://www.example.com/p/my-post
/tease-newsletter latest
```

### `/promote-github`

Fetch your public GitHub contributions (merged PRs, commits, releases) and compose value/impact-framed social media posts. Posts immediately to all connected Buffer channels.

```
/promote-github today
/promote-github this-week
/promote-github 2026-03-01..2026-03-30
/promote-github https://github.com/user/repo/pull/123
```

### `/crosspost-newsletter`

Cross-post a [beehiiv](https://beehiiv.com) newsletter article across five platforms in two modes:

- **Full-article syndication** to LinkedIn (native article), Substack, and Medium — preserves rich formatting, headings, and images. Sets canonical URL back to the original post.
- **Link submission** to Hacker News and Reddit — submits the beehiiv URL with the article title. For Reddit, picks one or more subreddits from a configurable default list.

```
/crosspost-newsletter https://www.example.com/p/my-post
/crosspost-newsletter latest
```

## Setup

1. Install [Claude Code](https://claude.ai/code)
2. Connect a [Buffer MCP server](https://publish.buffer.com/settings/api) with your API key (for promote-newsletter and promote-github)
3. Install [gstack](https://github.com/nichochar/gstack) browse (for crosspost-newsletter)
4. Symlink each skill directory into `~/.claude/skills/`:
   ```bash
   ln -s /path/to/claude-social-media-skills/promote-newsletter ~/.claude/skills/promote-newsletter
   ln -s /path/to/claude-social-media-skills/tease-newsletter ~/.claude/skills/tease-newsletter
   ln -s /path/to/claude-social-media-skills/promote-github ~/.claude/skills/promote-github
   ln -s /path/to/claude-social-media-skills/crosspost-newsletter ~/.claude/skills/crosspost-newsletter
   ```
5. Use the slash commands from any Claude Code session
