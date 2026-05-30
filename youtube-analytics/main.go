package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "fetch":
		err = cmdFetch(args)
	case "fetch-analytics":
		err = cmdFetchAnalytics(args)
	case "fetch-comments":
		err = cmdFetchComments(args)
	case "fetch-peers":
		err = cmdFetchPeers(args)
	case "analyze":
		err = cmdAnalyze(args)
	case "dashboard":
		err = cmdDashboard(args)
	case "find-duplicates":
		err = cmdFindDuplicates(args)
	case "data-quality":
		err = cmdDataQuality(args)
	case "titles":
		err = cmdTitles(args)
	case "video":
		err = cmdVideo(args)
	case "export":
		err = cmdExport(args)
	case "suggest-tags":
		err = cmdSuggestTags(args)
	case "update-tags":
		err = cmdUpdateTags(args)
	case "cohort":
		err = cmdCohort(args)
	case "insights":
		err = cmdInsights(args)
	case "-h", "--help", "help":
		printUsage()
		return
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: youtube-analytics <command> [flags]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  fetch             Fetch all video data from YouTube API → data/videos.json")
	fmt.Println("  fetch-analytics   Fetch watch time data via OAuth2 → merge into data/videos.json")
	fmt.Println("                    Flags: --daily, --traffic-sources, --sub-status, --all")
	fmt.Println("  fetch-comments    Fetch top-level comments per video → data/comments.json")
	fmt.Println("  fetch-peers       Fetch top recent videos from peer channels (data/peer-channels.yaml)")
	fmt.Println("                    → data/peer-videos.json. Used by /yt-ab for title grounding.")
	fmt.Println("  analyze           Analyze data and print terminal report")
	fmt.Println("  dashboard         Generate interactive HTML dashboard → data/dashboard.html")
	fmt.Println("  find-duplicates   Detect near-duplicate uploads")
	fmt.Println("  data-quality      Report on missing tags, templated descriptions, etc.")
	fmt.Println("  titles            Title pattern analysis for top performers")
	fmt.Println("  video <id>        Deep-dive on a single video")
	fmt.Println("  export            Export video data as CSV/TSV")
	fmt.Println("  suggest-tags      Generate data/tags.json with rule-based tag recommendations")
	fmt.Println("  update-tags       Preview or push tag updates to YouTube (dry-run default)")
	fmt.Println("  cohort <subcmd>   Local cohort tagging: list|show|assign|unassign|auto|report")
	fmt.Println("  insights <subcmd> Hypothesis ledger: list|pending|grade|new")
	fmt.Println()
	fmt.Println("Common flags (analyze, dashboard, export):")
	fmt.Println("  --since YYYY-MM-DD        Filter videos published on/after this date")
	fmt.Println("  --until YYYY-MM-DD        Filter videos published on/before this date")
	fmt.Println("  --type <short|long-form|live>   Filter by video type (repeatable)")
	fmt.Println("  --duration-min N          Minimum duration in seconds")
	fmt.Println("  --duration-max N          Maximum duration in seconds")
	fmt.Println("  --exclude <ID>            Exclude a specific video ID (repeatable)")
	fmt.Println("  --cohort <id>             Filter to a cohort id from data/cohorts.yaml (repeatable)")
	fmt.Println()
	fmt.Println("Analyze-only:")
	fmt.Println("  --diff <snapshot.json>    Delta report vs. a prior videos.json")
	fmt.Println("  --compare-periods A..B C..D  Side-by-side stats for two date ranges")
}

// stringSliceFlag collects repeated --flag values into a slice.
type stringSliceFlag []string

func (s *stringSliceFlag) String() string     { return strings.Join(*s, ",") }
func (s *stringSliceFlag) Set(v string) error { *s = append(*s, v); return nil }

// filterFlags binds filter-related flags onto a FlagSet and returns a resolver
// that produces a VideoFilter after fs.Parse has run.
type filterFlags struct {
	since, until   string
	types          stringSliceFlag
	durMin, durMax int
	excludes       stringSliceFlag
	cohorts        stringSliceFlag
}

