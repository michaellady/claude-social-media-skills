package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"time"
)

func runDashboard(dataPath, outputPath string, filter VideoFilter) error {
	data, err := loadData(dataPath)
	if err != nil {
		return err
	}

	if !filter.IsZero() {
		before := len(data.Videos)
		data.Videos = filter.Apply(data.Videos)
		fmt.Printf("Filter %s: %d of %d videos.\n", filter.Describe(), len(data.Videos), before)
	}

	result := analyze(data)
	dashData := buildDashboardData(data, result)
	dashData.FilterDescription = filter.Describe()

	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("creating dashboard file: %w", err)
	}
	defer f.Close()

	funcMap := template.FuncMap{
		"mul": func(a float64, b float64) float64 { return a * b },
		"fmtMinutes": func(m float64) string {
			if m >= 60 {
				return fmt.Sprintf("%.1fh", m/60)
			}
			return fmt.Sprintf("%.0fm", m)
		},
		"fmtDuration": func(s float64) string {
			if s >= 60 {
				return fmt.Sprintf("%.0fm %.0fs", float64(int(s)/60), float64(int(s)%60))
			}
			return fmt.Sprintf("%.0fs", s)
		},
	}
	tmpl, err := template.New("dashboard").Funcs(funcMap).Parse(dashboardHTML)
	if err != nil {
		return fmt.Errorf("parsing template: %w", err)
	}
	if err := tmpl.Execute(f, dashData); err != nil {
		return fmt.Errorf("executing template: %w", err)
	}

	fmt.Printf("Dashboard generated: %s\n", outputPath)
	openBrowser(outputPath)
	return nil
}

type DashboardData struct {
	ChannelTitle string
	GeneratedAt  string
	Stats        DashStats
	TimelineJSON template.JS
	ShortsJSON   template.JS
	LongFormJSON template.JS
	LiveJSON     template.JS
	BulkJSON     template.JS
	TopVideos    []DashVideo
	BottomVideos []DashVideo
	TopEngVideos []DashVideo
	BulkDays     []BulkPostingDay
	BulkPerf     PeriodPerformance
	NonBulkPerf  PeriodPerformance
	TitleIns     TitleInsights
	HasBulk      bool

	HasAnalytics       bool
	WatchTimeStats     WatchTimeAnalysis
	ShortsWatchTime    WatchTimeAnalysis
	LongFormWatchTime  WatchTimeAnalysis
	LiveWatchTime      WatchTimeAnalysis
	WatchTimeJSON      template.JS
	TopWatchVideos     []DashVideo
	TopRetentionVideos []DashVideo
	SubsJSON           template.JS
	TopSubVideos       []DashVideo

	// Cohort, traffic-source, and sub-status sections render only when the
	// matching data is present — keeps a fresh-checkout dashboard from
	// rendering empty tables.
	HasCohorts     bool
	CohortRows     []DashCohortRow
	HasTrafficMix  bool
	TrafficMixJSON template.JS
	HasSubStatus   bool
	TopUnsubVideos []DashVideo

	FilterDescription string
}

type DashCohortRow struct {
	ID         string
	Name       string
	Videos     int
	Views      string
	Retention  string // view-weighted, e.g. "22.9%"
	SubsPer1K  string // against all views
	UnsubConv  string // subs per 1K UNSUBSCRIBED views; "—" when sub-status not present
	Revenue    string
}

type DashStats struct {
	Total    int
	Shorts   int
	LongForm int
	Live     int
}

type DashVideo struct {
	Title     string
	Type      string
	Views     string
	Likes     string
	Eng       string
	Date      string
	Thumbnail   string
	URL         string
	WatchTime   string
	Retention   string
	AvgDuration string
	SubsGained  string
	SubConv     string
}

type timelinePoint struct {
	X     string  `json:"x"`
	Y     int64   `json:"y"`
	Title string  `json:"title"`
	Type  string  `json:"type"`
	Eng   float64 `json:"eng"`
}

type trendPoint struct {
	X     string  `json:"x"`
	Views float64 `json:"views"`
	Count int     `json:"count"`
	Eng   float64 `json:"eng"`
}

