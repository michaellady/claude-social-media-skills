package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"google.golang.org/api/option"
	"google.golang.org/api/youtubeanalytics/v2"
)

const (
	analyticsBatchSize      = 200
	analyticsDailyBatchSize = 50 // smaller batch because rows-per-video balloon with `day` dimension
)

// AnalyticsOptions controls which dimensional passes runFetchAnalytics
// performs. The aggregate pass always runs; the others are gated by flags.
type AnalyticsOptions struct {
	Daily          bool
	TrafficSources bool
	SubStatus      bool
}

func runFetchAnalytics(dataPath string, opts AnalyticsOptions) error {
	// Load existing video data
	data, err := loadData(dataPath)
	if err != nil {
		return fmt.Errorf("loading video data: %w\n\nRun 'fetch' first to download video data", err)
	}

	if len(data.Videos) == 0 {
		return fmt.Errorf("no videos found in %s — run 'fetch' first", dataPath)
	}

	ctx := context.Background()
	client, err := getAnalyticsClient(ctx)
	if err != nil {
		return fmt.Errorf("OAuth authentication failed: %w", err)
	}

	svc, err := youtubeanalytics.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return fmt.Errorf("creating YouTube Analytics service: %w", err)
	}

	// Determine date range from video publish dates
	startDate, endDate := getDateRange(data.Videos)
	fmt.Printf("Fetching analytics for %d videos (%s to %s)...\n", len(data.Videos), startDate, endDate)

	// Build video ID lookup for merging results
	videoIndex := make(map[string]int)
	for i := range data.Videos {
		videoIndex[data.Videos[i].ID] = i
	}

	// Batch query video IDs (API limit: ~200 IDs per filter)
	ids := make([]string, len(data.Videos))
	for i, v := range data.Videos {
		ids[i] = v.ID
	}

	fetched := 0
	for start := 0; start < len(ids); start += analyticsBatchSize {
		end := start + analyticsBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[start:end]

		filter := "video==" + strings.Join(batch, ",")

		resp, err := svc.Reports.Query().
			Ids("channel==MINE").
			StartDate(startDate).
			EndDate(endDate).
			Metrics("estimatedMinutesWatched,averageViewDuration,averageViewPercentage,subscribersGained,subscribersLost,estimatedRevenue,adImpressions,cpm,monetizedPlaybacks").
			Dimensions("video").
			Filters(filter).
			MaxResults(int64(len(batch))).
			Do()
		if err != nil {
			return fmt.Errorf("querying analytics (batch %d-%d): %w", start, end, err)
		}

		// Build column index map
		colIdx := make(map[string]int)
		for i, h := range resp.ColumnHeaders {
			colIdx[h.Name] = i
		}

		// Parse rows and merge into video data
		for _, row := range resp.Rows {
			videoID, ok := rowString(row, colIdx, "video")
			if !ok {
				continue
			}
			idx, exists := videoIndex[videoID]
			if !exists {
				continue
			}

			v := &data.Videos[idx]
			v.EstimatedMinutesWatched = rowFloat(row, colIdx, "estimatedMinutesWatched")
			v.AverageViewDuration = rowFloat(row, colIdx, "averageViewDuration")
			v.AverageViewPercentage = rowFloat(row, colIdx, "averageViewPercentage")
			v.SubscribersGained = rowInt(row, colIdx, "subscribersGained")
			v.SubscribersLost = rowInt(row, colIdx, "subscribersLost")
			v.EstimatedRevenue = rowFloat(row, colIdx, "estimatedRevenue")
			v.AdImpressions = rowInt(row, colIdx, "adImpressions")
			v.CPM = rowFloat(row, colIdx, "cpm")
			v.MonetizedPlaybacks = rowInt(row, colIdx, "monetizedPlaybacks")
			fetched++
		}

		fmt.Printf("  Fetched batch %d-%d (%d videos with data)\n", start+1, end, len(resp.Rows))
	}

	fmt.Printf("Analytics data merged for %d/%d videos.\n", fetched, len(data.Videos))

	if opts.Daily {
		if err := fetchAnalyticsDaily(svc, data.Videos, videoIndex, ids, startDate, endDate); err != nil {
			return fmt.Errorf("daily pass: %w", err)
		}
	}
	if opts.TrafficSources {
		if err := fetchAnalyticsCategorical(svc, data.Videos, videoIndex, ids, startDate, endDate,
			"insightTrafficSourceType", "traffic_sources", "traffic sources"); err != nil {
			return fmt.Errorf("traffic-sources pass: %w", err)
		}
	}
	if opts.SubStatus {
		if err := fetchAnalyticsCategorical(svc, data.Videos, videoIndex, ids, startDate, endDate,
			"subscribedStatus", "sub_status_metrics", "sub-status"); err != nil {
			return fmt.Errorf("sub-status pass: %w", err)
		}
	}

	// Update metadata
	data.HasAnalytics = true
	data.AnalyticsFetchedAt = time.Now()

	// Save updated data
	if err := saveData(dataPath, data); err != nil {
		return fmt.Errorf("saving updated data: %w", err)
	}
	fmt.Printf("Updated %s with analytics data.\n", dataPath)
	return nil
}

