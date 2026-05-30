package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"
)

func cmdCohort(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("cohort requires a subcommand: list|show|assign|unassign|auto|report")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "list":
		return runCohortList(rest)
	case "show":
		return runCohortShow(rest)
	case "assign":
		return runCohortAssign(rest, false)
	case "unassign":
		return runCohortAssign(rest, true)
	case "auto":
		return runCohortAuto(rest)
	case "report":
		return runCohortReport(rest)
	default:
		return fmt.Errorf("unknown cohort subcommand: %s", sub)
	}
}

// runCohortList prints all defined cohorts with member counts.
func runCohortList(args []string) error {
	fs := flag.NewFlagSet("cohort list", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	cohorts, err := loadCohorts(cohortsYAMLPath)
	if err != nil {
		return err
	}
	assignments, err := loadAssignments(cohortAssignmentsPath)
	if err != nil {
		return err
	}
	counts := map[string]int{}
	for _, ids := range assignments {
		for _, id := range ids {
			counts[id]++
		}
	}
	if len(cohorts) == 0 {
		fmt.Println("No cohorts defined. Edit data/cohorts.yaml to create them.")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tMEMBERS\tRULES")
	for _, c := range cohorts {
		fmt.Fprintf(w, "%s\t%s\t%d\t%d\n", c.ID, c.Name, counts[c.ID], len(c.Rules))
	}
	return w.Flush()
}

// runCohortShow prints videos in a cohort with key metrics. Filter flags
// (--since, --type, etc.) narrow the listing within the cohort.
func runCohortShow(args []string) error {
	fs := flag.NewFlagSet("cohort show", flag.ExitOnError)
	ff := addFilterFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: cohort show <cohort-id> [filter flags]")
	}
	id := fs.Arg(0)
	filter, err := ff.build()
	if err != nil {
		return err
	}
	data, err := loadData(videosJSONPath)
	if err != nil {
		return err
	}
	assignments, err := loadAssignments(cohortAssignmentsPath)
	if err != nil {
		return err
	}

	scoped := filter.Apply(data.Videos)
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "VIDEO_ID\tTYPE\tPUBLISHED\tVIEWS\tRETENTION%\tSUBS\tTITLE")
	count := 0
	for _, v := range scoped {
		if !contains(assignments[v.ID], id) {
			continue
		}
		count++
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%.1f\t%+d\t%s\n",
			v.ID, v.VideoType, v.PublishedAt.Format("2006-01-02"),
			v.ViewCount, v.AverageViewPercentage,
			v.SubscribersGained-v.SubscribersLost, truncate(v.Title, 60))
	}
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Printf("\n%d videos in cohort %q", count, id)
	if !filter.IsZero() {
		fmt.Printf(" %s", filter.Describe())
	}
	fmt.Println()
	return nil
}

