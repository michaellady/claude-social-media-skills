package main

import (
	"fmt"
	"sort"
	"strings"
)

func runDiff(currentPath, priorPath string, filter VideoFilter) error {
	current, err := loadData(currentPath)
	if err != nil {
		return err
	}
	prior, err := loadData(priorPath)
	if err != nil {
		return fmt.Errorf("loading snapshot %s: %w", priorPath, err)
	}

	currentVids := filter.Apply(current.Videos)
	priorVids := filter.Apply(prior.Videos)

	priorByID := map[string]Video{}
	for _, v := range priorVids {
		priorByID[v.ID] = v
	}
	currentByID := map[string]bool{}
	for _, v := range currentVids {
		currentByID[v.ID] = true
	}

	type delta struct {
		v              Video
		newVideo       bool
		removed        bool
		viewsDelta     int64
		subsDelta      int64
		revenueDelta   float64
		watchTimeDelta float64
	}
	var deltas []delta
	var totalViewsDelta, totalSubsDelta int64
	var totalRevDelta, totalWatchDelta float64
	newCount := 0

	for _, cur := range currentVids {
		prev, had := priorByID[cur.ID]
		if !had {
			newCount++
			deltas = append(deltas, delta{
				v: cur, newVideo: true,
				viewsDelta:     cur.ViewCount,
				subsDelta:      cur.SubscribersGained - cur.SubscribersLost,
				revenueDelta:   cur.EstimatedRevenue,
				watchTimeDelta: cur.EstimatedMinutesWatched,
			})
			totalViewsDelta += cur.ViewCount
			totalSubsDelta += cur.SubscribersGained - cur.SubscribersLost
			totalRevDelta += cur.EstimatedRevenue
			totalWatchDelta += cur.EstimatedMinutesWatched
			continue
		}
		d := delta{
			v:              cur,
			viewsDelta:     cur.ViewCount - prev.ViewCount,
			subsDelta:      (cur.SubscribersGained - cur.SubscribersLost) - (prev.SubscribersGained - prev.SubscribersLost),
			revenueDelta:   cur.EstimatedRevenue - prev.EstimatedRevenue,
			watchTimeDelta: cur.EstimatedMinutesWatched - prev.EstimatedMinutesWatched,
		}
		deltas = append(deltas, d)
		totalViewsDelta += d.viewsDelta
		totalSubsDelta += d.subsDelta
		totalRevDelta += d.revenueDelta
		totalWatchDelta += d.watchTimeDelta
	}
	removedCount := 0
	for id := range priorByID {
		if !currentByID[id] {
			removedCount++
		}
	}

	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Printf("  Delta Report\n")
	fmt.Printf("  Current:  %s (fetched %s)\n", currentPath, current.FetchedAt.Format("2006-01-02 15:04"))
	fmt.Printf("  Prior:    %s (fetched %s)\n", priorPath, prior.FetchedAt.Format("2006-01-02 15:04"))
	if !filter.IsZero() {
		fmt.Printf("  Filter:   %s\n", filter.Describe())
	}
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Printf("  Videos:        %d → %d (%+d new, %d removed)\n",
		len(priorVids), len(currentVids), newCount, removedCount)
	fmt.Printf("  Views:         %+s\n", signedNum(totalViewsDelta))
	fmt.Printf("  Net subs:      %+s\n", signedNum(totalSubsDelta))
	fmt.Printf("  Watch time:    %+.0f minutes\n", totalWatchDelta)
	fmt.Printf("  Revenue:       %+s\n", signedDollars(totalRevDelta))
	fmt.Println()

	// Top movers by views delta.
	fmt.Println("── Top 10 Videos by Views Gained ─────────────────────────")
	sort.Slice(deltas, func(i, j int) bool { return deltas[i].viewsDelta > deltas[j].viewsDelta })
	limit := 10
	if len(deltas) < limit {
		limit = len(deltas)
	}
	for i := 0; i < limit; i++ {
		d := deltas[i]
		if d.viewsDelta <= 0 {
			break
		}
		tag := ""
		if d.newVideo {
			tag = " [NEW]"
		}
		fmt.Printf("  %2d.%s [%s] %s\n", i+1, tag, d.v.VideoType, truncate(d.v.Title, 55))
		fmt.Printf("      Views: +%s  Subs: %+d  Revenue: %+s\n",
			signedNum(d.viewsDelta), d.subsDelta, signedDollars(d.revenueDelta))
	}
	fmt.Println()

	if newCount > 0 {
		fmt.Printf("── %d New Video(s) Since Snapshot ────────────────────────\n", newCount)
		var newVids []delta
		for _, d := range deltas {
			if d.newVideo {
				newVids = append(newVids, d)
			}
		}
		sort.Slice(newVids, func(i, j int) bool { return newVids[i].v.PublishedAt.After(newVids[j].v.PublishedAt) })
		max := 20
		if len(newVids) < max {
			max = len(newVids)
		}
		for i := 0; i < max; i++ {
			d := newVids[i]
			fmt.Printf("  %s  [%s]  %s views  %s\n",
				d.v.PublishedAt.Format("2006-01-02"),
				d.v.VideoType,
				fmtNum(d.v.ViewCount),
				truncate(d.v.Title, 50),
			)
		}
	}
	return nil
}