func buildDashboardData(data *ChannelData, r *AnalysisResult) DashboardData {
	d := DashboardData{
		ChannelTitle: data.ChannelTitle,
		GeneratedAt:  time.Now().Format("January 2, 2006 3:04 PM"),
		Stats: DashStats{
			Total:    r.TotalVideos,
			Shorts:   r.Shorts,
			LongForm: r.LongForm,
			Live:     r.LiveStreams,
		},
		BulkDays:    r.BulkPostingDays,
		BulkPerf:    r.BulkPeriodPerf,
		NonBulkPerf: r.NonBulkPeriodPerf,
		TitleIns:    r.TitleInsights,
		HasBulk:     len(r.BulkPostingDays) > 0,
	}

	// Timeline data: all videos as scatter points
	var shortPts, longPts, livePts []timelinePoint
	for _, v := range data.Videos {
		pt := timelinePoint{
			X:     v.PublishedAt.Format("2006-01-02"),
			Y:     v.ViewCount,
			Title: v.Title,
			Type:  v.VideoType,
			Eng:   engagementRate(v) * 100,
		}
		switch v.VideoType {
		case "short":
			shortPts = append(shortPts, pt)
		case "long-form":
			longPts = append(longPts, pt)
		case "live":
			livePts = append(livePts, pt)
		}
	}

	shortJSON, _ := json.Marshal(shortPts)
	longJSON, _ := json.Marshal(longPts)
	liveJSON, _ := json.Marshal(livePts)
	d.TimelineJSON = template.JS(shortJSON)

	// Monthly trend data
	d.ShortsJSON = marshalTrend(r.ShortsOverTime)
	d.LongFormJSON = marshalTrend(r.LongFormOverTime)
	d.LiveJSON = marshalTrend(r.LiveOverTime)

	// Build bulk posting overlay data (shorts per week with long-form avg views)
	d.BulkJSON = marshalBulkData(data.Videos)

	// Ignored: using shortJSON/longJSON/liveJSON for timeline
	_ = shortJSON
	_ = longJSON
	_ = liveJSON

	// Actually set all three timeline datasets as a combined JSON
	type timelineData struct {
		Shorts   []timelinePoint `json:"shorts"`
		LongForm []timelinePoint `json:"longform"`
		Live     []timelinePoint `json:"live"`
	}
	tlData := timelineData{Shorts: shortPts, LongForm: longPts, Live: livePts}
	if tlData.Shorts == nil {
		tlData.Shorts = []timelinePoint{}
	}
	if tlData.LongForm == nil {
		tlData.LongForm = []timelinePoint{}
	}
	if tlData.Live == nil {
		tlData.Live = []timelinePoint{}
	}
	tlJSON, _ := json.Marshal(tlData)
	d.TimelineJSON = template.JS(tlJSON)

	// Top/bottom videos
	for _, v := range r.TopByViews {
		d.TopVideos = append(d.TopVideos, toDashVideo(v))
	}
	for _, v := range r.BottomByViews {
		d.BottomVideos = append(d.BottomVideos, toDashVideo(v))
	}
	for _, v := range r.TopByEngagement {
		d.TopEngVideos = append(d.TopEngVideos, toDashVideo(v))
	}

	// Watch time data (only if analytics available)
	if r.HasAnalytics {
		d.HasAnalytics = true
		d.WatchTimeStats = r.WatchTime
		d.ShortsWatchTime = r.ShortsWatchTime
		d.LongFormWatchTime = r.LongFormWatchTime

		// Monthly watch time JSON for chart
		type wtPoint struct {
			X         string  `json:"x"`
			Minutes   float64 `json:"minutes"`
			Retention float64 `json:"retention"`
			Count     int     `json:"count"`
		}
		var wtPts []wtPoint
		for _, b := range r.WatchTimeOverTime {
			wtPts = append(wtPts, wtPoint{X: b.Period, Minutes: b.TotalMinutes, Retention: b.AvgRetention, Count: b.VideoCount})
		}
		if wtPts == nil {
			wtPts = []wtPoint{}
		}
		wtJSON, _ := json.Marshal(wtPts)
		d.WatchTimeJSON = template.JS(wtJSON)

		for _, v := range r.TopByWatchTime {
			d.TopWatchVideos = append(d.TopWatchVideos, toDashVideoWT(v))
		}
		for _, v := range r.TopByRetention {
			d.TopRetentionVideos = append(d.TopRetentionVideos, toDashVideoWT(v))
		}

		d.LiveWatchTime = r.LiveWatchTime

		// Monthly subscriber data for chart
		type subPoint struct {
			X       string  `json:"x"`
			Gained  int64   `json:"gained"`
			Lost    int64   `json:"lost"`
			Net     int64   `json:"net"`
			PerVid  float64 `json:"perVid"`
		}
		var subPts []subPoint
		for _, b := range r.SubsOverTime {
			subPts = append(subPts, subPoint{X: b.Period, Gained: b.Gained, Lost: b.Lost, Net: b.Net, PerVid: b.SubsPerVideo})
		}
		if subPts == nil {
			subPts = []subPoint{}
		}
		subJSON, _ := json.Marshal(subPts)
		d.SubsJSON = template.JS(subJSON)

		for _, v := range r.TopBySubscribers {
			dv := toDashVideoWT(v)
			dv.SubsGained = fmt.Sprintf("+%d", v.SubscribersGained)
			if v.ViewCount > 0 {
				dv.SubConv = fmt.Sprintf("%.2f%%", float64(v.SubscribersGained)/float64(v.ViewCount)*100)
			}
			d.TopSubVideos = append(d.TopSubVideos, dv)
		}
	}

	// Cohort, traffic-source, sub-status rollups. Each section is gated on
	// data availability so a fresh checkout (no cohorts.yaml, no
	// fetch-analytics --all) doesn't render empty tables.
	d.HasCohorts, d.CohortRows = buildDashCohorts(data.Videos)
	d.HasTrafficMix, d.TrafficMixJSON = buildDashTrafficMix(data.Videos)
	d.HasSubStatus, d.TopUnsubVideos = buildDashTopUnsubConversion(data.Videos)

	return d
}

