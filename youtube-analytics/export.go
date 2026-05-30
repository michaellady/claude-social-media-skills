package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func runExport(dataPath string, filter VideoFilter, format, output string) error {
	data, err := loadData(dataPath)
	if err != nil {
		return err
	}
	videos := filter.Apply(data.Videos)

	var w *csv.Writer
	if output == "" {
		w = csv.NewWriter(os.Stdout)
	} else {
		f, err := os.Create(output)
		if err != nil {
			return fmt.Errorf("creating %s: %w", output, err)
		}
		defer f.Close()
		w = csv.NewWriter(f)
	}
	switch strings.ToLower(format) {
	case "tsv":
		w.Comma = '\t'
	case "csv":
		w.Comma = ','
	default:
		return fmt.Errorf("--format must be csv or tsv (got %q)", format)
	}
	defer w.Flush()

	header := []string{
		"id", "title", "published_at", "duration_seconds", "video_type",
		"view_count", "like_count", "comment_count",
		"subscribers_gained", "subscribers_lost",
		"estimated_minutes_watched", "average_view_percentage", "average_view_duration",
		"estimated_revenue", "cpm", "ad_impressions", "monetized_playbacks",
		"tag_count", "has_thumbnail", "description_length",
	}
	if err := w.Write(header); err != nil {
		return err
	}
	for _, v := range videos {
		row := []string{
			v.ID,
			v.Title,
			v.PublishedAt.Format("2006-01-02T15:04:05Z"),
			strconv.Itoa(v.DurationSeconds),
			v.VideoType,
			strconv.FormatInt(v.ViewCount, 10),
			strconv.FormatInt(v.LikeCount, 10),
			strconv.FormatInt(v.CommentCount, 10),
			strconv.FormatInt(v.SubscribersGained, 10),
			strconv.FormatInt(v.SubscribersLost, 10),
			strconv.FormatFloat(v.EstimatedMinutesWatched, 'f', 2, 64),
			strconv.FormatFloat(v.AverageViewPercentage, 'f', 2, 64),
			strconv.FormatFloat(v.AverageViewDuration, 'f', 2, 64),
			strconv.FormatFloat(v.EstimatedRevenue, 'f', 4, 64),
			strconv.FormatFloat(v.CPM, 'f', 2, 64),
			strconv.FormatInt(v.AdImpressions, 10),
			strconv.FormatInt(v.MonetizedPlaybacks, 10),
			strconv.Itoa(len(v.Tags)),
			strconv.FormatBool(v.ThumbnailURL != ""),
			strconv.Itoa(len(v.Description)),
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	if output != "" {
		fmt.Fprintf(os.Stderr, "Exported %d videos to %s\n", len(videos), output)
	}
	return nil
}
