// publish-constraints — pre-publish validation gate for NON-Buffer scheduling.
//
// Pure transport. No cognition. Refuses invalid (clip × platform × field)
// combinations; never rewrites content. The OpusClip-native path (and any future
// direct-publish skill) pipes the batch it is ABOUT to schedule through this
// binary and reads the exit code — the unbypassable analog of buffer-post-prep
// for the Buffer path.
//
// Why this exists: OpusClip's `post schedule --title` is the post CAPTION on
// FB/IG/LinkedIn/TikTok but the VIDEO TITLE on YouTube (100-char hard cap). A
// 2026-05-31 batch passed the long caption as --title for every platform; it
// posted fine on four and failed silently on YouTube only (25/25). This gate
// makes that class of "wrong field, wrong cap, silent platform-specific failure"
// impossible to ship.
//
// Usage:
//
//	echo '[{"clip_id":"X","label":"YOUTUBE Enterprise Vibe Code","title":"...","description":"..."}, ...]' \
//	  | publish-constraints validate
//
// Input: a JSON array of cells on stdin, one per (clip × platform) schedule the
// caller is about to fire. Each cell carries the EXACT title/description args the
// schedule call will use.
//
// Output: nothing on success; one JSON verdict object per failing cell on stdout.
// Exit codes: 0=all valid, 64=usage error, 65=>=1 cell invalid.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
)

type cell struct {
	ClipID      string `json:"clip_id"`
	Label       string `json:"label"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type verdict struct {
	ClipID   string `json:"clip_id"`
	Label    string `json:"label"`
	Platform string `json:"platform"`
	Field    string `json:"field"`
	Reason   string `json:"reason"`
	FixHint  string `json:"fix_hint,omitempty"`
}

func main() {
	if len(os.Args) < 2 {
		fail(64, "usage: publish-constraints <validate>  (cells JSON on stdin)")
	}
	switch os.Args[1] {
	case "validate":
		runValidate()
	default:
		fail(64, "unknown subcommand %q (only: validate)", os.Args[1])
	}
}

func runValidate() {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		fail(64, "reading stdin: %v", err)
	}
	var cells []cell
	if err := json.Unmarshal(raw, &cells); err != nil {
		fail(64, "stdin is not a JSON array of cells: %v", err)
	}
	if len(cells) == 0 {
		fail(64, "no cells on stdin")
	}

	var verdicts []verdict
	for _, c := range cells {
		verdicts = append(verdicts, validateCell(c)...)
	}

	if len(verdicts) == 0 {
		os.Exit(0)
	}
	enc := json.NewEncoder(os.Stdout)
	for _, v := range verdicts {
		_ = enc.Encode(v) // one JSON object per line
	}
	fmt.Fprintf(os.Stderr, "publish-constraints: %d of %d cells invalid\n", len(verdicts), len(cells))
	os.Exit(65)
}

func validateCell(c cell) []verdict {
	platform := labelToPlatform(c.Label)
	pc, ok := constraints[platform]
	if !ok {
		// Unknown/unsupported label — a new connected account that no platform
		// row covers. FAIL CLOSED: never let an unvalidated cell through.
		return []verdict{{
			ClipID: c.ClipID, Label: c.Label, Platform: platform, Field: "label",
			Reason:  fmt.Sprintf("no constraints row for label %q (platform=%q)", c.Label, platform),
			FixHint: "add a row to _shared/publish-constraints/constraints.json + the golden fixture",
		}}
	}

	fieldVal := map[string]string{"title": c.Title, "description": c.Description}
	names := make([]string, 0, len(pc.Fields))
	for n := range pc.Fields {
		names = append(names, n)
	}
	sort.Strings(names) // deterministic verdict order: description, title

	var out []verdict
	for _, name := range names {
		fc := pc.Fields[name]
		val := fieldVal[name]

		if fc.Required && strings.TrimSpace(val) == "" {
			fh := ""
			if pc.TitleSource == "separate" && name == "title" {
				fh = fmt.Sprintf("%s needs a SEPARATE %q field; post-text goes to %q, not %q",
					platform, "title", pc.PostTextField, "title")
			}
			out = append(out, verdict{c.ClipID, c.Label, platform, name, "required field is empty", fh})
			continue
		}
		if fc.MaxRunes > 0 {
			if n := runeLen(val); n > fc.MaxRunes {
				fh := fmt.Sprintf("trim %q to <= %d chars", name, fc.MaxRunes)
				if pc.TitleSource == "separate" && name == "title" {
					fh = fmt.Sprintf("%s maps post-text to %q; supply a separate <=%d-char title, don't reuse the caption",
						platform, pc.PostTextField, fc.MaxRunes)
				}
				out = append(out, verdict{c.ClipID, c.Label, platform, name,
					fmt.Sprintf("%s length %d exceeds %s cap %d", name, n, platform, fc.MaxRunes), fh})
			}
		}
		if fc.ForbiddenRe != "" {
			re, err := regexp.Compile(fc.ForbiddenRe)
			if err == nil && re.MatchString(val) {
				out = append(out, verdict{c.ClipID, c.Label, platform, name,
					fmt.Sprintf("%s contains forbidden characters (/%s/)", name, fc.ForbiddenRe),
					"remove the disallowed characters"})
			}
		}
	}
	return out
}

func fail(code int, format string, a ...any) {
	fmt.Fprintf(os.Stderr, "publish-constraints: "+format+"\n", a...)
	os.Exit(code)
}