func runComparePeriods(dataPath, spec string, extra []string, outer VideoFilter) error {
	data, err := loadData(dataPath)
	if err != nil {
		return err
	}
	ranges, err := parseCompareSpec(spec, extra)
	if err != nil {
		return err
	}

	var all []*periodStats
	for _, r := range ranges {
		merged := mergeCompareFilter(outer, r.filter)
		p := &periodStats{label: r.label, filter: merged}
		p.videos = p.filter.Apply(data.Videos)
		fake := &ChannelData{
			ChannelID: data.ChannelID, ChannelTitle: data.ChannelTitle,
			HasAnalytics: data.HasAnalytics, Videos: p.videos,
		}
		p.result = analyze(fake)
		p.subs1K, p.totalVws = perTypeSubsPer1K(p.videos, p.result)
		all = append(all, p)
	}

	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Printf("  Period Comparison: %d ranges\n", len(all))
	fmt.Println("═══════════════════════════════════════════════════════════")
	// Header row with period labels.
	fmt.Printf("  %-28s", "Metric")
	for _, p := range all {
		fmt.Printf("  %14s", p.label)
	}
	fmt.Println()
	fmt.Printf("  %-28s", "Videos")
	for _, p := range all {
		fmt.Printf("  %14d", p.result.TotalVideos)
	}
	fmt.Println()
	rowInt64(all, "Total views", func(p *periodStats) int64 {
		var v int64
		for _, vid := range p.videos {
			v += vid.ViewCount
		}
		return v
	})
	rowFloat2(all, "Avg retention %", func(p *periodStats) float64 { return p.result.WatchTime.AvgRetention })
	rowInt64(all, "Net subs", func(p *periodStats) int64 { return p.result.WatchTime.NetSubscribers })
	rowFloat2(all, "Total watch (min)", func(p *periodStats) float64 { return p.result.WatchTime.TotalMinutes })
	rowMoney(all, "Revenue", func(p *periodStats) float64 { return p.result.Revenue.TotalRevenue })
	for _, t := range []string{"short", "long-form", "live"} {
		label := fmt.Sprintf("Subs/1K views (%s)", t)
		rowFloat2(all, label, func(p *periodStats) float64 { return p.subs1K[t] })
	}
	return nil
}

// mergeCompareFilter overlays a per-range date window on top of an outer
// filter. The range OWNS the date window (Since/Until); everything else
// (cohorts, types, duration bounds, exclude IDs, cohort assignments) is
// inherited so users can scope --compare-periods with --cohort/--type/etc.
func mergeCompareFilter(outer, rangeFilter VideoFilter) VideoFilter {
	merged := outer
	merged.Since = rangeFilter.Since
	merged.Until = rangeFilter.Until
	return merged
}

type compareRange struct {
	label  string
	filter VideoFilter
}

func parseCompareSpec(spec string, extra []string) ([]compareRange, error) {
	// Accept either one combined string or spec + extra args.
	tokens := []string{spec}
	tokens = append(tokens, extra...)
	// Each token is "YYYY-MM-DD..YYYY-MM-DD".
	var ranges []compareRange
	for _, t := range tokens {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		parts := strings.Split(t, "..")
		if len(parts) != 2 {
			return nil, fmt.Errorf("period %q must be YYYY-MM-DD..YYYY-MM-DD", t)
		}
		start, err := parseDateStr(parts[0])
		if err != nil {
			return nil, fmt.Errorf("period %q: %w", t, err)
		}
		end, err := parseDateStr(parts[1])
		if err != nil {
			return nil, fmt.Errorf("period %q: %w", t, err)
		}
		ranges = append(ranges, compareRange{
			label: fmt.Sprintf("%s..%s", parts[0], parts[1]),
			filter: VideoFilter{
				Since: start,
				Until: end.Add(24*60*60*1e9 - 1e9), // +1 day - 1s
			},
		})
	}
	if len(ranges) < 2 {
		return nil, fmt.Errorf("--compare-periods requires at least two YYYY-MM-DD..YYYY-MM-DD ranges")
	}
	return ranges, nil
}

func perTypeSubsPer1K(videos []Video, r *AnalysisResult) (map[string]float64, map[string]int64) {
	rate := map[string]float64{}
	totals := map[string]int64{}
	subs := map[string]int64{"short": 0, "long-form": 0, "live": 0}
	views := map[string]int64{"short": 0, "long-form": 0, "live": 0}
	for _, v := range videos {
		subs[v.VideoType] += v.SubscribersGained
		views[v.VideoType] += v.ViewCount
	}
	for t := range subs {
		totals[t] = views[t]
		if views[t] > 0 {
			rate[t] = float64(subs[t]) / float64(views[t]) * 1000
		}
	}
	return rate, totals
}

func rowInt64(ps []*periodStats, label string, f func(*periodStats) int64) {
	fmt.Printf("  %-28s", label)
	for _, p := range ps {
		fmt.Printf("  %14s", fmtNum(f(p)))
	}
	fmt.Println()
}

func rowFloat2(ps []*periodStats, label string, f func(*periodStats) float64) {
	fmt.Printf("  %-28s", label)
	for _, p := range ps {
		fmt.Printf("  %14.2f", f(p))
	}
	fmt.Println()
}

func rowMoney(ps []*periodStats, label string, f func(*periodStats) float64) {
	fmt.Printf("  %-28s", label)
	for _, p := range ps {
		fmt.Printf("  %13s", fmt.Sprintf("$%.2f", f(p)))
	}
	fmt.Println()
}

func signedNum(n int64) string {
	if n < 0 {
		return "-" + fmtNum(-n)
	}
	return fmtNum(n)
}

func signedDollars(x float64) string {
	return fmt.Sprintf("$%.2f", x)
}

type periodStats struct {
	label    string
	filter   VideoFilter
	videos   []Video
	result   *AnalysisResult
	subs1K   map[string]float64
	totalVws map[string]int64
}
