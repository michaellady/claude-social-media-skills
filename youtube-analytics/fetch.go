package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

func runFetch(apiKey, channelHandle string) error {
	ctx := context.Background()

	service, err := youtube.NewService(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return fmt.Errorf("creating YouTube service: %w", err)
	}

	// Resolve channel handle → channel ID
	fmt.Printf("Resolving channel: %s\n", channelHandle)
	handle := strings.TrimPrefix(channelHandle, "@")
	channelResp, err := service.Channels.List([]string{"contentDetails", "snippet"}).
		ForHandle(handle).Do()
	if err != nil {
		return fmt.Errorf("looking up channel: %w", err)
	}
	if len(channelResp.Items) == 0 {
		return fmt.Errorf("no channel found for handle: %s", channelHandle)
	}

	channel := channelResp.Items[0]
	uploadsPlaylistID := channel.ContentDetails.RelatedPlaylists.Uploads
	fmt.Printf("Channel: %s (ID: %s)\n", channel.Snippet.Title, channel.Id)
	fmt.Printf("Uploads playlist: %s\n", uploadsPlaylistID)

	// Paginate through all video IDs in the uploads playlist
	var videoIDs []string
	pageToken := ""
	for {
		req := service.PlaylistItems.List([]string{"contentDetails"}).
			PlaylistId(uploadsPlaylistID).
			MaxResults(50)
		if pageToken != "" {
			req = req.PageToken(pageToken)
		}

		resp, err := req.Do()
		if err != nil {
			return fmt.Errorf("listing playlist items: %w", err)
		}

		for _, item := range resp.Items {
			videoIDs = append(videoIDs, item.ContentDetails.VideoId)
		}
		fmt.Printf("  Fetched %d video IDs (total: %d)\n", len(resp.Items), len(videoIDs))

		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	// Batch fetch full video details (50 at a time)
	var videos []Video
	for i := 0; i < len(videoIDs); i += 50 {
		end := i + 50
		if end > len(videoIDs) {
			end = len(videoIDs)
		}
		batch := videoIDs[i:end]

		resp, err := service.Videos.List([]string{"snippet", "statistics", "contentDetails", "liveStreamingDetails"}).
			Id(strings.Join(batch, ",")).Do()
		if err != nil {
			return fmt.Errorf("fetching video details: %w", err)
		}

		for _, v := range resp.Items {
			video := Video{
				ID:              v.Id,
				Title:           v.Snippet.Title,
				Description:     v.Snippet.Description,
				ThumbnailURL:    bestThumbnail(v.Snippet.Thumbnails),
				Duration:        v.ContentDetails.Duration,
				DurationSeconds: parseDuration(v.ContentDetails.Duration),
				Tags:            v.Snippet.Tags,
			}

			if t, err := time.Parse(time.RFC3339, v.Snippet.PublishedAt); err == nil {
				video.PublishedAt = t
			}

			if v.Statistics != nil {
				video.ViewCount = int64(v.Statistics.ViewCount)
				video.LikeCount = int64(v.Statistics.LikeCount)
				video.CommentCount = int64(v.Statistics.CommentCount)
			}

			video.VideoType = categorizeVideo(v)
			videos = append(videos, video)
		}
		fmt.Printf("  Fetched details for %d videos (batch %d-%d)\n", len(resp.Items), i+1, end)
	}

	// Sort by published date (oldest first)
	sort.Slice(videos, func(i, j int) bool {
		return videos[i].PublishedAt.Before(videos[j].PublishedAt)
	})

	data := ChannelData{
		ChannelID:    channel.Id,
		ChannelTitle: channel.Snippet.Title,
		Handle:       channelHandle,
		FetchedAt:    time.Now(),
		Videos:       videos,
	}

	if err := os.MkdirAll("data", 0755); err != nil {
		return fmt.Errorf("creating data directory: %w", err)
	}

	f, err := os.Create("data/videos.json")
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		return fmt.Errorf("encoding JSON: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing output file: %w", err)
	}

	if snapPath, err := snapshotVideosJSON("data/videos.json"); err != nil {
		fmt.Fprintf(os.Stderr, "  (warning: snapshot failed: %v)\n", err)
	} else {
		fmt.Printf("  Snapshot saved: %s\n", snapPath)
		if err := pruneSnapshots(snapshotKeep); err != nil {
			fmt.Fprintf(os.Stderr, "  (warning: snapshot prune failed: %v)\n", err)
		}
	}

	shorts := countByType(videos, "short")
	longform := countByType(videos, "long-form")
	live := countByType(videos, "live")

	fmt.Printf("\nSaved %d videos to data/videos.json\n", len(videos))
	fmt.Printf("  Shorts:       %d\n", shorts)
	fmt.Printf("  Long-form:    %d\n", longform)
	fmt.Printf("  Live streams: %d\n", live)

	return nil
}

func categorizeVideo(v *youtube.Video) string {
	if v.LiveStreamingDetails != nil {
		if v.LiveStreamingDetails.ActualStartTime != "" || v.LiveStreamingDetails.ActualEndTime != "" {
			return "live"
		}
	}
	if v.Snippet.LiveBroadcastContent != "" && v.Snippet.LiveBroadcastContent != "none" {
		return "live"
	}

	duration := parseDuration(v.ContentDetails.Duration)
	if duration > 0 && duration <= 60 {
		return "short"
	}

	return "long-form"
}

var durationRe = regexp.MustCompile(`PT(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?`)

func parseDuration(iso string) int {
	matches := durationRe.FindStringSubmatch(iso)
	if matches == nil {
		return 0
	}
	var h, m, s int
	if matches[1] != "" {
		h, _ = strconv.Atoi(matches[1])
	}
	if matches[2] != "" {
		m, _ = strconv.Atoi(matches[2])
	}
	if matches[3] != "" {
		s, _ = strconv.Atoi(matches[3])
	}
	return h*3600 + m*60 + s
}

func bestThumbnail(t *youtube.ThumbnailDetails) string {
	if t == nil {
		return ""
	}
	if t.Maxres != nil {
		return t.Maxres.Url
	}
	if t.High != nil {
		return t.High.Url
	}
	if t.Medium != nil {
		return t.Medium.Url
	}
	if t.Default != nil {
		return t.Default.Url
	}
	return ""
}

func countByType(videos []Video, vtype string) int {
	n := 0
	for _, v := range videos {
		if v.VideoType == vtype {
			n++
		}
	}
	return n
}