// buildDashCohorts loads cohorts + assignments and rolls up per-cohort metrics
// for the filtered slice. Returns (false, nil) when no assignments exist.
func buildDashCohorts(videos []Video) (bool, []DashCohortRow) {
	cohorts, err := loadCohorts(cohortsYAMLPath)
	if err != nil || len(cohorts) == 0 {
		return false, nil
	}
	assignments, err := loadAssignments(cohortAssignmentsPath)
	if err != nil || len(assignments) == 0 {
		return false, nil
	}
	stats := make(map[string]*cohortStats, len(cohorts))
	for _, c := range cohorts {
		stats[c.ID] = &cohortStats{id: c.ID, name: c.Name}
	}
	// Track UNSUBSCRIBED views per cohort separately for the cleaner conversion column.
	unsubViews := map[string]int64{}
	for _, v := range videos {
		ids := assignments[v.ID]
		uns, hasUnsub := v.SubStatusMetrics["UNSUBSCRIBED"]
		for _, id := range ids {
			s, ok := stats[id]
			if !ok {
				continue
			}
			s.add(v)
			if hasUnsub {
				unsubViews[id] += uns.Views
			}
		}
	}
	rows := make([]DashCohortRow, 0, len(cohorts))
	for _, c := range cohorts {
		s := stats[c.ID]
		ret := 0.0
		if s.retDenom > 0 {
			ret = s.retention / s.retDenom
		}
		subsPer1K := 0.0
		if s.views > 0 {
			subsPer1K = float64(s.subs) / float64(s.views) * 1000
		}
		unsubConv := "—"
		if uv := unsubViews[c.ID]; uv > 0 {
			unsubConv = fmt.Sprintf("%.2f", float64(s.subs)/float64(uv)*1000)
		}
		rows = append(rows, DashCohortRow{
			ID:        c.ID,
			Name:      c.Name,
			Videos:    s.count,
			Views:     fmtNum(s.views),
			Retention: fmt.Sprintf("%.1f%%", ret),
			SubsPer1K: fmt.Sprintf("%.2f", subsPer1K),
			UnsubConv: unsubConv,
			Revenue:   fmt.Sprintf("$%.2f", s.revenue),
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Videos > rows[j].Videos })
	return true, rows
}

// buildDashTrafficMix aggregates traffic_sources across the filtered videos
// and emits a chart-ready JSON ([{label, views, watchMin}]) sorted desc.
func buildDashTrafficMix(videos []Video) (bool, template.JS) {
	totals := map[string]int64{}
	watch := map[string]float64{}
	for _, v := range videos {
		for source, m := range v.TrafficSources {
			totals[source] += m.Views
			watch[source] += m.WatchMin
		}
	}
	if len(totals) == 0 {
		return false, ""
	}
	type point struct {
		Label    string  `json:"label"`
		Views    int64   `json:"views"`
		WatchMin float64 `json:"watchMin"`
	}
	pts := make([]point, 0, len(totals))
	for k, v := range totals {
		pts = append(pts, point{Label: k, Views: v, WatchMin: watch[k]})
	}
	sort.Slice(pts, func(i, j int) bool { return pts[i].Views > pts[j].Views })
	b, _ := json.Marshal(pts)
	return true, template.JS(b)
}

// buildDashTopUnsubConversion ranks videos by subs-per-1K-UNSUBSCRIBED.
// Mirrors printTopByUnsubscribedConversion from analyze.go.
func buildDashTopUnsubConversion(videos []Video) (bool, []DashVideo) {
	type entry struct {
		v     Video
		unsub int64
		rate  float64
	}
	var rows []entry
	for _, v := range videos {
		uns, ok := v.SubStatusMetrics["UNSUBSCRIBED"]
		if !ok || uns.Views < 100 {
			continue
		}
		rate := float64(v.SubscribersGained) / float64(uns.Views) * 1000
		rows = append(rows, entry{v: v, unsub: uns.Views, rate: rate})
	}
	if len(rows) == 0 {
		return false, nil
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].rate > rows[j].rate })
	limit := 10
	if len(rows) < limit {
		limit = len(rows)
	}
	out := make([]DashVideo, 0, limit)
	for _, e := range rows[:limit] {
		dv := toDashVideoWT(e.v)
		dv.SubsGained = fmt.Sprintf("+%d", e.v.SubscribersGained)
		dv.SubConv = fmt.Sprintf("%.2f / 1K unsub", e.rate)
		// Reuse Views slot to show the UNSUBSCRIBED denominator (clearer in this table).
		dv.Views = fmtNum(e.unsub) + " unsub"
		out = append(out, dv)
	}
	return true, out
}

func toDashVideo(v Video) DashVideo {
	return DashVideo{
		Title:     v.Title,
		Type:      v.VideoType,
		Views:     fmtNum(v.ViewCount),
		Likes:     fmtNum(v.LikeCount),
		Eng:       fmt.Sprintf("%.2f%%", engagementRate(v)*100),
		Date:      v.PublishedAt.Format("2006-01-02"),
		Thumbnail: v.ThumbnailURL,
		URL:       "https://youtube.com/watch?v=" + v.ID,
	}
}

func toDashVideoWT(v Video) DashVideo {
	d := toDashVideo(v)
	d.WatchTime = fmt.Sprintf("%.0f min", v.EstimatedMinutesWatched)
	d.Retention = fmt.Sprintf("%.1f%%", v.AverageViewPercentage)
	d.AvgDuration = fmt.Sprintf("%.0fs", v.AverageViewDuration)
	return d
}

func marshalTrend(buckets []TimeBucket) template.JS {
	var pts []trendPoint
	for _, b := range buckets {
		pts = append(pts, trendPoint{
			X:     b.Period,
			Views: b.AvgViews,
			Count: b.VideoCount,
			Eng:   b.AvgEng * 100,
		})
	}
	if pts == nil {
		pts = []trendPoint{}
	}
	b, _ := json.Marshal(pts)
	return template.JS(b)
}

func marshalBulkData(videos []Video) template.JS {
	// Group by week: count shorts and avg long-form views
	type weekData struct {
		Week       string `json:"week"`
		ShortCount int    `json:"shortCount"`
		LFAvgViews float64 `json:"lfAvgViews"`
	}

	weekShorts := map[string]int{}
	weekLFViews := map[string][]int64{}

	for _, v := range videos {
		year, week := v.PublishedAt.ISOWeek()
		key := fmt.Sprintf("%d-W%02d", year, week)
		if v.VideoType == "short" {
			weekShorts[key]++
		} else if v.VideoType == "long-form" {
			weekLFViews[key] = append(weekLFViews[key], v.ViewCount)
		}
	}

	// Collect all weeks
	allWeeks := map[string]bool{}
	for k := range weekShorts {
		allWeeks[k] = true
	}
	for k := range weekLFViews {
		allWeeks[k] = true
	}

	var weeks []string
	for k := range allWeeks {
		weeks = append(weeks, k)
	}
	sort.Strings(weeks)

	var result []weekData
	for _, w := range weeks {
		wd := weekData{Week: w, ShortCount: weekShorts[w]}
		if views, ok := weekLFViews[w]; ok && len(views) > 0 {
			var total int64
			for _, vv := range views {
				total += vv
			}
			wd.LFAvgViews = float64(total) / float64(len(views))
		}
		result = append(result, wd)
	}

	b, _ := json.Marshal(result)
	return template.JS(b)
}

