package main

import (
	"fmt"
	"strings"
	"time"
)

// Video represents a single YouTube video with all metadata and statistics.
type Video struct {
	ID              string    `json:"id"`
	Title           string    `json:"title"`
	Description     string    `json:"description"`
	PublishedAt     time.Time `json:"published_at"`
	ThumbnailURL    string    `json:"thumbnail_url"`
	Duration        string    `json:"duration"`
	DurationSeconds int       `json:"duration_seconds"`
	ViewCount       int64     `json:"view_count"`
	LikeCount       int64     `json:"like_count"`
	CommentCount    int64     `json:"comment_count"`
	Tags            []string  `json:"tags,omitempty"`
	VideoType       string    `json:"video_type"` // "short", "long-form", "live"

	// Analytics API fields (only populated after fetch-analytics)
	EstimatedMinutesWatched float64 `json:"estimated_minutes_watched,omitempty"`
	AverageViewDuration     float64 `json:"average_view_duration,omitempty"`     // seconds
	AverageViewPercentage   float64 `json:"average_view_percentage,omitempty"`   // 0-100
	SubscribersGained       int64   `json:"subscribers_gained,omitempty"`
	SubscribersLost         int64   `json:"subscribers_lost,omitempty"`

	// Monetization fields (requires yt-analytics-monetary.readonly scope)
	EstimatedRevenue float64 `json:"estimated_revenue,omitempty"` // USD
	AdImpressions    int64   `json:"ad_impressions,omitempty"`
	CPM              float64 `json:"cpm,omitempty"`                // cost per mille
	MonetizedPlaybacks int64 `json:"monetized_playbacks,omitempty"`

	// Optional dimensional breakdowns. Only populated when the matching
	// fetch-analytics flag is passed. Aggregate fields above stay populated
	// regardless — these are additive context, not replacements.
	DailyMetrics     []DailyMetric                  `json:"daily_metrics,omitempty"`
	TrafficSources   map[string]TrafficSourceMetric `json:"traffic_sources,omitempty"`   // key: insightTrafficSourceType
	SubStatusMetrics map[string]TrafficSourceMetric `json:"sub_status_metrics,omitempty"` // keys: SUBSCRIBED|UNSUBSCRIBED
}

// DailyMetric is one row from a `Dimensions("video,day")` Analytics query.
// Optional fields (retention, revenue) populate only when the channel has
// monetization scope and the underlying response includes them.
type DailyMetric struct {
	Date              string  `json:"date"` // YYYY-MM-DD
	Views             int64   `json:"views"`
	EstimatedMinutes  float64 `json:"estimated_minutes"`
	AvgRetention      float64 `json:"avg_retention,omitempty"`
	SubscribersGained int64   `json:"subs_gained,omitempty"`
	SubscribersLost   int64   `json:"subs_lost,omitempty"`
	Revenue           float64 `json:"revenue,omitempty"`
}

// TrafficSourceMetric is reused for both traffic-source and sub-status
// breakdowns since both shapes are "views + watch_min keyed by category".
type TrafficSourceMetric struct {
	Views    int64   `json:"views"`
	WatchMin float64 `json:"watch_min,omitempty"`
}

// ChannelData is the top-level structure saved to data/videos.json.
type ChannelData struct {
	ChannelID          string    `json:"channel_id"`
	ChannelTitle       string    `json:"channel_title"`
	Handle             string    `json:"handle"`
	FetchedAt          time.Time `json:"fetched_at"`
	HasAnalytics       bool      `json:"has_analytics,omitempty"`
	AnalyticsFetchedAt time.Time `json:"analytics_fetched_at,omitempty"`
	Videos             []Video   `json:"videos"`
}

// AnalysisResult holds all computed analysis data.
type AnalysisResult struct {
	TotalVideos int
	Shorts      int
	LongForm    int
	LiveStreams  int

	BulkPostingDays   []BulkPostingDay
	BulkPeriodPerf    PeriodPerformance
	NonBulkPeriodPerf PeriodPerformance

	ShortsOverTime   []TimeBucket
	LongFormOverTime []TimeBucket
	LiveOverTime     []TimeBucket

	TopByViews      []Video
	BottomByViews   []Video
	TopByEngagement []Video

	TitleInsights TitleInsights

	// Watch time analytics (only populated when HasAnalytics is true)
	HasAnalytics       bool
	WatchTime          WatchTimeAnalysis
	ShortsWatchTime    WatchTimeAnalysis
	LongFormWatchTime  WatchTimeAnalysis
	WatchTimeOverTime  []WatchTimeBucket
	TopByWatchTime     []Video
	TopByRetention     []Video
	TopBySubscribers   []Video
	LiveWatchTime      WatchTimeAnalysis
	SubsOverTime       []SubBucket

	// Monetization
	HasMonetization       bool
	Revenue               RevenueAnalysis
	ShortsRevenue         RevenueAnalysis
	LongFormRevenue       RevenueAnalysis
	LiveRevenue           RevenueAnalysis
	RevenueOverTime       []RevenueBucket
	TopByRevenue          []Video
}

// SubBucket groups subscriber data by month.
type SubBucket struct {
	Period        string
	Gained        int64
	Lost          int64
	Net           int64
	VideoCount    int
	SubsPerVideo  float64
}