func getDateRange(videos []Video) (string, string) {
	if len(videos) == 0 {
		return "", ""
	}
	earliest := videos[0].PublishedAt
	latest := videos[0].PublishedAt
	for _, v := range videos[1:] {
		if v.PublishedAt.Before(earliest) {
			earliest = v.PublishedAt
		}
		if v.PublishedAt.After(latest) {
			latest = v.PublishedAt
		}
	}
	// End date is today (analytics may have data beyond last publish)
	endDate := time.Now().Format("2006-01-02")
	return earliest.Format("2006-01-02"), endDate
}

func saveData(path string, data *ChannelData) error {
	if err := os.MkdirAll("data", 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

// fetchAnalyticsDaily runs the per-day breakdown pass. Smaller batch size
// because rows = videos × days; capacity caps at start/end MaxResults limit
// (~10K). At 50 videos × 90 days = 4500 rows, well under the cap.
func fetchAnalyticsDaily(svc *youtubeanalytics.Service, videos []Video, videoIndex map[string]int, ids []string, startDate, endDate string) error {
	fmt.Println("Running daily breakdown pass…")
	totalRows := 0
	for start := 0; start < len(ids); start += analyticsDailyBatchSize {
		end := start + analyticsDailyBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[start:end]
		filter := "video==" + strings.Join(batch, ",")
		resp, err := svc.Reports.Query().
			Ids("channel==MINE").
			StartDate(startDate).
			EndDate(endDate).
			Metrics("views,estimatedMinutesWatched,averageViewPercentage,subscribersGained,subscribersLost,estimatedRevenue").
			Dimensions("video,day").
			Filters(filter).
			MaxResults(10000).
			Do()
		if err != nil {
			return fmt.Errorf("daily query (batch %d-%d): %w", start, end, err)
		}
		colIdx := make(map[string]int, len(resp.ColumnHeaders))
		for i, h := range resp.ColumnHeaders {
			colIdx[h.Name] = i
		}
		applied := mergeDailyRows(resp.Rows, colIdx, videoIndex, videos)
		totalRows += applied
		fmt.Printf("  Daily batch %d-%d: %d rows merged\n", start+1, end, applied)
	}
	fmt.Printf("Daily breakdown: %d total rows across %d videos\n", totalRows, len(videos))
	return nil
}

// fetchAnalyticsCategorical runs a per-bucket breakdown pass for a single
// categorical dimension (insightTrafficSourceType or subscribedStatus).
func fetchAnalyticsCategorical(svc *youtubeanalytics.Service, videos []Video, videoIndex map[string]int, ids []string, startDate, endDate, dimension, target, label string) error {
	fmt.Printf("Running %s pass…\n", label)
	totalRows := 0
	for start := 0; start < len(ids); start += analyticsBatchSize {
		end := start + analyticsBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[start:end]
		filter := "video==" + strings.Join(batch, ",")
		resp, err := svc.Reports.Query().
			Ids("channel==MINE").
			StartDate(startDate).
			EndDate(endDate).
			Metrics("views,estimatedMinutesWatched").
			Dimensions("video," + dimension).
			Filters(filter).
			MaxResults(10000).
			Do()
		if err != nil {
			return fmt.Errorf("%s query (batch %d-%d): %w", label, start, end, err)
		}
		colIdx := make(map[string]int, len(resp.ColumnHeaders))
		for i, h := range resp.ColumnHeaders {
			colIdx[h.Name] = i
		}
		applied := mergeCategoricalRows(resp.Rows, colIdx, videoIndex, videos, dimension, target)
		totalRows += applied
		fmt.Printf("  %s batch %d-%d: %d rows merged\n", label, start+1, end, applied)
	}
	fmt.Printf("%s breakdown: %d total rows\n", label, totalRows)
	return nil
}

// Row parsing helpers for youtubeanalytics QueryResponse rows ([][]interface{})
func rowString(row []interface{}, colIdx map[string]int, col string) (string, bool) {
	idx, ok := colIdx[col]
	if !ok || idx >= len(row) {
		return "", false
	}
	s, ok := row[idx].(string)
	return s, ok
}

func rowFloat(row []interface{}, colIdx map[string]int, col string) float64 {
	idx, ok := colIdx[col]
	if !ok || idx >= len(row) {
		return 0
	}
	switch v := row[idx].(type) {
	case float64:
		return v
	case json.Number:
		f, _ := v.Float64()
		return f
	default:
		return 0
	}
}

func rowInt(row []interface{}, colIdx map[string]int, col string) int64 {
	idx, ok := colIdx[col]
	if !ok || idx >= len(row) {
		return 0
	}
	switch v := row[idx].(type) {
	case float64:
		return int64(v)
	case json.Number:
		i, _ := v.Int64()
		return i
	default:
		return 0
	}
}
