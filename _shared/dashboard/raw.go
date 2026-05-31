// raw.go — read-only loaders for per-platform snapshots, used ONLY for
// drill-down detail the flywheel snapshot doesn't aggregate (per-video scatter,
// per-post timelines, voice freshness). Aggregates (ROI, amplification,
// reconciled reach) always come from the flywheel snapshot — never recomputed
// here (that would duplicate flywheel's judgment-laden logic). All loaders
// tolerate a missing file by returning a zero value + ok=false.
package main

import (
	"encoding/json"
	"os"
	"sort"
	"time"
)

// ---------- voice corpus freshness ----------

type voiceFreshness struct {
	NewsletterFetchedAt       string `json:"newsletter_fetched_at"`
	YouTubeTranscriptFetchedAt string `json:"youtube_transcript_fetched_at"`
	NumNewsletter             int    `json:"num_newsletter"`
	NumYouTube                int    `json:"num_youtube"`
	Available                 bool   `json:"available"`
}

func loadVoiceFreshness() voiceFreshness {
	raw, err := os.ReadFile(voiceCachePath())
	if err != nil {
		return voiceFreshness{Available: false}
	}
	var c struct {
		FetchedAt        string `json:"fetched_at"`
		YouTubeFetchedAt string `json:"youtube_fetched_at"`
		Posts            []struct {
			SourceType string `json:"source_type"`
		} `json:"posts"`
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return voiceFreshness{Available: false}
	}
	vf := voiceFreshness{
		NewsletterFetchedAt:        c.FetchedAt,
		YouTubeTranscriptFetchedAt: c.YouTubeFetchedAt,
		Available:                  true,
	}
	for _, p := range c.Posts {
		switch p.SourceType {
		case "youtube_live", "youtube_transcript":
			vf.NumYouTube++
		default: // "" or "newsletter"
			vf.NumNewsletter++
		}
	}
	return vf
}

// ---------- YouTube videos (drill-down) ----------

type ytVideoLite struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	PublishedAt string  `json:"published_at"`
	VideoType   string  `json:"video_type"`
	Views       int64   `json:"views"`
	Likes       int64   `json:"likes"`
	Comments    int64   `json:"comments"`
	Retention   float64 `json:"retention"`
	SubsGained  int64   `json:"subs_gained"`
}

// loadYouTubeVideos returns the most-recent N videos (any type) for the
// per-video drill-down chart. ok=false if videos.json is absent.
func loadYouTubeVideos(limit int) ([]ytVideoLite, bool) {
	raw, err := os.ReadFile(ytVideosPath())
	if err != nil {
		return nil, false
	}
	var file struct {
		Videos []struct {
			ID                    string  `json:"id"`
			Title                 string  `json:"title"`
			PublishedAt           string  `json:"published_at"`
			VideoType             string  `json:"video_type"`
			ViewCount             int64   `json:"view_count"`
			LikeCount             int64   `json:"like_count"`
			CommentCount          int64   `json:"comment_count"`
			AverageViewPercentage float64 `json:"average_view_percentage"`
			SubscribersGained     int64   `json:"subscribers_gained"`
		} `json:"videos"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, false
	}
	out := make([]ytVideoLite, 0, len(file.Videos))
	for _, v := range file.Videos {
		out = append(out, ytVideoLite{
			ID: v.ID, Title: v.Title, PublishedAt: v.PublishedAt, VideoType: v.VideoType,
			Views: v.ViewCount, Likes: v.LikeCount, Comments: v.CommentCount,
			Retention: v.AverageViewPercentage, SubsGained: v.SubscribersGained,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PublishedAt > out[j].PublishedAt })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, true
}

// ---------- Buffer channels (drill-down; aggregate-only) ----------

func loadBufferChannels() (json.RawMessage, bool) {
	path := newestMatch(bufferCacheDir(), ".json")
	if path == "" {
		return nil, false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var probe struct {
		Channels json.RawMessage `json:"channels"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil || len(probe.Channels) == 0 {
		return nil, false
	}
	return probe.Channels, true
}

// ---------- LinkedIn recent posts (drill-down) ----------

func loadLinkedInPosts() (json.RawMessage, bool) {
	path := newestMatch(linkedinCacheDir(), ".json")
	if path == "" {
		return nil, false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var probe struct {
		Profile struct {
			RecentPosts json.RawMessage `json:"recent_posts"`
		} `json:"profile"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil || len(probe.Profile.RecentPosts) == 0 {
		return nil, false
	}
	return probe.Profile.RecentPosts, true
}

// snapshotAgeDays returns whole days since a YYYY-MM-DD date string; -1 if unparseable.
func snapshotAgeDays(date string, now time.Time) int {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return -1
	}
	return int(now.Sub(t).Hours() / 24)
}
