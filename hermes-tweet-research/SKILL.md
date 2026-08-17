---
name: hermes-tweet-research
description: Use when user wants X/Twitter market research, launch listening, creator discovery, or social proof before composing Buffer posts with the social media skills.
user_invocable: true
---

# hermes-tweet-research

Collect X/Twitter social signals with Hermes Tweet and return a compact research packet for the compose skills. Keep this skill read-only by default: it gathers context, then the existing compose, adversarial review, user approval, and Buffer path handles publishing.

## Setup

Install Hermes Tweet in Hermes Agent before using this skill:

```bash
hermes plugins install Xquik-dev/hermes-tweet --enable
export XQUIK_API_KEY="your-xquik-api-key"
```

Leave `HERMES_TWEET_ENABLE_ACTIONS` unset unless the user explicitly asks for live X/Twitter actions and has confirmed the action gate.

## Usage

```text
/hermes-tweet-research "open source launch"
/hermes-tweet-research https://github.com/user/repo
/hermes-tweet-research "newsletter topic" --handles @founder,@project
```

## Process

1. Parse the topic, URL, product name, or handles from the request.
2. Use Hermes Tweet read/search tools to collect current public X/Twitter posts, profile context, links, and recurring language around the topic.
3. Deduplicate by URL, post ID, and substantially similar text.
4. Filter out private details, unsupported claims, and anything unrelated to the requested campaign.
5. Return a research packet for `/promote-github`, `/tease-newsletter`, `/promote-newsletter`, or `/carousel-newsletter`.

## Research Packet

```markdown
## Hermes Tweet Research: {topic}

### High-Signal Posts
| Handle | Signal | Why It Matters | Link |
|--------|--------|----------------|------|

### Angles To Test
1. {angle tied to observed demand}
2. {angle tied to objections}
3. {angle tied to proof or audience language}

### Terms And Hashtags
| Term | Use Case |
|------|----------|

### Risks To Avoid
- {unsupported claim, stale reference, or sensitive detail}

### Recommended Next Skill
`/promote-github` or `/tease-newsletter` with the selected angle.
```

## Guardrails

- Do not publish, reply, repost, like, follow, or DM from this skill.
- Do not invent engagement metrics. Mark unavailable metrics as unknown.
- Do not quote more than short excerpts; summarize the rest.
- Do not pass secrets, private handles, private repository details, or unpublished campaign plans into external posts.
- Keep links intact so the compose skill can re-check context before publishing.
