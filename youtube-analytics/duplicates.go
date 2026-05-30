package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

var whitespaceRe = regexp.MustCompile(`\s+`)

// normalizeTitle collapses whitespace, lowercases, and strips common trailing markers
// so that "Foo Bar #shorts" and "foo  bar" compare equal.
func normalizeTitle(s string) string {
	s = strings.ToLower(s)
	s = whitespaceRe.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	for _, suffix := range []string{" #shorts", " #short", " #live"} {
		s = strings.TrimSuffix(s, suffix)
	}
	return s
}

type dupeGroup struct {
	titleNormalized string
	videos          []Video
}

func runFindDuplicates(dataPath string, filter VideoFilter, durTolSec int, timeWindow time.Duration) error {
	data, err := loadData(dataPath)
	if err != nil {
		return err
	}
	videos := filter.Apply(data.Videos)

	byTitle := map[string][]Video{}
	for _, v := range videos {
		key := normalizeTitle(v.Title)
		if key == "" {
			continue
		}
		byTitle[key] = append(byTitle[key], v)
	}

	var groups []dupeGroup
	for title, vs := range byTitle {
		if len(vs) < 2 {
			continue
		}
		// Find clusters within the group where each pair is within both tolerances.
		sort.Slice(vs, func(i, j int) bool { return vs[i].PublishedAt.Before(vs[j].PublishedAt) })
		used := make([]bool, len(vs))
		for i := 0; i < len(vs); i++ {
			if used[i] {
				continue
			}
			cluster := []Video{vs[i]}
			used[i] = true
			for j := i + 1; j < len(vs); j++ {
				if used[j] {
					continue
				}
				if abs(vs[i].DurationSeconds-vs[j].DurationSeconds) > durTolSec {
					continue
				}
				if vs[j].PublishedAt.Sub(vs[i].PublishedAt) > timeWindow {
					continue
				}
				cluster = append(cluster, vs[j])
				used[j] = true
			}
			if len(cluster) >= 2 {
				groups = append(groups, dupeGroup{titleNormalized: title, videos: cluster})
			}
		}
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].videos[0].PublishedAt.After(groups[j].videos[0].PublishedAt)
	})

	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Printf("  Duplicate Upload Detection\n")
	fmt.Printf("  (duration tolerance: %ds, time window: %s)\n", durTolSec, timeWindow)
	if !filter.IsZero() {
		fmt.Printf("  Filter: %s\n", filter.Describe())
	}
	fmt.Println("═══════════════════════════════════════════════════════════")
	if len(groups) == 0 {
		fmt.Println("  No near-duplicate uploads detected.")
		return nil
	}
	fmt.Printf("  Found %d duplicate cluster(s):\n\n", len(groups))
	for i, g := range groups {
		fmt.Printf("  Cluster %d: %q\n", i+1, g.videos[0].Title)
		for _, v := range g.videos {
			fmt.Printf("    %s  %-12s  %6s views  retention %5.1f%%  (%ds)\n",
				v.PublishedAt.Format("2006-01-02 15:04:05"),
				v.ID,
				fmtNum(v.ViewCount),
				v.AverageViewPercentage,
				v.DurationSeconds,
			)
		}
		fmt.Println()
	}
	return nil
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
