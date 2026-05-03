// Command adversarial-review dispatches the SAME prompt to both the Claude
// and Codex CLIs in parallel, parses each reviewer's JSON verdict, and emits
// a merged canonical response on stdout.
//
// Usage:
//
//	cat prompt.txt | adversarial-review
//	adversarial-review --prompt-file prompt.txt
//
// The merged response shape (see SKILL.md for the contract):
//
//	{
//	  "summary": "all_pass" | "some_fail" | "parse_error",
//	  "verdicts": [
//	    {"draft_id": "<id>", "verdict": "PASS"|"FAIL", "issues": ["[claude] ...", "[codex] ...", "[both] ..."]}
//	  ],
//	  "reviewers": ["claude", "codex"],
//	  "claude_skipped": bool, "claude_skip_reason": string,
//	  "codex_skipped":  bool, "codex_skip_reason":  string,
//	  "claude_parse_error": bool, "codex_parse_error": bool,
//	  "error": string, "raw_response": string
//	}
//
// Merge rule: a draft is FAIL if either reviewer flagged it FAIL.
// Issues are deduplicated and prefixed [claude] / [codex] / [both].
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/michaellady/mike-skills/llm-provider/claude"
	"github.com/michaellady/mike-skills/llm-provider/codex"
	"github.com/michaellady/mike-skills/llm-provider/provider"
)

type verdict struct {
	DraftID string   `json:"draft_id"`
	Verdict string   `json:"verdict"`
	Issues  []string `json:"issues"`
}

type reviewerResp struct {
	Summary  string    `json:"summary"`
	Verdicts []verdict `json:"verdicts"`
}

type mergedResp struct {
	Summary           string    `json:"summary"`
	Verdicts          []verdict `json:"verdicts"`
	Reviewers         []string  `json:"reviewers"`
	ClaudeSkipped     bool      `json:"claude_skipped,omitempty"`
	ClaudeSkipReason  string    `json:"claude_skip_reason,omitempty"`
	CodexSkipped      bool      `json:"codex_skipped,omitempty"`
	CodexSkipReason   string    `json:"codex_skip_reason,omitempty"`
	ClaudeParseError  bool      `json:"claude_parse_error,omitempty"`
	CodexParseError   bool      `json:"codex_parse_error,omitempty"`
	Error             string    `json:"error,omitempty"`
	RawResponse       string    `json:"raw_response,omitempty"`
}

func main() {
	var promptFile string
	var timeoutSec int
	var quiet bool
	flag.StringVar(&promptFile, "prompt-file", "", "path to prompt file; if empty, read from stdin")
	flag.IntVar(&timeoutSec, "timeout", 300, "per-reviewer timeout (seconds)")
	flag.BoolVar(&quiet, "quiet", false, "suppress provider heartbeat lines on stderr")
	flag.Parse()

	promptPath, cleanup, err := resolvePromptPath(promptFile)
	if err != nil {
		die("prompt input: %v", err)
	}
	defer cleanup()

	out := mergedResp{Verdicts: []verdict{}, Reviewers: []string{}}

	_, claudeAvailable := lookCLI("claude")
	_, codexAvailable := lookCLI("codex")
	if !claudeAvailable {
		out.ClaudeSkipped = true
		out.ClaudeSkipReason = "claude CLI not on PATH"
	}
	if !codexAvailable {
		out.CodexSkipped = true
		out.CodexSkipReason = "codex CLI not on PATH"
	}

	var (
		wg                   sync.WaitGroup
		claudeOut, codexOut  string
		claudeErr, codexErr  error
	)
	if claudeAvailable {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claudeOut, claudeErr = runProvider(claude.New(), promptPath, timeoutSec, quiet)
		}()
	}
	if codexAvailable {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codexOut, codexErr = runProvider(codex.New(), promptPath, timeoutSec, quiet)
		}()
	}
	wg.Wait()

	var claudeResp, codexResp *reviewerResp
	if claudeAvailable {
		if claudeErr != nil {
			out.ClaudeParseError = true
			out.RawResponse += fmt.Sprintf("[claude error] %v\n", claudeErr)
		} else if r, perr := parseResponse(claudeOut); perr != nil {
			out.ClaudeParseError = true
			out.RawResponse += fmt.Sprintf("[claude raw]\n%s\n", claudeOut)
		} else {
			claudeResp = r
		}
	}
	if codexAvailable {
		if codexErr != nil {
			out.CodexParseError = true
			out.RawResponse += fmt.Sprintf("[codex error] %v\n", codexErr)
		} else if r, perr := parseResponse(codexOut); perr != nil {
			out.CodexParseError = true
			out.RawResponse += fmt.Sprintf("[codex raw]\n%s\n", codexOut)
		} else {
			codexResp = r
		}
	}

	if claudeResp == nil && codexResp == nil {
		out.Summary = "parse_error"
		out.Error = "both reviewers unavailable, errored, or returned malformed JSON"
		emit(out)
		os.Exit(2)
	}

	merged := merge(claudeResp, codexResp)
	out.Summary = merged.Summary
	out.Verdicts = merged.Verdicts
	if claudeResp != nil {
		out.Reviewers = append(out.Reviewers, "claude")
	}
	if codexResp != nil {
		out.Reviewers = append(out.Reviewers, "codex")
	}
	emit(out)
}

