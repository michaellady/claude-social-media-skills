// Command adversarial-review is the self-contained, vendored build of
// converge's "audit" fan-out (the folded-in adversarial review). It dispatches
// the SAME prompt to every selected reviewer (claude, codex, agy by default;
// agent opt-in) in parallel, parses each reviewer's JSON verdict, and emits a
// merged canonical response:
//
//	cat prompt.txt | adversarial-review
//	adversarial-review --prompt-file prompt.txt --reviewers claude,codex,agy
//
// The fan-out logic in internal/fanout is synced VERBATIM from
// mike-skills/converge/go/internal/fanout via sync.sh; this main is the only
// vendored-specific glue (so the social-media-skills repo builds without a
// mike-skills checkout). The merged JSON shape and merge rules are documented
// in internal/fanout/fanout.go and converge's `audit` mode.
package main

import (
	"os"

	"github.com/michaellady/claude-social-media-skills/_shared/adversarial-review/internal/fanout"
)

func main() { os.Exit(fanout.Run(os.Args[1:])) }
