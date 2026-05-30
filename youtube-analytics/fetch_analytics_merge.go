package main

// Pure functions that merge youtubeanalytics QueryResponse rows into Video
// records. Extracted from fetch_analytics.go so the dimensional-breakdown
// logic can be unit-tested without an API client.

// mergeDailyRows applies rows from a `Dimensions("video,day")` query to the
// matching videos. Each row contributes one DailyMetric. Existing
// DailyMetrics on a video are replaced (not merged) — running --daily again
// re-fetches the full series.
func mergeDailyRows(rows [][]interface{}, colIdx map[string]int, videoIndex map[string]int, videos []Video) (rowsApplied int) {
	// First pass: clear existing DailyMetrics on any video that appears in
	// the response, so a partial response doesn't leave a stale series mixed
	// with fresh rows.
	touched := map[string]bool{}
	for _, row := range rows {
		vid, ok := rowString(row, colIdx, "video")
		if !ok {
			continue
		}
		if !touched[vid] {
			if idx, exists := videoIndex[vid]; exists {
				videos[idx].DailyMetrics = videos[idx].DailyMetrics[:0]
			}
			touched[vid] = true
		}
	}
	// Second pass: append metrics in row order.
	for _, row := range rows {
		vid, ok := rowString(row, colIdx, "video")
		if !ok {
			continue
		}
		idx, exists := videoIndex[vid]
		if !exists {
			continue
		}
		videos[idx].DailyMetrics = append(videos[idx].DailyMetrics, DailyMetric{
			Date:              optString(row, colIdx, "day"),
			Views:             rowInt(row, colIdx, "views"),
			EstimatedMinutes:  rowFloat(row, colIdx, "estimatedMinutesWatched"),
			AvgRetention:      rowFloat(row, colIdx, "averageViewPercentage"),
			SubscribersGained: rowInt(row, colIdx, "subscribersGained"),
			SubscribersLost:   rowInt(row, colIdx, "subscribersLost"),
			Revenue:           rowFloat(row, colIdx, "estimatedRevenue"),
		})
		rowsApplied++
	}
	return rowsApplied
}

// mergeCategoricalRows applies rows from a query whose dimension is a
// categorical bucket (`insightTrafficSourceType` or `subscribedStatus`). The
// `category` argument names the dimension column.
func mergeCategoricalRows(rows [][]interface{}, colIdx map[string]int, videoIndex map[string]int, videos []Video, category string, target string) (rowsApplied int) {
	// Clear the target map on any video that appears, then repopulate.
	touched := map[string]bool{}
	for _, row := range rows {
		vid, ok := rowString(row, colIdx, "video")
		if !ok {
			continue
		}
		if !touched[vid] {
			if idx, exists := videoIndex[vid]; exists {
				switch target {
				case "traffic_sources":
					videos[idx].TrafficSources = map[string]TrafficSourceMetric{}
				case "sub_status_metrics":
					videos[idx].SubStatusMetrics = map[string]TrafficSourceMetric{}
				}
			}
			touched[vid] = true
		}
	}
	for _, row := range rows {
		vid, ok := rowString(row, colIdx, "video")
		if !ok {
			continue
		}
		idx, exists := videoIndex[vid]
		if !exists {
			continue
		}
		bucket, ok := rowString(row, colIdx, category)
		if !ok || bucket == "" {
			continue
		}
		m := TrafficSourceMetric{
			Views:    rowInt(row, colIdx, "views"),
			WatchMin: rowFloat(row, colIdx, "estimatedMinutesWatched"),
		}
		switch target {
		case "traffic_sources":
			videos[idx].TrafficSources[bucket] = m
		case "sub_status_metrics":
			videos[idx].SubStatusMetrics[bucket] = m
		}
		rowsApplied++
	}
	return rowsApplied
}

// optString is a non-erroring rowString that returns "" when the column is
// absent or the cell is non-string. The "day" column is always present in
// daily queries but defensive-program for response schema drift.
func optString(row []interface{}, colIdx map[string]int, col string) string {
	s, _ := rowString(row, colIdx, col)
	return s
}
