module github.com/michaellady/claude-social-media-skills/_shared/adversarial-review

go 1.25

require github.com/michaellady/mike-skills/llm-provider v0.0.0

replace github.com/michaellady/mike-skills/llm-provider => ./internal/llm-provider
