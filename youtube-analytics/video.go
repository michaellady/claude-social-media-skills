package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func runVideo(dataPath, id string) error {
	data, err := loadData(dataPath)
	if err != nil {
		return err
	}
	var found *Video
	for i := range data.Videos {
		if data.Videos[i].ID == id {
			found = &data.Videos[i]
			break
		}
	}
	if found == nil {
		return fmt.Errorf("video %q not found in %s", id, dataPath)
	}
	v := *found

	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Printf("  %s\n", v.Title)
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Printf("  ID:            %s\n", v.ID)
	fmt.Printf("  URL:           https://youtube.com/watch?v=%s\n", v.ID)
	fmt.Printf("  Type:          %s\n", v.VideoType)
	fmt.Printf("  Published:     %s\n", v.PublishedAt.Format("2006-01-02 15:04:05 MST"))
	fmt.Printf("  Duration:      %ds (%s)\n", v.DurationSeconds, v.Duration)
	fmt.Println()

	fmt.Println("── Engagement ────────────────────────────────────────────")
	fmt.Printf("  Views:         %s\n", fmtNum(v.ViewCount))
	fmt.Printf("  Likes:         %s\n", fmtNum(v.LikeCount))
	fmt.Printf("  Comments:      %s\n", fmtNum(v.CommentCount))
	fmt.Printf("  Engagement:    %.2f%%\n", engagementRate(v)*100)
	fmt.Println()

	if v.EstimatedMinutesWatched > 0 || v.AverageViewPercentage > 0 || v.SubscribersGained > 0 {
		fmt.Println("── Analytics ─────────────────────────────────────────────")
		fmt.Printf("  Watch time:    %.0f minutes (%.1f hours)\n", v.EstimatedMinutesWatched, v.EstimatedMinutesWatched/60)
		fmt.Printf("  Avg duration:  %.0f seconds\n", v.AverageViewDuration)
		fmt.Printf("  Retention:     %.1f%%\n", v.AverageViewPercentage)
		fmt.Printf("  Subs gained:   +%d\n", v.SubscribersGained)
		fmt.Printf("  Subs lost:     -%d\n", v.SubscribersLost)
		fmt.Printf("  Net subs:      %+d\n", v.SubscribersGained-v.SubscribersLost)
		fmt.Printf("  Subs/1K views: %.2f\n", subsPer1K(v))
		fmt.Println()
	}

	if v.EstimatedRevenue > 0 || v.AdImpressions > 0 {
		fmt.Println("── Monetization ──────────────────────────────────────────")
		fmt.Printf("  Revenue:       $%.4f\n", v.EstimatedRevenue)
		fmt.Printf("  CPM:           $%.2f\n", v.CPM)
		fmt.Printf("  Ad impressions:%s\n", fmtNum(v.AdImpressions))
		fmt.Printf("  Monetized plays:%s\n", fmtNum(v.MonetizedPlaybacks))
		fmt.Println()
	}

	if len(v.SubStatusMetrics) > 0 {
		fmt.Println("── Subscriber-Status Split ───────────────────────────────")
		sub := v.SubStatusMetrics["SUBSCRIBED"]
		uns := v.SubStatusMetrics["UNSUBSCRIBED"]
		total := sub.Views + uns.Views
		if total > 0 {
			fmt.Printf("  SUBSCRIBED:    %s views (%.1f%%) — %.0f watch-min\n",
				fmtNum(sub.Views), float64(sub.Views)/float64(total)*100, sub.WatchMin)
			fmt.Printf("  UNSUBSCRIBED:  %s views (%.1f%%) — %.0f watch-min\n",
				fmtNum(uns.Views), float64(uns.Views)/float64(total)*100, uns.WatchMin)
		}
		// Conversion against UNSUBSCRIBED is the meaningful subs/1K — SUBSCRIBED viewers can't sub again.
		if uns.Views > 0 {
			fmt.Printf("  Subs/1K UNSUBSCRIBED: %.2f (cleaner conversion signal than overall subs/1K)\n",
				float64(v.SubscribersGained)/float64(uns.Views)*1000)
		}
		fmt.Println()
	}

	if len(v.TrafficSources) > 0 {
		fmt.Println("── Traffic Sources ───────────────────────────────────────")
		printTrafficSourcesAll(v.TrafficSources)
		fmt.Println()
	}

	if len(v.DailyMetrics) > 0 {
		fmt.Println("── Last 14 Days ──────────────────────────────────────────")
		printDailyTrajectory(v.DailyMetrics, 14)
		fmt.Println()
	}

	memberships := videoCohorts(v.ID)
	if len(memberships) > 0 {
		fmt.Println("── Cohorts ───────────────────────────────────────────────")
		fmt.Printf("  %s\n", strings.Join(memberships, ", "))
		fmt.Println()
	}

	// Comments surface qualitative signal. The model interprets sentiment;
	// Go just shuttles the text in like-rank order.
	if comments, fetchedAt := videoCommentsFor(v.ID); len(comments) > 0 {
		fmt.Println("── Top Comments ──────────────────────────────────────────")
		fmt.Printf("  (fetched %s; %d total)\n", fetchedAt.Format("2006-01-02"), len(comments))
		limit := 5
		if len(comments) < limit {
			limit = len(comments)
		}
		for _, c := range comments[:limit] {
			meta := fmt.Sprintf("%s · 👍 %d · %s", c.AuthorName, c.LikeCount, c.PublishedAt.Format("2006-01-02"))
			if c.ReplyCount > 0 {
				meta += fmt.Sprintf(" · 💬 %d replies", c.ReplyCount)
			}
			fmt.Printf("  %s\n", meta)
			// Indent each line of the comment body so multi-line text is readable.
			for _, line := range strings.Split(strings.TrimSpace(c.Text), "\n") {
				fmt.Printf("    %s\n", line)
			}
			fmt.Println()
		}
	}

	fmt.Println("── Tags ──────────────────────────────────────────────────")
	if len(v.Tags) == 0 {
		fmt.Println("  (no tags)")
	} else {
		fmt.Printf("  %s\n", strings.Join(v.Tags, ", "))
	}
	fmt.Println()

	fmt.Println("── Description ───────────────────────────────────────────")
	if v.Description == "" {
		fmt.Println("  (no description)")
	} else {
		for _, line := range strings.Split(v.Description, "\n") {
			fmt.Printf("  %s\n", line)
		}
	}
	fmt.Println()

	if v.ThumbnailURL != "" {
		fmt.Printf("Thumbnail: %s\n", v.ThumbnailURL)
	}
	return nil
}