func openBrowser(path string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "linux":
		cmd = exec.Command("xdg-open", path)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", path)
	default:
		return
	}
	_ = cmd.Start()
}

const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>{{.ChannelTitle}} - YouTube Analytics</title>
<script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.7/dist/chart.umd.min.js"></script>
<script src="https://cdn.jsdelivr.net/npm/chartjs-adapter-date-fns@3.0.0/dist/chartjs-adapter-date-fns.bundle.min.js"></script>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #0f0f0f; color: #e8e8e8; }
  .header { background: #1a1a2e; padding: 24px 32px; border-bottom: 2px solid #e74c3c; }
  .header h1 { font-size: 24px; font-weight: 600; }
  .header .subtitle { color: #aaa; margin-top: 4px; font-size: 13px; }
  .container { max-width: 1400px; margin: 0 auto; padding: 24px; }
  .stats-row { display: grid; grid-template-columns: repeat(4, 1fr); gap: 16px; margin-bottom: 24px; }
  .stat-card { background: #1a1a2e; border-radius: 12px; padding: 20px; text-align: center; }
  .stat-card .number { font-size: 32px; font-weight: 700; }
  .stat-card .label { color: #aaa; font-size: 13px; margin-top: 4px; }
  .stat-card.shorts .number { color: #e74c3c; }
  .stat-card.longform .number { color: #3498db; }
  .stat-card.live .number { color: #e67e22; }
  .stat-card.total .number { color: #2ecc71; }
  .chart-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 20px; margin-bottom: 24px; }
  .chart-card { background: #1a1a2e; border-radius: 12px; padding: 20px; }
  .chart-card.full { grid-column: 1 / -1; }
  .chart-card h2 { font-size: 16px; font-weight: 600; margin-bottom: 16px; color: #fff; }
  .chart-container { position: relative; height: 350px; }
  .section { background: #1a1a2e; border-radius: 12px; padding: 20px; margin-bottom: 24px; }
  .section h2 { font-size: 16px; font-weight: 600; margin-bottom: 16px; color: #fff; }
  table { width: 100%; border-collapse: collapse; font-size: 13px; }
  th { text-align: left; padding: 10px 12px; border-bottom: 1px solid #333; color: #aaa; font-weight: 500; }
  td { padding: 10px 12px; border-bottom: 1px solid #1f1f1f; }
  tr:hover { background: #16213e; }
  .type-badge { display: inline-block; padding: 2px 8px; border-radius: 4px; font-size: 11px; font-weight: 600; }
  .type-badge.short { background: #e74c3c22; color: #e74c3c; }
  .type-badge.long-form { background: #3498db22; color: #3498db; }
  .type-badge.live { background: #e67e2222; color: #e67e22; }
  a { color: #3498db; text-decoration: none; }
  a:hover { text-decoration: underline; }
  .insight-box { background: #16213e; border-left: 3px solid #3498db; padding: 16px; border-radius: 0 8px 8px 0; margin: 12px 0; font-size: 14px; line-height: 1.6; }
  .insight-box.warning { border-color: #e74c3c; }
  .insight-box.success { border-color: #2ecc71; }
  .two-col { display: grid; grid-template-columns: 1fr 1fr; gap: 20px; }
  .perf-stat { display: flex; justify-content: space-between; padding: 8px 0; border-bottom: 1px solid #1f1f1f; }
  .perf-stat .label { color: #aaa; }
  .title-thumb { width: 80px; height: 45px; object-fit: cover; border-radius: 4px; }
  @media (max-width: 900px) {
    .stats-row { grid-template-columns: repeat(2, 1fr); }
    .chart-grid { grid-template-columns: 1fr; }
    .two-col { grid-template-columns: 1fr; }
  }
</style>
</head>
<body>
<div class="header">
  <h1>{{.ChannelTitle}} Analytics Dashboard</h1>
  <div class="subtitle">Generated {{.GeneratedAt}} | Data from YouTube Data API v3{{if .FilterDescription}} | Filter: {{.FilterDescription}}{{end}}</div>
</div>

<div class="container">

<!-- Stats Row -->
<div class="stats-row">
  <div class="stat-card total"><div class="number">{{.Stats.Total}}</div><div class="label">Total Videos</div></div>
  <div class="stat-card shorts"><div class="number">{{.Stats.Shorts}}</div><div class="label">Shorts</div></div>
  <div class="stat-card longform"><div class="number">{{.Stats.LongForm}}</div><div class="label">Long-Form</div></div>
  <div class="stat-card live"><div class="number">{{.Stats.Live}}</div><div class="label">Live Streams</div></div>
</div>

{{if .HasAnalytics}}
<div class="stats-row">
  <div class="stat-card" style="border-left: 3px solid #9b59b6;"><div class="number" style="color:#9b59b6;">{{fmtMinutes .WatchTimeStats.TotalMinutes}}</div><div class="label">Total Watch Time</div></div>
  <div class="stat-card" style="border-left: 3px solid #1abc9c;"><div class="number" style="color:#1abc9c;">{{printf "%.1f%%" .WatchTimeStats.AvgRetention}}</div><div class="label">Avg Retention</div></div>
  <div class="stat-card" style="border-left: 3px solid #f39c12;"><div class="number" style="color:#f39c12;">{{printf "%+d" .WatchTimeStats.NetSubscribers}}</div><div class="label">Net Subscribers</div></div>
  <div class="stat-card" style="border-left: 3px solid #e056fd;"><div class="number" style="color:#e056fd;">{{fmtDuration .WatchTimeStats.AvgViewDuration}}</div><div class="label">Avg View Duration</div></div>
</div>
{{end}}

<!-- Charts -->
<div class="chart-grid">
  <div class="chart-card full">
    <h2>Video Performance Timeline</h2>
    <div class="chart-container"><canvas id="timelineChart"></canvas></div>
  </div>

  <div class="chart-card">
    <h2>Shorts: Monthly Avg Views</h2>
    <div class="chart-container"><canvas id="shortsChart"></canvas></div>
  </div>

  <div class="chart-card">
    <h2>Long-Form: Monthly Avg Views</h2>
    <div class="chart-container"><canvas id="longformChart"></canvas></div>
  </div>

  <div class="chart-card full">
    <h2>Shorts Posting Frequency vs Long-Form Performance</h2>
    <div class="chart-container"><canvas id="bulkChart"></canvas></div>
  </div>

  <div class="chart-card">
    <h2>Shorts: Monthly Engagement Rate</h2>
    <div class="chart-container"><canvas id="shortsEngChart"></canvas></div>
  </div>

  <div class="chart-card">
    <h2>Long-Form: Monthly Engagement Rate</h2>
    <div class="chart-container"><canvas id="longformEngChart"></canvas></div>
  </div>

{{if .HasAnalytics}}
  <div class="chart-card full">
    <h2>Monthly Watch Time &amp; Retention</h2>
    <div class="chart-container"><canvas id="watchTimeChart"></canvas></div>
  </div>

  <div class="chart-card">
    <h2>Watch Time by Content Type</h2>
    <div class="chart-container"><canvas id="watchTypeChart"></canvas></div>
  </div>

  <div class="chart-card">
    <h2>Subscriber Impact by Type</h2>
    <div class="chart-container"><canvas id="subChart"></canvas></div>
  </div>

  <div class="chart-card full">
    <h2>Monthly Subscriber Trend</h2>
    <div class="chart-container"><canvas id="subTrendChart"></canvas></div>
  </div>

  <div class="chart-card">
    <h2>Subs per 1K Views by Type</h2>
    <div class="chart-container"><canvas id="subRateChart"></canvas></div>
  </div>
{{end}}
</div>

<!-- Shorts Bulk Posting Analysis -->
<div class="section">
  <h2>Shorts Bulk Posting Impact Analysis</h2>
  {{if .HasBulk}}
  <p style="color:#aaa; margin-bottom:12px;">Comparing long-form video performance when published within ±7 days of a bulk shorts day (3+ shorts in one day) vs. outside that window.</p>
  <div class="two-col">
    <div>
      <h3 style="color:#e74c3c; margin-bottom:8px;">Near Bulk Posting</h3>
      <div class="perf-stat"><span class="label">Videos</span><span>{{.BulkPerf.VideoCount}}</span></div>
      <div class="perf-stat"><span class="label">Avg Views</span><span>{{printf "%.0f" .BulkPerf.AvgViews}}</span></div>
      <div class="perf-stat"><span class="label">Avg Likes</span><span>{{printf "%.0f" .BulkPerf.AvgLikes}}</span></div>
      <div class="perf-stat"><span class="label">Avg Engagement</span><span>{{printf "%.2f%%" (mul .BulkPerf.AvgEngagement 100)}}</span></div>
    </div>
    <div>
      <h3 style="color:#2ecc71; margin-bottom:8px;">Away From Bulk Posting</h3>
      <div class="perf-stat"><span class="label">Videos</span><span>{{.NonBulkPerf.VideoCount}}</span></div>
      <div class="perf-stat"><span class="label">Avg Views</span><span>{{printf "%.0f" .NonBulkPerf.AvgViews}}</span></div>
      <div class="perf-stat"><span class="label">Avg Likes</span><span>{{printf "%.0f" .NonBulkPerf.AvgLikes}}</span></div>
      <div class="perf-stat"><span class="label">Avg Engagement</span><span>{{printf "%.2f%%" (mul .NonBulkPerf.AvgEngagement 100)}}</span></div>
    </div>
  </div>
  {{else}}
  <div class="insight-box">No bulk posting days detected (3+ shorts in one day). Cannot compare impact.</div>
  {{end}}
</div>

<!-- Top Videos Table -->
<div class="section">
  <h2>Top 10 Videos by Views</h2>
  <table>
    <thead><tr><th></th><th>Title</th><th>Type</th><th>Views</th><th>Likes</th><th>Engagement</th><th>Published</th></tr></thead>
    <tbody>
    {{range .TopVideos}}
    <tr>
      <td><img class="title-thumb" src="{{.Thumbnail}}" alt="" loading="lazy"></td>
      <td><a href="{{.URL}}" target="_blank">{{.Title}}</a></td>
      <td><span class="type-badge {{.Type}}">{{.Type}}</span></td>
      <td>{{.Views}}</td>
      <td>{{.Likes}}</td>
      <td>{{.Eng}}</td>
      <td>{{.Date}}</td>
    </tr>
    {{end}}
    </tbody>
  </table>
</div>

<!-- Top by Engagement -->
<div class="section">
  <h2>Top 10 Videos by Engagement Rate (min 100 views)</h2>
  <table>
    <thead><tr><th></th><th>Title</th><th>Type</th><th>Views</th><th>Engagement</th><th>Published</th></tr></thead>
    <tbody>
    {{range .TopEngVideos}}
    <tr>
      <td><img class="title-thumb" src="{{.Thumbnail}}" alt="" loading="lazy"></td>
      <td><a href="{{.URL}}" target="_blank">{{.Title}}</a></td>
      <td><span class="type-badge {{.Type}}">{{.Type}}</span></td>
      <td>{{.Views}}</td>
      <td>{{.Eng}}</td>
      <td>{{.Date}}</td>
    </tr>
    {{end}}
    </tbody>
  </table>
</div>

<!-- Title Analysis -->
<div class="section">
  <h2>Title Analysis (Top vs Bottom Quartile - Long-Form)</h2>
  {{if gt .TitleIns.AvgLenTop 0.0}}
  <div class="two-col">
    <div>
      <div class="perf-stat"><span class="label">Avg Title Length</span><span>Top: {{printf "%.0f" .TitleIns.AvgLenTop}} / Bottom: {{printf "%.0f" .TitleIns.AvgLenBottom}} chars</span></div>
      <div class="perf-stat"><span class="label">Titles with ?</span><span>Top: {{.TitleIns.QuestionsTop}} / Bottom: {{.TitleIns.QuestionsBottom}}</span></div>
    </div>
    <div>
      <div class="perf-stat"><span class="label">Titles with Numbers</span><span>Top: {{.TitleIns.NumbersTop}} / Bottom: {{.TitleIns.NumbersBottom}}</span></div>
      <div class="perf-stat"><span class="label">Avg ALL-CAPS Words</span><span>Top: {{printf "%.1f" .TitleIns.CapsWordsTop}} / Bottom: {{printf "%.1f" .TitleIns.CapsWordsBottom}}</span></div>
    </div>
  </div>
  {{else}}
  <div class="insight-box">Not enough long-form videos for title analysis.</div>
  {{end}}
</div>

{{if .HasAnalytics}}
<!-- Top Videos by Watch Time -->
<div class="section">
  <h2>Top 10 Videos by Watch Time</h2>
  <table>
    <thead><tr><th></th><th>Title</th><th>Type</th><th>Watch Time</th><th>Views</th><th>Retention</th><th>Published</th></tr></thead>
    <tbody>
    {{range .TopWatchVideos}}
    <tr>
      <td><img class="title-thumb" src="{{.Thumbnail}}" alt="" loading="lazy"></td>
      <td><a href="{{.URL}}" target="_blank">{{.Title}}</a></td>
      <td><span class="type-badge {{.Type}}">{{.Type}}</span></td>
      <td>{{.WatchTime}}</td>
      <td>{{.Views}}</td>
      <td>{{.Retention}}</td>
      <td>{{.Date}}</td>
    </tr>
    {{end}}
    </tbody>
  </table>
</div>

<!-- Top Videos by Retention -->
<div class="section">
  <h2>Top 10 Videos by Retention (min 100 views)</h2>
  <table>
    <thead><tr><th></th><th>Title</th><th>Type</th><th>Retention</th><th>Avg Duration</th><th>Views</th><th>Published</th></tr></thead>
    <tbody>
    {{range .TopRetentionVideos}}
    <tr>
      <td><img class="title-thumb" src="{{.Thumbnail}}" alt="" loading="lazy"></td>
      <td><a href="{{.URL}}" target="_blank">{{.Title}}</a></td>
      <td><span class="type-badge {{.Type}}">{{.Type}}</span></td>
      <td>{{.Retention}}</td>
      <td>{{.AvgDuration}}</td>
      <td>{{.Views}}</td>
      <td>{{.Date}}</td>
    </tr>
    {{end}}
    </tbody>
  </table>
</div>

<!-- Top Videos by Subscribers -->
<div class="section">
  <h2>Top 10 Videos by Subscribers Gained</h2>
  <table>
    <thead><tr><th></th><th>Title</th><th>Type</th><th>Subs</th><th>Conv Rate</th><th>Views</th><th>Watch Time</th><th>Published</th></tr></thead>
    <tbody>
    {{range .TopSubVideos}}
    <tr>
      <td><img class="title-thumb" src="{{.Thumbnail}}" alt="" loading="lazy"></td>
      <td><a href="{{.URL}}" target="_blank">{{.Title}}</a></td>
      <td><span class="type-badge {{.Type}}">{{.Type}}</span></td>
      <td>{{.SubsGained}}</td>
      <td>{{.SubConv}}</td>
      <td>{{.Views}}</td>
      <td>{{.WatchTime}}</td>
      <td>{{.Date}}</td>
    </tr>
    {{end}}
    </tbody>
  </table>
</div>
{{end}}

{{if .HasCohorts}}
<!-- Cohort performance -->
<div class="section">
  <h2>Cohort Performance</h2>
  <p style="color:#888;margin-bottom:12px;font-size:14px;">
    Defined in <code>data/cohorts.yaml</code>. <em>Subs/1K UNSUB</em> requires <code>fetch-analytics --sub-status</code>; falls back to em-dash when missing.
  </p>
  <table>
    <thead><tr><th>Cohort</th><th>Videos</th><th>Views</th><th>Retention</th><th>Subs / 1K</th><th>Subs / 1K UNSUB</th><th>Revenue</th></tr></thead>
    <tbody>
    {{range .CohortRows}}
    <tr>
      <td>{{.Name}} <span style="color:#666;font-size:12px;">({{.ID}})</span></td>
      <td>{{.Videos}}</td>
      <td>{{.Views}}</td>
      <td>{{.Retention}}</td>
      <td>{{.SubsPer1K}}</td>
      <td>{{.UnsubConv}}</td>
      <td>{{.Revenue}}</td>
    </tr>
    {{end}}
    </tbody>
  </table>
</div>
{{end}}

{{if .HasTrafficMix}}
<!-- Traffic source mix -->
<div class="section">
  <h2>Traffic Source Mix</h2>
  <p style="color:#888;margin-bottom:12px;font-size:14px;">
    Channel-wide aggregate from <code>fetch-analytics --traffic-sources</code>. Click a bar to see watch-min in the tooltip.
  </p>
  <div class="chart-container"><canvas id="trafficMixChart"></canvas></div>
</div>
{{end}}

{{if .HasSubStatus}}
<!-- Top by UNSUBSCRIBED conversion -->
<div class="section">
  <h2>Top 10 by UNSUBSCRIBED Conversion</h2>
  <p style="color:#888;margin-bottom:12px;font-size:14px;">
    Subs gained per 1K <em>UNSUBSCRIBED</em> views — the actual conversion-target population. Min 100 unsub views. Requires <code>fetch-analytics --sub-status</code>.
  </p>
  <table>
    <thead><tr><th></th><th>Title</th><th>Type</th><th>Subs</th><th>Conversion</th><th>Unsub views</th><th>Watch Time</th><th>Published</th></tr></thead>
    <tbody>
    {{range .TopUnsubVideos}}
    <tr>
      <td><img class="title-thumb" src="{{.Thumbnail}}" alt="" loading="lazy"></td>
      <td><a href="{{.URL}}" target="_blank">{{.Title}}</a></td>
      <td><span class="type-badge {{.Type}}">{{.Type}}</span></td>
      <td>{{.SubsGained}}</td>
      <td>{{.SubConv}}</td>
      <td>{{.Views}}</td>
      <td>{{.WatchTime}}</td>
      <td>{{.Date}}</td>
    </tr>
    {{end}}
    </tbody>
  </table>
</div>
{{end}}

</div>

<script>
const chartColors = {
  short: '#e74c3c',
  'long-form': '#3498db',
  live: '#e67e22',
  grid: '#222',
  text: '#888'
};

Chart.defaults.color = chartColors.text;
Chart.defaults.borderColor = chartColors.grid;

// Timeline chart
const tlData = {{.TimelineJSON}};
new Chart(document.getElementById('timelineChart'), {
  type: 'scatter',
  data: {
    datasets: [
      {
        label: 'Shorts',
        data: tlData.shorts.map(p => ({x: p.x, y: p.y, title: p.title})),
        backgroundColor: chartColors.short + '99',
        pointRadius: 5,
        pointHoverRadius: 8
      },
      {
        label: 'Long-Form',
        data: tlData.longform.map(p => ({x: p.x, y: p.y, title: p.title})),
        backgroundColor: chartColors['long-form'] + '99',
        pointRadius: 5,
        pointHoverRadius: 8
      },
      {
        label: 'Live',
        data: tlData.live.map(p => ({x: p.x, y: p.y, title: p.title})),
        backgroundColor: chartColors.live + '99',
        pointRadius: 5,
        pointHoverRadius: 8
      }
    ]
  },
  options: {
    responsive: true,
    maintainAspectRatio: false,
    scales: {
      x: { type: 'time', time: { unit: 'month' } },
      y: { beginAtZero: true, title: { display: true, text: 'Views' } }
    },
    plugins: {
      tooltip: {
        callbacks: {
          label: ctx => {
            const pt = ctx.raw;
            return pt.title + ' — ' + pt.y.toLocaleString() + ' views';
          }
        }
      }
    }
  }
});

// Trend chart helper
function trendChart(canvasId, data, color, yLabel) {
  new Chart(document.getElementById(canvasId), {
    type: 'bar',
    data: {
      labels: data.map(d => d.x),
      datasets: [{
        data: data.map(d => d.views),
        backgroundColor: color + '66',
        borderColor: color,
        borderWidth: 1
      }]
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      plugins: { legend: { display: false } },
      scales: {
        y: { beginAtZero: true, title: { display: true, text: yLabel || 'Avg Views' } }
      }
    }
  });
}

function engChart(canvasId, data, color) {
  new Chart(document.getElementById(canvasId), {
    type: 'line',
    data: {
      labels: data.map(d => d.x),
      datasets: [{
        data: data.map(d => d.eng),
        borderColor: color,
        backgroundColor: color + '22',
        fill: true,
        tension: 0.3,
        pointRadius: 3
      }]
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      plugins: { legend: { display: false } },
      scales: {
        y: { beginAtZero: true, title: { display: true, text: 'Engagement %' } }
      }
    }
  });
}

const shortsData = {{.ShortsJSON}};
const longformData = {{.LongFormJSON}};

trendChart('shortsChart', shortsData, chartColors.short);
trendChart('longformChart', longformData, chartColors['long-form']);
engChart('shortsEngChart', shortsData, chartColors.short);
engChart('longformEngChart', longformData, chartColors['long-form']);

// Bulk posting chart
const bulkData = {{.BulkJSON}};
new Chart(document.getElementById('bulkChart'), {
  type: 'bar',
  data: {
    labels: bulkData.map(d => d.week),
    datasets: [
      {
        label: 'Shorts Count',
        data: bulkData.map(d => d.shortCount),
        backgroundColor: chartColors.short + '88',
        yAxisID: 'y'
      },
      {
        label: 'Long-Form Avg Views',
        data: bulkData.map(d => d.lfAvgViews || null),
        borderColor: chartColors['long-form'],
        backgroundColor: chartColors['long-form'] + '22',
        type: 'line',
        fill: false,
        tension: 0.3,
        pointRadius: 2,
        yAxisID: 'y1'
      }
    ]
  },
  options: {
    responsive: true,
    maintainAspectRatio: false,
    scales: {
      y: { beginAtZero: true, position: 'left', title: { display: true, text: 'Shorts Count' } },
      y1: { beginAtZero: true, position: 'right', title: { display: true, text: 'Long-Form Avg Views' }, grid: { drawOnChartArea: false } }
    }
  }
});

{{if .HasAnalytics}}
// Watch time + retention dual-axis chart
const wtData = {{.WatchTimeJSON}};
new Chart(document.getElementById('watchTimeChart'), {
  type: 'bar',
  data: {
    labels: wtData.map(d => d.x),
    datasets: [
      {
        label: 'Watch Time (min)',
        data: wtData.map(d => d.minutes),
        backgroundColor: '#9b59b666',
        borderColor: '#9b59b6',
        borderWidth: 1,
        yAxisID: 'y'
      },
      {
        label: 'Avg Retention %',
        data: wtData.map(d => d.retention),
        borderColor: '#1abc9c',
        backgroundColor: '#1abc9c22',
        type: 'line',
        fill: false,
        tension: 0.3,
        pointRadius: 3,
        yAxisID: 'y1'
      }
    ]
  },
  options: {
    responsive: true,
    maintainAspectRatio: false,
    scales: {
      y: { beginAtZero: true, position: 'left', title: { display: true, text: 'Minutes Watched' } },
      y1: { beginAtZero: true, position: 'right', title: { display: true, text: 'Retention %' }, grid: { drawOnChartArea: false }, max: 100 }
    }
  }
});

// Watch time by content type (doughnut)
new Chart(document.getElementById('watchTypeChart'), {
  type: 'doughnut',
  data: {
    labels: ['Shorts', 'Long-Form'],
    datasets: [{
      data: [{{.ShortsWatchTime.TotalMinutes}}, {{.LongFormWatchTime.TotalMinutes}}],
      backgroundColor: ['#e74c3c88', '#3498db88'],
      borderColor: ['#e74c3c', '#3498db'],
      borderWidth: 2
    }]
  },
  options: {
    responsive: true,
    maintainAspectRatio: false,
    plugins: {
      legend: { position: 'bottom' },
      tooltip: {
        callbacks: {
          label: ctx => {
            const val = ctx.raw;
            const total = ctx.dataset.data.reduce((a,b) => a+b, 0);
            const pct = total > 0 ? (val/total*100).toFixed(1) : '0';
            return ctx.label + ': ' + Math.round(val).toLocaleString() + ' min (' + pct + '%)';
          }
        }
      }
    }
  }
});

// Subscriber impact by type
new Chart(document.getElementById('subChart'), {
  type: 'bar',
  data: {
    labels: ['Shorts', 'Long-Form', 'Live'],
    datasets: [
      {
        label: 'Gained',
        data: [{{.ShortsWatchTime.SubscribersGained}}, {{.LongFormWatchTime.SubscribersGained}}, {{.LiveWatchTime.SubscribersGained}}],
        backgroundColor: '#2ecc7188',
        borderColor: '#2ecc71',
        borderWidth: 1
      },
      {
        label: 'Lost',
        data: [{{.ShortsWatchTime.SubscribersLost}}, {{.LongFormWatchTime.SubscribersLost}}, {{.LiveWatchTime.SubscribersLost}}],
        backgroundColor: '#e74c3c88',
        borderColor: '#e74c3c',
        borderWidth: 1
      }
    ]
  },
  options: {
    responsive: true,
    maintainAspectRatio: false,
    scales: {
      y: { beginAtZero: true, title: { display: true, text: 'Subscribers' } }
    }
  }
});

// Monthly subscriber trend
const subData = {{.SubsJSON}};
new Chart(document.getElementById('subTrendChart'), {
  type: 'bar',
  data: {
    labels: subData.map(d => d.x),
    datasets: [
      {
        label: 'Gained',
        data: subData.map(d => d.gained),
        backgroundColor: '#2ecc7188',
        borderColor: '#2ecc71',
        borderWidth: 1
      },
      {
        label: 'Lost',
        data: subData.map(d => -d.lost),
        backgroundColor: '#e74c3c88',
        borderColor: '#e74c3c',
        borderWidth: 1
      },
      {
        label: 'Net',
        data: subData.map(d => d.net),
        borderColor: '#f39c12',
        backgroundColor: '#f39c1222',
        type: 'line',
        fill: false,
        tension: 0.3,
        pointRadius: 4,
        borderWidth: 2
      }
    ]
  },
  options: {
    responsive: true,
    maintainAspectRatio: false,
    scales: {
      y: { title: { display: true, text: 'Subscribers' } }
    }
  }
});

// Subs per 1K views by type
(function() {
  const types = ['Shorts', 'Long-Form', 'Live'];
  const gained = [{{.ShortsWatchTime.SubscribersGained}}, {{.LongFormWatchTime.SubscribersGained}}, {{.LiveWatchTime.SubscribersGained}}];
  // Calculate total views per type from the timeline data
  const shortViews = tlData.shorts.reduce((s,p) => s + p.y, 0);
  const lfViews = tlData.longform.reduce((s,p) => s + p.y, 0);
  const liveViews = tlData.live.reduce((s,p) => s + p.y, 0);
  const views = [shortViews, lfViews, liveViews];
  const rates = gained.map((g, i) => views[i] > 0 ? (g / views[i] * 1000) : 0);

  new Chart(document.getElementById('subRateChart'), {
    type: 'bar',
    data: {
      labels: types,
      datasets: [{
        label: 'Subs per 1K Views',
        data: rates,
        backgroundColor: [chartColors.short + '88', chartColors['long-form'] + '88', chartColors.live + '88'],
        borderColor: [chartColors.short, chartColors['long-form'], chartColors.live],
        borderWidth: 1
      }]
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      plugins: {
        tooltip: {
          callbacks: {
            label: ctx => ctx.raw.toFixed(2) + ' subs per 1K views'
          }
        }
      },
      scales: {
        y: { beginAtZero: true, title: { display: true, text: 'Subs per 1K Views' } }
      }
    }
  });
})();
{{end}}

{{if .HasTrafficMix}}
// Traffic source mix bar chart
(function() {
  const data = {{.TrafficMixJSON}};
  const labels = data.map(d => d.label);
  const views = data.map(d => d.views);
  const watchMin = data.map(d => d.watchMin);
  // Distinct color per bar so the legend isn't needed.
  const palette = ['#3498db','#e74c3c','#e67e22','#9b59b6','#1abc9c','#f39c12','#16a085','#2ecc71','#c0392b','#7f8c8d'];
  new Chart(document.getElementById('trafficMixChart'), {
    type: 'bar',
    data: {
      labels: labels,
      datasets: [{
        label: 'Views',
        data: views,
        backgroundColor: labels.map((_,i) => palette[i % palette.length] + 'cc'),
        borderColor: labels.map((_,i) => palette[i % palette.length]),
        borderWidth: 1
      }]
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      plugins: {
        legend: { display: false },
        tooltip: {
          callbacks: {
            afterLabel: function(ctx) {
              return 'Watch-min: ' + Math.round(watchMin[ctx.dataIndex]).toLocaleString();
            }
          }
        }
      },
      scales: {
        y: { beginAtZero: true, title: { display: true, text: 'Views' } }
      }
    }
  });
})();
{{end}}
</script>
</body>
</html>`
