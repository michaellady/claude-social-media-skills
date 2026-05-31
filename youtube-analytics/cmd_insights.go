package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"text/tabwriter"
	"time"
)

func cmdInsights(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("insights requires a subcommand: list|pending|grade|new")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "list":
		return runInsightsList(rest)
	case "pending":
		return runInsightsPending(rest)
	case "grade":
		return runInsightsGrade(rest)
	case "new":
		return runInsightsNew(rest)
	default:
		return fmt.Errorf("unknown insights subcommand: %s", sub)
	}
}

func runInsightsList(args []string) error {
	fs := flag.NewFlagSet("insights list", flag.ExitOnError)
	ledgerDir := fs.String("ledger-dir", defaultInsightsDir, "ledger directory to read")
	asJSON := fs.Bool("json", false, "emit hypotheses as JSON (for the dashboard / programmatic consumers)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	files, err := listInsightFiles(*ledgerDir)
	if err != nil {
		return err
	}

	if *asJSON {
		return emitInsightsJSON(files)
	}

	if len(files) == 0 {
		fmt.Printf("No insight files in %s. Use `insights new <date>` to create one.\n", *ledgerDir)
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "DATE\tHYPOTHESIS_ID\tSCOPE\tEVAL_AFTER\tVERDICT\tPREDICTION")
	for _, f := range files {
		date := f.Date.Format("2006-01-02")
		if f.Date.IsZero() {
			date = filepath.Base(f.Path)
		}
		if len(f.Hypotheses) == 0 {
			fmt.Fprintf(w, "%s\t(no hypotheses)\t\t\t\t\n", date)
			continue
		}
		for _, h := range f.Hypotheses {
			verdict := h.Verdict
			if verdict == "" {
				verdict = "(pending)"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				date, h.ID, scopeLabel(h),
				h.EvaluateAfter.Format("2006-01-02"),
				verdict, truncate(h.Prediction, 60))
		}
	}
	return w.Flush()
}

// scopeLabel prefers Surface (cross-surface ledger) and falls back to Cohort
// (YouTube ledger) so one column reads sensibly for either ledger.
func scopeLabel(h Hypothesis) string {
	if h.Surface != "" {
		return h.Surface
	}
	return h.Cohort
}

// emitInsightsJSON prints every hypothesis (with its file date) as a JSON array.
// The dashboard's /api/learning consumes this for both ledgers rather than
// re-implementing the frontmatter parser.
func emitInsightsJSON(files []*InsightFile) error {
	type row struct {
		Date          string   `json:"date"`
		ID            string   `json:"id"`
		Surface       string   `json:"surface,omitempty"`
		Cohort        string   `json:"cohort,omitempty"`
		Prediction    string   `json:"prediction"`
		Metric        string   `json:"metric,omitempty"`
		Direction     string   `json:"direction,omitempty"`
		EvaluateAfter string   `json:"evaluate_after,omitempty"`
		Outcome       string   `json:"outcome,omitempty"`
		Verdict       string   `json:"verdict,omitempty"`
		EvidenceIDs   []string `json:"evidence_video_ids,omitempty"`
	}
	out := make([]row, 0)
	for _, f := range files {
		date := f.Date.Format("2006-01-02")
		if f.Date.IsZero() {
			date = filepath.Base(f.Path)
		}
		for _, h := range f.Hypotheses {
			ea := ""
			if !h.EvaluateAfter.IsZero() {
				ea = h.EvaluateAfter.Format("2006-01-02")
			}
			out = append(out, row{
				Date: date, ID: h.ID, Surface: h.Surface, Cohort: h.Cohort,
				Prediction: h.Prediction, Metric: h.Metric, Direction: h.Direction,
				EvaluateAfter: ea, Outcome: h.Outcome, Verdict: h.Verdict,
				EvidenceIDs: h.EvidenceVideoIDs,
			})
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// runInsightsPending lists hypotheses past their evaluate_after with no verdict,
// AND emits the current metrics for each cited video so the model can grade
// them without re-running analyze. Pure transport — no judgment.
func runInsightsPending(args []string) error {
	fs := flag.NewFlagSet("insights pending", flag.ExitOnError)
	asOf := fs.String("as-of", "", "evaluate as of this date (default: today)")
	ledgerDir := fs.String("ledger-dir", defaultInsightsDir, "ledger directory to read")
	if err := fs.Parse(args); err != nil {
		return err
	}
	now := time.Now()
	if *asOf != "" {
		t, err := time.Parse("2006-01-02", *asOf)
		if err != nil {
			return fmt.Errorf("--as-of: %w", err)
		}
		now = t
	}

	pending, err := pendingHypotheses(*ledgerDir, now)
	if err != nil {
		return err
	}
	if len(pending) == 0 {
		fmt.Println("No pending hypotheses.")
		return nil
	}

	// Only load videos.json when at least one pending hypothesis cites video
	// evidence. A cross-surface ledger (Buffer/LinkedIn/source hypotheses with
	// no evidence_video_ids) must not require a fresh YouTube snapshot to grade —
	// the caller (/flywheel) supplies cross-surface evidence at grade time.
	byID := make(map[string]Video)
	if anyHasVideoEvidence(pending) {
		data, err := loadData(videosJSONPath)
		if err != nil {
			return err
		}
		for _, v := range data.Videos {
			byID[v.ID] = v
		}
	}

	for _, h := range pending {
		fmt.Printf("─── %s ───────────────────────────────────────────\n", h.ID)
		if h.Surface != "" {
			fmt.Printf("  Surface:       %s\n", h.Surface)
		}
		fmt.Printf("  Cohort:        %s\n", h.Cohort)
		fmt.Printf("  Prediction:    %s\n", h.Prediction)
		if h.Metric != "" {
			fmt.Printf("  Metric:        %s (direction: %s)\n", h.Metric, h.Direction)
		}
		fmt.Printf("  Evaluate after: %s\n", h.EvaluateAfter.Format("2006-01-02"))
		if len(h.EvidenceVideoIDs) > 0 {
			fmt.Println("  Evidence videos (current metrics):")
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "    \tVIDEO_ID\tVIEWS\tWATCH_MIN\tRET%\tNET_SUBS\tTITLE")
			for _, vid := range h.EvidenceVideoIDs {
				v, ok := byID[vid]
				if !ok {
					fmt.Fprintf(w, "    \t%s\t(not found)\t\t\t\t\n", vid)
					continue
				}
				fmt.Fprintf(w, "    \t%s\t%d\t%.0f\t%.1f\t%+d\t%s\n",
					v.ID, v.ViewCount, v.EstimatedMinutesWatched,
					v.AverageViewPercentage,
					v.SubscribersGained-v.SubscribersLost,
					truncate(v.Title, 50))
			}
			w.Flush()

			// Surface dimensional context when present. Pure transport — the
			// model decides what these numbers mean.
			for _, vid := range h.EvidenceVideoIDs {
				v, ok := byID[vid]
				if !ok {
					continue
				}
				if len(v.DailyMetrics) > 0 {
					fmt.Printf("    %s — last-7-days view trajectory:\n", vid)
					printDailyTail(v.DailyMetrics, 7)
				}
				if len(v.TrafficSources) > 0 {
					fmt.Printf("    %s — top traffic sources:\n", vid)
					printTopTrafficSources(v.TrafficSources, 3)
				}
				if len(v.SubStatusMetrics) > 0 {
					fmt.Printf("    %s — subscriber-status split:\n", vid)
					printSubStatus(v.SubStatusMetrics)
				}
			}
		}
		fmt.Println()
	}
	fmt.Printf("%d pending hypothesis(es). Grade with: insights grade <id> --verdict <v> --outcome \"<text>\"\n", len(pending))
	return nil
}

// anyHasVideoEvidence reports whether any hypothesis cites a YouTube video,
// gating the (potentially stale/absent) videos.json load.
func anyHasVideoEvidence(hs []Hypothesis) bool {
	for _, h := range hs {
		if len(h.EvidenceVideoIDs) > 0 {
			return true
		}
	}
	return false
}

func runInsightsGrade(args []string) error {
	fs := flag.NewFlagSet("insights grade", flag.ExitOnError)
	verdict := fs.String("verdict", "", "confirm | refute | inconclusive")
	outcome := fs.String("outcome", "", "free-form outcome description (required)")
	ledgerDir := fs.String("ledger-dir", defaultInsightsDir, "ledger directory to write")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: insights grade <hypothesis-id> --verdict <v> --outcome \"<text>\"")
	}
	id := fs.Arg(0)
	switch *verdict {
	case "confirm", "refute", "inconclusive":
	default:
		return fmt.Errorf("--verdict must be confirm|refute|inconclusive, got %q", *verdict)
	}
	if *outcome == "" {
		return fmt.Errorf("--outcome is required (describe what actually happened)")
	}
	if err := gradeHypothesis(*ledgerDir, id, *verdict, *outcome); err != nil {
		return err
	}
	fmt.Printf("Graded %s: %s — %s\n", id, *verdict, *outcome)
	return nil
}

func runInsightsNew(args []string) error {
	fs := flag.NewFlagSet("insights new", flag.ExitOnError)
	ledgerDir := fs.String("ledger-dir", defaultInsightsDir, "ledger directory to write")
	if err := fs.Parse(args); err != nil {
		return err
	}
	dateStr := ""
	if fs.NArg() > 0 {
		dateStr = fs.Arg(0)
	}
	var date time.Time
	if dateStr == "" {
		date = mostRecentMonday(time.Now())
	} else {
		t, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			return fmt.Errorf("invalid date %q (want YYYY-MM-DD): %w", dateStr, err)
		}
		date = t
	}

	path := filepath.Join(*ledgerDir, date.Format("2006-01-02")+".md")
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists; edit it directly", path)
	}

	body := fmt.Sprintf("# Insights — %s\n\nNotes / hypotheses for the week starting %s.\n",
		date.Format("2006-01-02"), date.Format("2006-01-02"))

	if err := writeInsightFile(path, &InsightFile{
		Date: date,
		Body: body,
	}); err != nil {
		return err
	}
	fmt.Printf("Created %s — edit it to add hypotheses with frontmatter.\n", path)
	return nil
}

