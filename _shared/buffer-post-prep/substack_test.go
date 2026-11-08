package main

import (
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSubstackNotesCLI(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "buffer-post-prep")
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build real CLI: %v\n%s", err, out)
	}
	for _, tc := range []struct {
		name, service, text string
		exitCode            int
	}{
		{"text-only note", "substack", "A note about attention.\n\nComment \"newsletter\"", 0},
		{"ASCII at limit", "substack", strings.Repeat("a", 10000), 0},
		{"ASCII over limit", "substack", strings.Repeat("a", 10001), 65},
		{"emoji at UTF-16 limit", "substack", strings.Repeat("📈", 5000), 0},
		{"emoji over UTF-16 limit", "substack", strings.Repeat("📈", 5001), 65},
		{"empty note", "substack", " ", 64},
		{"unknown service still rejected", "not-a-service", "hello", 64},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(binary, "--channel-id", "000000000000000000000000", "--service", tc.service, "--text", tc.text, "--format-tag", "verbatim_quote")
			out, err := cmd.Output()
			code := 0
			if err != nil {
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) {
					t.Fatal(err)
				}
				code = exitErr.ExitCode()
			}
			if code != tc.exitCode {
				t.Fatalf("exit=%d, want=%d: %v", code, tc.exitCode, err)
			}
			if code != 0 {
				return
			}
			var args map[string]any
			if err := json.Unmarshal(out, &args); err != nil {
				t.Fatalf("invalid create_post JSON: %v\n%s", err, out)
			}
			if args["text"] != tc.text || args["mode"] != "addToQueue" || args["schedulingType"] != "automatic" || args["channelId"] != "000000000000000000000000" {
				t.Fatalf("wrong post args: %v", args)
			}
			if _, ok := args["assets"]; ok {
				t.Fatal("text-only Notes must not acquire assets")
			}
			if _, ok := args["metadata"]; ok {
				t.Fatal("text-only Notes need no invented metadata")
			}
		})
	}
}