// printTrafficSourcesAll prints every traffic-source bucket with its share of views.
func printTrafficSourcesAll(m map[string]TrafficSourceMetric) {
	type kv struct {
		k string
		v TrafficSourceMetric
	}
	all := make([]kv, 0, len(m))
	var totalViews int64
	for k, v := range m {
		all = append(all, kv{k, v})
		totalViews += v.Views
	}
	sort.Slice(all, func(i, j int) bool { return all[i].v.Views > all[j].v.Views })
	for _, e := range all {
		share := 0.0
		if totalViews > 0 {
			share = float64(e.v.Views) / float64(totalViews) * 100
		}
		fmt.Printf("  %-20s %s views (%.1f%%) — %.0f watch-min\n",
			e.k, fmtNum(e.v.Views), share, e.v.WatchMin)
	}
}

// printDailyTrajectory prints the most-recent n daily metrics in chronological order.
func printDailyTrajectory(series []DailyMetric, n int) {
	sorted := make([]DailyMetric, len(series))
	copy(sorted, series)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Date < sorted[j].Date })
	start := 0
	if len(sorted) > n {
		start = len(sorted) - n
	}
	for _, d := range sorted[start:] {
		fmt.Printf("  %s   views=%-6d watch=%5.0fm  ret=%4.1f%%  subs=%+d\n",
			d.Date, d.Views, d.EstimatedMinutes, d.AvgRetention,
			d.SubscribersGained-d.SubscribersLost)
	}
}

// videoCommentsFor reads data/comments.json and returns the comments for the
// given video plus the per-video fetched_at timestamp. Returns (nil, zero) if
// the file is missing, unparseable, or has no entry for the video.
func videoCommentsFor(videoID string) ([]Comment, time.Time) {
	cf, err := loadComments(commentsJSONPath)
	if err != nil {
		return nil, time.Time{}
	}
	vc, ok := cf.Videos[videoID]
	if !ok {
		return nil, time.Time{}
	}
	return vc.Comments, vc.FetchedAt
}

// videoCohorts returns sorted cohort IDs assigned to a video, or nil if none.
// Loads the assignments file lazily — returns nil silently if file is missing
// or unparseable so that `video <id>` still works on a fresh checkout.
func videoCohorts(videoID string) []string {
	assignments, err := loadAssignments(cohortAssignmentsPath)
	if err != nil {
		return nil
	}
	ids := assignments[videoID]
	if len(ids) == 0 {
		return nil
	}
	out := append([]string(nil), ids...)
	sort.Strings(out)
	return out
}
