// dashboard — the single-pane-of-glass web app for the whole social presence.
//
// Reads the FROZEN flywheel snapshot (~/dev/flywheel-snapshots/<date>.json) for
// every cross-surface aggregate (reach, channel ROI, format ROI, per-source
// amplification) and the raw per-platform snapshots only for drill-down detail.
// Serves an interactive localhost SPA with filters + click-through drill-downs.
//
// Pure transport per PRIMITIVE-TEST.md: it reads + serves + renders. All
// judgment (ROI bucketing, amplification capping, recommendations, hypothesis
// verdicts) lives upstream in /flywheel and the insights ledger — the dashboard
// never recomputes an aggregate flywheel already computed.
//
// Usage:
//
//	dashboard serve [--port 7777] [--open]
//
// Exit codes: 0=ok, 64=usage error.
package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "serve":
		runServe(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "dashboard: unknown command %q\n", os.Args[1])
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: dashboard serve [--port 7777] [--open]")
	os.Exit(64)
}

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	port := fs.Int("port", 7777, "localhost port to bind")
	open := fs.Bool("open", false, "open the dashboard in the default browser")
	_ = fs.Parse(args)

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dashboard: cannot bind %s: %v\n", addr, err)
		os.Exit(1)
	}

	url := fmt.Sprintf("http://%s/", addr)
	fmt.Printf("EVC single-pane dashboard → %s\n", url)
	if snap := flywheelSnapshotPath(); snap != "" {
		fmt.Printf("  reading snapshot: %s\n", snap)
	} else {
		fmt.Println("  ⚠ no flywheel snapshot found yet — run /flywheel to populate it")
	}
	fmt.Println("  Ctrl-C to stop")

	srv := &http.Server{Handler: newServer().mux, ReadHeaderTimeout: 5 * time.Second}
	if *open {
		go func() { time.Sleep(300 * time.Millisecond); openBrowser(url) }()
	}
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "dashboard: %v\n", err)
		os.Exit(1)
	}
}

// openBrowser opens url in the default browser (cross-platform). Best-effort.
func openBrowser(url string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler"}
	default:
		cmd = "xdg-open"
	}
	args = append(args, url)
	_ = exec.Command(cmd, args...).Start()
}