func addFilterFlags(fs *flag.FlagSet) *filterFlags {
	ff := &filterFlags{}
	fs.StringVar(&ff.since, "since", "", "filter videos published on or after YYYY-MM-DD")
	fs.StringVar(&ff.until, "until", "", "filter videos published on or before YYYY-MM-DD")
	fs.Var(&ff.types, "type", "filter by video type: short|long-form|live (repeatable)")
	fs.IntVar(&ff.durMin, "duration-min", 0, "minimum duration in seconds")
	fs.IntVar(&ff.durMax, "duration-max", 0, "maximum duration in seconds")
	fs.Var(&ff.excludes, "exclude", "exclude a specific video ID (repeatable)")
	fs.Var(&ff.cohorts, "cohort", "filter to a cohort id from data/cohorts.yaml (repeatable)")
	return ff
}

func (ff *filterFlags) build() (VideoFilter, error) {
	f := VideoFilter{
		DurMinSec: ff.durMin,
		DurMaxSec: ff.durMax,
		Types:     []string(ff.types),
	}
	if ff.since != "" {
		t, err := parseDateStr(ff.since)
		if err != nil {
			return f, fmt.Errorf("--since: %w", err)
		}
		f.Since = t
	}
	if ff.until != "" {
		t, err := parseDateStr(ff.until)
		if err != nil {
			return f, fmt.Errorf("--until: %w", err)
		}
		// Include the whole --until day by rolling to 23:59:59.
		f.Until = t.Add(24*time.Hour - time.Second)
	}
	for _, t := range f.Types {
		if t != "short" && t != "long-form" && t != "live" {
			return f, fmt.Errorf("--type: %q must be short|long-form|live", t)
		}
	}
	if len(ff.excludes) > 0 {
		f.ExcludeIDs = map[string]bool{}
		for _, id := range ff.excludes {
			f.ExcludeIDs[id] = true
		}
	}
	if len(ff.cohorts) > 0 {
		f.Cohorts = []string(ff.cohorts)
		assignments, err := loadAssignments(cohortAssignmentsPath)
		if err != nil {
			return f, fmt.Errorf("--cohort: %w", err)
		}
		f.CohortAssignments = assignments
	}
	return f, nil
}

