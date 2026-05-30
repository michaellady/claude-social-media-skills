package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var (
	titleWordRe = regexp.MustCompile(`[A-Za-z][A-Za-z0-9'$]*`)
	stopwords   = map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true, "but": true,
		"of": true, "in": true, "on": true, "at": true, "to": true, "for": true,
		"with": true, "is": true, "it": true, "as": true, "by": true, "i": true,
		"my": true, "we": true, "you": true, "our": true, "your": true, "this": true,
		"that": true, "from": true, "be": true, "are": true, "was": true, "can": true,
		"will": true, "has": true, "have": true, "not": true, "vs": false, // keep vs — signal word
	}
)

func extractWords(s string) []string {
	matches := titleWordRe.FindAllString(s, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		lower := strings.ToLower(m)
		if stopwords[lower] {
			continue
		}
		if len(lower) < 2 {
			continue
		}
		out = append(out, lower)
	}
	return out
}

func runTitles(dataPath string, filter VideoFilter, top int, by string, minViews int64) error {
	data, err := loadData(dataPath)
	if err != nil {
		return err
	}
	videos := filter.Apply(data.Videos)

	var less func(a, b Video) bool
	switch by {
	case "views":
		less = func(a, b Video) bool { return a.ViewCount > b.ViewCount }
	case "subs":
		less = func(a, b Video) bool { return a.SubscribersGained > b.SubscribersGained }
	case "retention":
		less = func(a, b Video) bool { return a.AverageViewPercentage > b.AverageViewPercentage }
	case "revenue":
		less = func(a, b Video) bool { return a.EstimatedRevenue > b.EstimatedRevenue }
	default:
		return fmt.Errorf("--by must be one of: views, subs, retention, revenue (got %q)", by)
	}

	pool := videos
	if by == "retention" && minViews == 0 {
		minViews = 100
	}
	if minViews > 0 {
		pool = filterMinViews(videos, minViews)
	}
	topVids := topN(pool, top, less)

	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Printf("  Title Analysis — Top %d by %s\n", top, by)
	if !filter.IsZero() {
		fmt.Printf("  Filter: %s\n", filter.Describe())
	}
	if minViews > 0 {
		fmt.Printf("  Min views: %d\n", minViews)
	}
	fmt.Println("═══════════════════════════════════════════════════════════")
	if len(topVids) == 0 {
		fmt.Println("  No videos match the criteria.")
		return nil
	}

	fmt.Println()
	fmt.Printf("── Top %d Titles ─────────────────────────────────────────\n", len(topVids))
	for i, v := range topVids {
		fmt.Printf("  %2d. [%s] %s\n", i+1, v.VideoType, truncate(v.Title, 60))
		switch by {
		case "views":
			fmt.Printf("      Views: %-8s  Subs: +%-4d  Retention: %.1f%%\n",
				fmtNum(v.ViewCount), v.SubscribersGained, v.AverageViewPercentage)
		case "subs":
			fmt.Printf("      Subs: +%-4d  Views: %-8s  Subs/1K: %.1f\n",
				v.SubscribersGained, fmtNum(v.ViewCount),
				subsPer1K(v))
		case "retention":
			fmt.Printf("      Retention: %5.1f%%  Views: %-8s  Avg Duration: %.0fs\n",
				v.AverageViewPercentage, fmtNum(v.ViewCount), v.AverageViewDuration)
		case "revenue":
			fmt.Printf("      Revenue: $%-6.2f  Views: %-8s  CPM: $%.2f\n",
				v.EstimatedRevenue, fmtNum(v.ViewCount), v.CPM)
		}
	}
	fmt.Println()

	// Keyword frequency in top N vs baseline (whole pool).
	topCounts := map[string]int{}
	for _, v := range topVids {
		seen := map[string]bool{}
		for _, w := range extractWords(v.Title) {
			if seen[w] {
				continue
			}
			seen[w] = true
			topCounts[w]++
		}
	}
	baseCounts := map[string]int{}
	for _, v := range pool {
		seen := map[string]bool{}
		for _, w := range extractWords(v.Title) {
			if seen[w] {
				continue
			}
			seen[w] = true
			baseCounts[w]++
		}
	}

	type kwScore struct {
		word     string
		topFreq  float64
		baseFreq float64
		lift     float64
		topCnt   int
	}
	var scored []kwScore
	topTotal := float64(len(topVids))
	baseTotal := float64(len(pool))
	if baseTotal == 0 {
		return nil
	}
	for w, c := range topCounts {
		if c < 3 {
			continue
		}
		topFreq := float64(c) / topTotal
		baseFreq := float64(baseCounts[w]) / baseTotal
		if baseFreq == 0 {
			baseFreq = 1e-6
		}
		scored = append(scored, kwScore{
			word: w, topFreq: topFreq, baseFreq: baseFreq,
			lift: topFreq / baseFreq, topCnt: c,
		})
	}
	sort.Slice(scored, func(i, j int) bool { return scored[i].lift > scored[j].lift })

	fmt.Println("── Overrepresented Keywords in Top Titles (≥3 appearances) ────")
	if len(scored) == 0 {
		fmt.Println("  No repeated keywords in top titles.")
	} else {
		fmt.Printf("  %-20s  %7s  %8s  %8s  %s\n", "Keyword", "In Top", "Top%", "Base%", "Lift")
		limit := 15
		if len(scored) < limit {
			limit = len(scored)
		}
		for i := 0; i < limit; i++ {
			s := scored[i]
			fmt.Printf("  %-20s  %7d  %7.1f%%  %7.1f%%  %.1fx\n",
				s.word, s.topCnt, s.topFreq*100, s.baseFreq*100, s.lift)
		}
	}
	fmt.Println()

	// Title shape stats.
	var totalLen float64
	var withQ, withNum, capsWords int
	for _, v := range topVids {
		totalLen += float64(len(v.Title))
		if strings.Contains(v.Title, "?") {
			withQ++
		}
		if numberRe.MatchString(v.Title) {
			withNum++
		}
		capsWords += countCapsWords(v.Title)
	}
	fmt.Println("── Title Shape Stats (Top N) ─────────────────────────────")
	fmt.Printf("  Avg title length:  %.0f chars\n", totalLen/topTotal)
	fmt.Printf("  With '?':          %d / %d\n", withQ, len(topVids))
	fmt.Printf("  With numbers:      %d / %d\n", withNum, len(topVids))
	fmt.Printf("  Avg CAPS words:    %.1f\n", float64(capsWords)/topTotal)
	return nil
}

func subsPer1K(v Video) float64 {
	if v.ViewCount == 0 {
		return 0
	}
	return float64(v.SubscribersGained) / float64(v.ViewCount) * 1000
}