// runCohortAssign manually adds (or removes, when remove=true) a cohort to a set of videos.
func runCohortAssign(args []string, remove bool) error {
	fs := flag.NewFlagSet("cohort assign", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 2 {
		verb := "assign"
		if remove {
			verb = "unassign"
		}
		return fmt.Errorf("usage: cohort %s <cohort-id> <video-id>...", verb)
	}
	cohortID := fs.Arg(0)
	videoIDs := fs.Args()[1:]

	cohorts, err := loadCohorts(cohortsYAMLPath)
	if err != nil {
		return err
	}
	known := false
	for _, c := range cohorts {
		if c.ID == cohortID {
			known = true
			break
		}
	}
	if !known {
		return fmt.Errorf("cohort %q not defined in %s", cohortID, cohortsYAMLPath)
	}

	assignments, err := loadAssignments(cohortAssignmentsPath)
	if err != nil {
		return err
	}
	changed := 0
	for _, vid := range videoIDs {
		current := assignments[vid]
		if remove {
			next := make([]string, 0, len(current))
			for _, id := range current {
				if id != cohortID {
					next = append(next, id)
				}
			}
			if len(next) != len(current) {
				if len(next) == 0 {
					delete(assignments, vid)
				} else {
					assignments[vid] = next
				}
				changed++
			}
		} else {
			if !contains(current, cohortID) {
				assignments[vid] = append(current, cohortID)
				changed++
			}
		}
	}
	if err := saveAssignments(cohortAssignmentsPath, assignments); err != nil {
		return err
	}
	verb := "assigned to"
	if remove {
		verb = "removed from"
	}
	fmt.Printf("%d video(s) %s cohort %q\n", changed, verb, cohortID)
	return nil
}

// runCohortAuto re-derives assignments from rules and prints the diff.
func runCohortAuto(args []string) error {
	fs := flag.NewFlagSet("cohort auto", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "print diff without writing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cohorts, err := loadCohorts(cohortsYAMLPath)
	if err != nil {
		return err
	}
	if len(cohorts) == 0 {
		return fmt.Errorf("no cohorts defined in %s", cohortsYAMLPath)
	}
	data, err := loadData(videosJSONPath)
	if err != nil {
		return err
	}
	existing, err := loadAssignments(cohortAssignmentsPath)
	if err != nil {
		return err
	}
	next := autoAssignAll(cohorts, data.Videos, existing)

	added, removed := assignmentDiff(existing, next)
	fmt.Printf("Auto-assignment diff: +%d added, -%d removed\n", added, removed)
	printDiffSamples(existing, next)

	if *dryRun {
		fmt.Println("(dry-run: no changes written)")
		return nil
	}
	if err := saveAssignments(cohortAssignmentsPath, next); err != nil {
		return err
	}
	fmt.Printf("Wrote %s (%d videos with at least one cohort)\n", cohortAssignmentsPath, len(next))
	return nil
}

// runCohortReport prints a per-cohort performance table for the filtered slice.
func runCohortReport(args []string) error {
	fs := flag.NewFlagSet("cohort report", flag.ExitOnError)
	ff := addFilterFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	filter, err := ff.build()
	if err != nil {
		return err
	}
	// Reuse the filter's assignments when --cohort populated them; otherwise
	// load directly so the per-cohort tally still works without --cohort.
	assignments := filter.CohortAssignments
	if assignments == nil {
		assignments, err = loadAssignments(cohortAssignmentsPath)
		if err != nil {
			return err
		}
	}
	cohorts, err := loadCohorts(cohortsYAMLPath)
	if err != nil {
		return err
	}
	data, err := loadData(videosJSONPath)
	if err != nil {
		return err
	}

	videos := filter.Apply(data.Videos)

	stats := make(map[string]*cohortStats, len(cohorts))
	for _, c := range cohorts {
		stats[c.ID] = &cohortStats{id: c.ID, name: c.Name}
	}
	uncategorized := &cohortStats{id: "(uncategorized)", name: "videos not in any cohort"}

	for _, v := range videos {
		ids := assignments[v.ID]
		if len(ids) == 0 {
			uncategorized.add(v)
			continue
		}
		for _, id := range ids {
			s, ok := stats[id]
			if !ok {
				continue // orphan
			}
			s.add(v)
		}
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	if !filter.IsZero() {
		fmt.Fprintf(w, "Filter %s\n\n", filter.Describe())
	}
	fmt.Fprintln(w, "COHORT\tVIDEOS\tVIEWS\tWATCH_MIN\tAVG_RET%\tNET_SUBS\tSUBS/1K\tREVENUE")
	all := make([]*cohortStats, 0, len(stats)+1)
	for _, s := range stats {
		all = append(all, s)
	}
	if uncategorized.count > 0 {
		all = append(all, uncategorized)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].views > all[j].views })
	for _, s := range all {
		ret := 0.0
		if s.retDenom > 0 {
			ret = s.retention / s.retDenom
		}
		subsPer1K := 0.0
		if s.views > 0 {
			subsPer1K = float64(s.subs) / float64(s.views) * 1000
		}
		fmt.Fprintf(w, "%s\t%d\t%d\t%.0f\t%.1f\t%+d\t%.2f\t$%.2f\n",
			s.id, s.count, s.views, s.watchMin, ret, s.subs, subsPer1K, s.revenue)
	}
	return w.Flush()
}

// cohortStats accumulates per-cohort metrics. Retention is view-weighted.
type cohortStats struct {
	id        string
	name      string
	count     int
	views     int64
	watchMin  float64
	retention float64 // sum of (avg% * views)
	retDenom  float64 // sum of views (denominator for the weighted retention avg)
	subs      int64
	revenue   float64
}

func (s *cohortStats) add(v Video) {
	s.count++
	s.views += v.ViewCount
	s.watchMin += v.EstimatedMinutesWatched
	if v.ViewCount > 0 {
		s.retention += v.AverageViewPercentage * float64(v.ViewCount)
		s.retDenom += float64(v.ViewCount)
	}
	s.subs += v.SubscribersGained - v.SubscribersLost
	s.revenue += v.EstimatedRevenue
}

// assignmentDiff counts (added, removed) assignment edges between two maps.
func assignmentDiff(prev, next map[string][]string) (added, removed int) {
	keys := map[string]struct{}{}
	for k := range prev {
		keys[k] = struct{}{}
	}
	for k := range next {
		keys[k] = struct{}{}
	}
	for k := range keys {
		p := setOf(prev[k])
		n := setOf(next[k])
		for id := range n {
			if !p[id] {
				added++
			}
		}
		for id := range p {
			if !n[id] {
				removed++
			}
		}
	}
	return added, removed
}

// printDiffSamples prints up to 5 added/removed examples for human review.
func printDiffSamples(prev, next map[string][]string) {
	type edge struct{ video, cohort, op string }
	var edges []edge
	keys := map[string]struct{}{}
	for k := range prev {
		keys[k] = struct{}{}
	}
	for k := range next {
		keys[k] = struct{}{}
	}
	for k := range keys {
		p := setOf(prev[k])
		n := setOf(next[k])
		for id := range n {
			if !p[id] {
				edges = append(edges, edge{k, id, "+"})
			}
		}
		for id := range p {
			if !n[id] {
				edges = append(edges, edge{k, id, "-"})
			}
		}
	}
	if len(edges) == 0 {
		return
	}
	sort.Slice(edges, func(i, j int) bool { return edges[i].op < edges[j].op })
	fmt.Println("Sample changes:")
	limit := 10
	if len(edges) < limit {
		limit = len(edges)
	}
	for _, e := range edges[:limit] {
		fmt.Printf("  %s %s -> %s\n", e.op, e.video, e.cohort)
	}
	if len(edges) > limit {
		fmt.Printf("  ...and %d more\n", len(edges)-limit)
	}
}

func setOf(s []string) map[string]bool {
	m := map[string]bool{}
	for _, v := range s {
		m[v] = true
	}
	return m
}

func contains(s []string, x string) bool {
	for _, v := range s {
		if v == x {
			return true
		}
	}
	return false
}