// WatchTimeAnalysis holds aggregated watch time metrics.
type WatchTimeAnalysis struct {
	TotalMinutes       float64
	AvgMinutesPerVideo float64
	AvgViewDuration    float64 // seconds
	AvgRetention       float64 // 0-100
	SubscribersGained  int64
	SubscribersLost    int64
	NetSubscribers     int64
}

// WatchTimeBucket groups watch time data by month.
type WatchTimeBucket struct {
	Period             string
	TotalMinutes       float64
	AvgMinutesPerVideo float64
	AvgRetention       float64
	VideoCount         int
}

// BulkPostingDay records a date with 3+ shorts uploaded.
type BulkPostingDay struct {
	Date       string
	ShortCount int
}

// PeriodPerformance holds aggregated metrics for a time period.
type PeriodPerformance struct {
	VideoCount    int
	AvgViews      float64
	AvgLikes      float64
	AvgComments   float64
	AvgEngagement float64 // (likes+comments)/views
}

// TimeBucket groups video performance by a time period (week/month).
type TimeBucket struct {
	Period     string
	VideoCount int
	AvgViews   float64
	TotalViews int64
	AvgEng     float64
}

// RevenueAnalysis holds aggregated monetization metrics.
type RevenueAnalysis struct {
	TotalRevenue       float64
	AvgRevenuePerVideo float64
	TotalAdImpressions int64
	TotalMonetized     int64
	AvgCPM             float64
	RPM                float64 // revenue per mille (per 1K views)
}

// RevenueBucket groups revenue data by month.
type RevenueBucket struct {
	Period         string
	TotalRevenue   float64
	VideoCount     int
	AvgRevPerVideo float64
	TotalViews     int64
	RPM            float64
}

// VideoFilter restricts a []Video slice by published date, video type, duration, id, or cohort.
// A zero-valued field means "no bound" for that dimension.
type VideoFilter struct {
	Since             time.Time
	Until             time.Time
	Types             []string
	DurMinSec         int
	DurMaxSec         int
	ExcludeIDs        map[string]bool
	Cohorts           []string            // filter videos to ones in any of these cohorts
	CohortAssignments map[string][]string // videoID -> cohort IDs (loaded from data/cohort_assignments.json)
}

func (f VideoFilter) IsZero() bool {
	return f.Since.IsZero() && f.Until.IsZero() && len(f.Types) == 0 &&
		f.DurMinSec == 0 && f.DurMaxSec == 0 && len(f.ExcludeIDs) == 0 &&
		len(f.Cohorts) == 0
}

func (f VideoFilter) Apply(videos []Video) []Video {
	if f.IsZero() {
		return videos
	}
	typeSet := map[string]bool{}
	for _, t := range f.Types {
		typeSet[t] = true
	}
	cohortSet := map[string]bool{}
	for _, c := range f.Cohorts {
		cohortSet[c] = true
	}
	out := make([]Video, 0, len(videos))
	for _, v := range videos {
		if f.ExcludeIDs[v.ID] {
			continue
		}
		if !f.Since.IsZero() && v.PublishedAt.Before(f.Since) {
			continue
		}
		if !f.Until.IsZero() && v.PublishedAt.After(f.Until) {
			continue
		}
		if len(typeSet) > 0 && !typeSet[v.VideoType] {
			continue
		}
		if f.DurMinSec > 0 && v.DurationSeconds < f.DurMinSec {
			continue
		}
		if f.DurMaxSec > 0 && v.DurationSeconds > f.DurMaxSec {
			continue
		}
		if len(cohortSet) > 0 {
			matched := false
			for _, assigned := range f.CohortAssignments[v.ID] {
				if cohortSet[assigned] {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		out = append(out, v)
	}
	return out
}

// Describe returns a human-readable filter summary for report headers. Empty when filter is zero.
func (f VideoFilter) Describe() string {
	if f.IsZero() {
		return ""
	}
	var parts []string
	if !f.Since.IsZero() {
		parts = append(parts, "since "+f.Since.Format("2006-01-02"))
	}
	if !f.Until.IsZero() {
		parts = append(parts, "until "+f.Until.Format("2006-01-02"))
	}
	if len(f.Types) > 0 {
		parts = append(parts, "type="+strings.Join(f.Types, "|"))
	}
	if f.DurMinSec > 0 || f.DurMaxSec > 0 {
		lo, hi := "", ""
		if f.DurMinSec > 0 {
			lo = fmt.Sprintf("%d", f.DurMinSec)
		}
		if f.DurMaxSec > 0 {
			hi = fmt.Sprintf("%d", f.DurMaxSec)
		}
		parts = append(parts, fmt.Sprintf("duration=%s..%ss", lo, hi))
	}
	if len(f.ExcludeIDs) > 0 {
		parts = append(parts, fmt.Sprintf("excluded=%d", len(f.ExcludeIDs)))
	}
	if len(f.Cohorts) > 0 {
		parts = append(parts, "cohort="+strings.Join(f.Cohorts, "|"))
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

// TitleInsights compares title characteristics of top vs bottom performers.
type TitleInsights struct {
	AvgLenTop       float64
	AvgLenBottom    float64
	QuestionsTop    int
	QuestionsBottom int
	NumbersTop      int
	NumbersBottom   int
	CapsWordsTop    float64
	CapsWordsBottom float64
}
