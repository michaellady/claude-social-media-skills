// gstack-recover — self-heal a wedged gstack browse session.
//
// Failure mode (hit 2026-06-19): the gstack `browse` bun server keeps running
// but loses its pipe to its Chrome-for-Testing, so EVERY page command (goto,
// url, tabs, even `restart`) returns "No active page. Use 'browse goto <url>'
// first." — while `--help` still works. The server can't recover itself; only a
// process-level restart clears it.
//
// This binary probes the browser and, if wedged, hard-kills the gstack bun
// server + its Chrome-for-Testing (the `.gstack/chromium-profile` instance ONLY
// — never the user's real Chrome) and re-probes so the next `goto` respawns a
// clean server. Idempotent: a no-op when the browser is healthy.
//
// Usage:
//
//	gstack-recover [--probe-url <url>]   # default https://example.com
//	# exit 0 = healthy (already, or after recovery); 1 = still wedged; 2 = binary missing
//
// It's a Go binary (not a bash snippet) for the same reason the other _shared
// primitives are: the control flow + process management runs identically
// regardless of the caller's shell.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// The sentinel gstack prints for every page command when the server has lost
// its Chrome pipe. Matching this (case-insensitively) is the wedge signal.
const wedgeMarker = "no active page"

// Process patterns to kill. Both are scoped to the gstack profile so the user's
// real Chrome is never touched.
var killPatterns = []string{
	"gstack/browse/src/server.ts", // the bun server
	".gstack/chromium-profile",    // gstack's Chrome-for-Testing (+ its helpers)
}

func browseBin() string {
	if b := os.Getenv("B"); b != "" {
		return b
	}
	return os.ExpandEnv("$HOME/.claude/skills/gstack/browse/dist/browse")
}

// probe runs `browse goto <url>` and returns combined output. `goto` is the
// right probe: on a healthy OR cold server it navigates (and starts the server
// if needed); only a WEDGED server returns the wedge marker.
func probe(bin, url string) string {
	out, _ := exec.Command(bin, "goto", url).CombinedOutput()
	return string(out)
}

func isWedged(out string) bool {
	return strings.Contains(strings.ToLower(out), wedgeMarker)
}

func main() {
	url := "https://example.com"
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		if args[i] == "--probe-url" && i+1 < len(args) {
			url = args[i+1]
		}
	}

	bin := browseBin()
	if fi, err := os.Stat(bin); err != nil || fi.IsDir() {
		fmt.Fprintf(os.Stderr, "gstack-recover: browse binary not found at %s (set $B or install gstack)\n", bin)
		os.Exit(2)
	}

	if !isWedged(probe(bin, url)) {
		fmt.Println("gstack healthy")
		os.Exit(0)
	}

	fmt.Fprintln(os.Stderr, "gstack wedged (no active page) — hard-restarting bun server + Chrome-for-Testing…")
	for _, pat := range killPatterns {
		_ = exec.Command("pkill", "-f", pat).Run() // ignore "no match" exit codes
	}
	time.Sleep(3 * time.Second)

	if isWedged(probe(bin, url)) {
		fmt.Fprintln(os.Stderr, "gstack-recover: STILL wedged after restart — manual intervention needed")
		os.Exit(1)
	}
	fmt.Println("gstack recovered (respawned fresh server + Chrome)")
}