func cmdFetch(args []string) error {
	fs := flag.NewFlagSet("fetch", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	apiKey := os.Getenv("YOUTUBE_API_KEY")
	handle := os.Getenv("CHANNEL_HANDLE")
	if apiKey == "" || handle == "" {
		return fmt.Errorf("YOUTUBE_API_KEY and CHANNEL_HANDLE must be set in .env")
	}
	return runFetch(apiKey, handle)
}

func cmdFetchAnalytics(args []string) error {
	fs := flag.NewFlagSet("fetch-analytics", flag.ExitOnError)
	daily := fs.Bool("daily", false, "fetch per-day breakdown into Video.daily_metrics")
	traffic := fs.Bool("traffic-sources", false, "fetch per-source view share into Video.traffic_sources")
	subStatus := fs.Bool("sub-status", false, "fetch SUBSCRIBED/UNSUBSCRIBED split into Video.sub_status_metrics")
	all := fs.Bool("all", false, "shorthand for --daily --traffic-sources --sub-status")
	if err := fs.Parse(args); err != nil {
		return err
	}
	opts := AnalyticsOptions{Daily: *daily, TrafficSources: *traffic, SubStatus: *subStatus}
	if *all {
		opts.Daily = true
		opts.TrafficSources = true
		opts.SubStatus = true
	}
	return runFetchAnalytics("data/videos.json", opts)
}

func cmdAnalyze(args []string) error {
	fs := flag.NewFlagSet("analyze", flag.ExitOnError)
	ff := addFilterFlags(fs)
	diffPath := fs.String("diff", "", "path to a prior videos.json snapshot for delta report")
	comparePeriods := fs.String("compare-periods", "", "two ranges YYYY-MM-DD..YYYY-MM-DD YYYY-MM-DD..YYYY-MM-DD")
	if err := fs.Parse(args); err != nil {
		return err
	}
	filter, err := ff.build()
	if err != nil {
		return err
	}
	if *comparePeriods != "" {
		return runComparePeriods("data/videos.json", *comparePeriods, fs.Args(), filter)
	}
	if *diffPath != "" {
		return runDiff("data/videos.json", *diffPath, filter)
	}
	return runAnalyze("data/videos.json", filter)
}

func cmdDashboard(args []string) error {
	fs := flag.NewFlagSet("dashboard", flag.ExitOnError)
	ff := addFilterFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	filter, err := ff.build()
	if err != nil {
		return err
	}
	return runDashboard("data/videos.json", "data/dashboard.html", filter)
}

func cmdFindDuplicates(args []string) error {
	fs := flag.NewFlagSet("find-duplicates", flag.ExitOnError)
	ff := addFilterFlags(fs)
	durTol := fs.Int("duration-tolerance", 10, "max duration difference in seconds for a pair to count as duplicate")
	timeWindow := fs.Duration("time-window", 5*time.Minute, "max publish-time gap for a pair to count as duplicate")
	if err := fs.Parse(args); err != nil {
		return err
	}
	filter, err := ff.build()
	if err != nil {
		return err
	}
	return runFindDuplicates("data/videos.json", filter, *durTol, *timeWindow)
}

func cmdDataQuality(args []string) error {
	fs := flag.NewFlagSet("data-quality", flag.ExitOnError)
	ff := addFilterFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	filter, err := ff.build()
	if err != nil {
		return err
	}
	return runDataQuality("data/videos.json", filter)
}

func cmdTitles(args []string) error {
	fs := flag.NewFlagSet("titles", flag.ExitOnError)
	ff := addFilterFlags(fs)
	top := fs.Int("top", 10, "how many top titles to sample")
	by := fs.String("by", "views", "sort metric: views|subs|retention|revenue")
	minViews := fs.Int64("min-views", 0, "minimum view count for retention ranking")
	if err := fs.Parse(args); err != nil {
		return err
	}
	filter, err := ff.build()
	if err != nil {
		return err
	}
	return runTitles("data/videos.json", filter, *top, *by, *minViews)
}

func cmdVideo(args []string) error {
	fs := flag.NewFlagSet("video", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("video requires a YouTube video ID argument")
	}
	return runVideo("data/videos.json", fs.Arg(0))
}

func cmdSuggestTags(args []string) error {
	fs := flag.NewFlagSet("suggest-tags", flag.ExitOnError)
	ff := addFilterFlags(fs)
	output := fs.String("output", "data/tags.json", "path to write recommendations")
	if err := fs.Parse(args); err != nil {
		return err
	}
	filter, err := ff.build()
	if err != nil {
		return err
	}
	// Default to live-only when no type filter is set — that's the primary use case.
	if len(filter.Types) == 0 {
		filter.Types = []string{"live"}
	}
	return runSuggestTags("data/videos.json", *output, filter)
}

func cmdUpdateTags(args []string) error {
	fs := flag.NewFlagSet("update-tags", flag.ExitOnError)
	ff := addFilterFlags(fs)
	input := fs.String("input", "data/tags.json", "path to tags file")
	apply := fs.Bool("apply", false, "actually push updates (default is dry-run preview)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	filter, err := ff.build()
	if err != nil {
		return err
	}
	return runUpdateTags("data/videos.json", *input, filter, *apply)
}

func cmdExport(args []string) error {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	ff := addFilterFlags(fs)
	format := fs.String("format", "csv", "csv|tsv")
	output := fs.String("output", "", "path to write (default: stdout)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	filter, err := ff.build()
	if err != nil {
		return err
	}
	return runExport("data/videos.json", filter, *format, *output)
}