// printDailyTail prints the most recent n DailyMetric rows in chronological
// order. The API returns rows in arbitrary order; we sort by Date string
// (YYYY-MM-DD sorts lexicographically as chronologically) and take the tail.
func printDailyTail(series []DailyMetric, n int) {
	sorted := make([]DailyMetric, len(series))
	copy(sorted, series)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Date < sorted[j].Date })
	start := 0
	if len(sorted) > n {
		start = len(sorted) - n
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "      \tDATE\tVIEWS\tWATCH_MIN\tRET%\tSUBS")
	for _, d := range sorted[start:] {
		fmt.Fprintf(w, "      \t%s\t%d\t%.0f\t%.1f\t%+d\n",
			d.Date, d.Views, d.EstimatedMinutes, d.AvgRetention, d.SubscribersGained-d.SubscribersLost)
	}
	w.Flush()
}

// printTopTrafficSources prints the n traffic-source buckets with the most
// views, sorted descending.
func printTopTrafficSources(m map[string]TrafficSourceMetric, n int) {
	type kv struct {
		k string
		v TrafficSourceMetric
	}
	all := make([]kv, 0, len(m))
	for k, v := range m {
		all = append(all, kv{k, v})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].v.Views > all[j].v.Views })
	limit := n
	if len(all) < limit {
		limit = len(all)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "      \tSOURCE\tVIEWS\tWATCH_MIN")
	for _, e := range all[:limit] {
		fmt.Fprintf(w, "      \t%s\t%d\t%.0f\n", e.k, e.v.Views, e.v.WatchMin)
	}
	w.Flush()
}

// printSubStatus emits the SUBSCRIBED vs UNSUBSCRIBED split (always two rows).
func printSubStatus(m map[string]TrafficSourceMetric) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "      \tSTATUS\tVIEWS\tWATCH_MIN")
	for _, status := range []string{"SUBSCRIBED", "UNSUBSCRIBED"} {
		v, ok := m[status]
		if !ok {
			continue
		}
		fmt.Fprintf(w, "      \t%s\t%d\t%.0f\n", status, v.Views, v.WatchMin)
	}
	w.Flush()
}

// mostRecentMonday returns the most recent Monday on or before now.
func mostRecentMonday(now time.Time) time.Time {
	day := int(now.Weekday())
	if day == 0 {
		day = 7
	}
	offset := day - 1
	monday := now.AddDate(0, 0, -offset)
	return time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, monday.Location())
}