func resolvePromptPath(p string) (string, func(), error) {
	if p != "" {
		if _, err := os.Stat(p); err != nil {
			return "", nil, err
		}
		return p, func() {}, nil
	}
	tmp, err := os.CreateTemp("", "adversarial-review-prompt-*.txt")
	if err != nil {
		return "", nil, err
	}
	if _, err := io.Copy(tmp, os.Stdin); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return "", nil, err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return "", nil, err
	}
	return tmp.Name(), func() { _ = os.Remove(tmp.Name()) }, nil
}

func lookCLI(name string) (string, bool) {
	p, err := exec.LookPath(name)
	if err != nil {
		return "", false
	}
	return p, true
}

func runProvider(p provider.Provider, promptPath string, timeoutSec int, quiet bool) (string, error) {
	var buf strings.Builder
	opts := provider.Options{
		PromptFile: promptPath,
		Timeout:    time.Duration(timeoutSec) * time.Second,
		Quiet:      quiet,
		Stdout:     &buf,
		Stderr:     os.Stderr,
		ThreadOut:  filepath.Join(os.TempDir(), fmt.Sprintf("ar-%s-%d.thread", p.Name(), os.Getpid())),
	}
	err := p.Run(context.Background(), opts)
	return buf.String(), err
}

// parseResponse extracts the JSON verdict object from a reviewer's reply.
// Reviewers sometimes wrap JSON in markdown fences or surrounding prose;
// tolerate that by trimming fences and slicing between the first '{' and
// last '}'.
func parseResponse(s string) (*reviewerResp, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if i := strings.Index(s, "\n"); i >= 0 {
			s = s[i+1:]
		}
		if j := strings.LastIndex(s, "```"); j >= 0 {
			s = s[:j]
		}
		s = strings.TrimSpace(s)
	}
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end < 0 || end < start {
		return nil, fmt.Errorf("no JSON object found")
	}
	s = s[start : end+1]
	var r reviewerResp
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func merge(claudeR, codexR *reviewerResp) mergedResp {
	type slot struct {
		id      string
		claudeV *verdict
		codexV  *verdict
	}
	order := []string{}
	slots := map[string]*slot{}

	addAll := func(reviewer string, r *reviewerResp) {
		if r == nil {
			return
		}
		for i := range r.Verdicts {
			v := &r.Verdicts[i]
			if _, ok := slots[v.DraftID]; !ok {
				order = append(order, v.DraftID)
				slots[v.DraftID] = &slot{id: v.DraftID}
			}
			switch reviewer {
			case "claude":
				slots[v.DraftID].claudeV = v
			case "codex":
				slots[v.DraftID].codexV = v
			}
		}
	}
	addAll("claude", claudeR)
	addAll("codex", codexR)

	out := mergedResp{Verdicts: []verdict{}}
	anyFail := false
	for _, id := range order {
		s := slots[id]
		v := verdict{DraftID: id, Verdict: "PASS", Issues: []string{}}
		if (s.claudeV != nil && s.claudeV.Verdict == "FAIL") ||
			(s.codexV != nil && s.codexV.Verdict == "FAIL") {
			v.Verdict = "FAIL"
			anyFail = true
		}
		v.Issues = mergeIssues(s.claudeV, s.codexV)
		out.Verdicts = append(out.Verdicts, v)
	}
	if anyFail {
		out.Summary = "some_fail"
	} else {
		out.Summary = "all_pass"
	}
	return out
}

// mergeIssues attributes each issue to its source. If an issue from Claude
// substantially overlaps an issue from Codex, the pair collapses into a
// single [both] entry; otherwise each issue is prefixed with its origin.
func mergeIssues(claudeV, codexV *verdict) []string {
	var c, x []string
	if claudeV != nil {
		c = claudeV.Issues
	}
	if codexV != nil {
		x = codexV.Issues
	}
	out := []string{}
	used := map[int]bool{}
	for _, ci := range c {
		match := -1
		for j, xi := range x {
			if used[j] {
				continue
			}
			if issueOverlaps(ci, xi) {
				match = j
				break
			}
		}
		if match >= 0 {
			used[match] = true
			out = append(out, "[both] "+ci)
		} else {
			out = append(out, "[claude] "+ci)
		}
	}
	for j, xi := range x {
		if used[j] {
			continue
		}
		out = append(out, "[codex] "+xi)
	}
	return out
}

// issueOverlaps returns true when two issue strings appear to flag the same
// underlying problem. Heuristic: identical (case-insensitive) OR a 12-char
// substring of one appears verbatim in the other. Tuned to favor PASS-only
// dedup over false collapses (when in doubt, keep them separate).
func issueOverlaps(a, b string) bool {
	la := strings.ToLower(a)
	lb := strings.ToLower(b)
	if la == lb {
		return true
	}
	const window = 12
	if len(la) < window || len(lb) < window {
		return false
	}
	for i := 0; i+window <= len(la); i++ {
		if strings.Contains(lb, la[i:i+window]) {
			return true
		}
	}
	return false
}

func emit(out mergedResp) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		die("emit json: %v", err)
	}
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "adversarial-review: "+format+"\n", args...)
	os.Exit(2)
}
