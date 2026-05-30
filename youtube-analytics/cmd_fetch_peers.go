package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

const (
	defaultPeerWindow      = 90 * 24 * time.Hour
	defaultPeerTopN        = 15
	peerPlaylistMaxFetched = 150 // cap pagination to keep quota predictable
)

func cmdFetchPeers(args []string) error {
	fs := flag.NewFlagSet("fetch-peers", flag.ExitOnError)
	window := fs.Duration("window", defaultPeerWindow, "recency window — only videos published within are eligible")
	top := fs.Int("top", defaultPeerTopN, "max videos kept per channel (after recency filter, ranked by views desc)")
	dryRun := fs.Bool("dry-run", false, "list channels we'd fetch + estimated quota; no API calls")
	if err := fs.Parse(args); err != nil {
		return err
	}

	channels, err := loadPeerChannels(peerChannelsYAMLPath)
	if err != nil {
		return err
	}
	if len(channels) == 0 {
		return fmt.Errorf("no peer channels in %s — see README for setup", peerChannelsYAMLPath)
	}

	// Quota preflight: 1 channels.list + 3 playlistItems pages + 3 videos.list batches per channel ≈ 7 units.
	estimatedQuota := len(channels) * 7
	fmt.Printf("Quota preflight: %d peer channels (~%d Data API units, %dd window, top %d/channel)\n",
		len(channels), estimatedQuota, int(window.Hours())/24, *top)
	for _, c := range channels {
		fmt.Printf("  - %s — %s\n", c.Handle, c.Angle)
	}
	if *dryRun {
		fmt.Println("(dry-run: no API calls)")
		return nil
	}

	apiKey := os.Getenv("YOUTUBE_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("YOUTUBE_API_KEY must be set in .env")
	}

	ctx := context.Background()
	svc, err := youtube.NewService(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return fmt.Errorf("creating YouTube service: %w", err)
	}

	out := &PeerVideosFile{
		FetchedAt: time.Now().UTC(),
		Channels:  map[string]PeerChannelVideos{},
	}
	now := time.Now()
	skipped := 0
	totalKept := 0
	for _, ch := range channels {
		fmt.Printf("\n--- %s\n", ch.Handle)
		entry, err := fetchPeer(ctx, svc, ch, now, *window, *top)
		if err != nil {
			// Non-fatal: handle deletions / rate limits don't kill the run.
			fmt.Printf("  skipped: %v\n", err)
			skipped++
			continue
		}
		out.Channels[ch.Handle] = entry
		totalKept += len(entry.Videos)
		fmt.Printf("  kept %d videos (channel: %s, top view: %s views)\n",
			len(entry.Videos), entry.ChannelTitle, fmtNum(topView(entry.Videos)))
	}

	if err := savePeerVideos(peerVideosJSONPath, out); err != nil {
		return err
	}
	fmt.Printf("\nWrote %s — %d/%d channels, %d videos total (%d skipped)\n",
		peerVideosJSONPath, len(out.Channels), len(channels), totalKept, skipped)
	return nil
}

// fetchPeer pulls one peer channel: handle → channel ID → uploads playlist
// → recent video IDs → bulk videos.list → top-N selection. Returns a
// populated PeerChannelVideos or an error if any pipeline stage fails.
func fetchPeer(ctx context.Context, svc *youtube.Service, ch PeerChannel, now time.Time, window time.Duration, top int) (PeerChannelVideos, error) {
	handle := strings.TrimPrefix(ch.Handle, "@")

	chResp, err := svc.Channels.List([]string{"contentDetails", "snippet"}).
		ForHandle(handle).Context(ctx).Do()
	if err != nil {
		return PeerChannelVideos{}, fmt.Errorf("channels.list: %w", err)
	}
	if len(chResp.Items) == 0 {
		return PeerChannelVideos{}, fmt.Errorf("no channel found for handle %q", ch.Handle)
	}
	channel := chResp.Items[0]
	uploadsPlaylistID := channel.ContentDetails.RelatedPlaylists.Uploads

	// Paginate uploads playlist for the most recent video IDs (capped).
	var videoIDs []string
	pageToken := ""
	for len(videoIDs) < peerPlaylistMaxFetched {
		req := svc.PlaylistItems.List([]string{"contentDetails"}).
			PlaylistId(uploadsPlaylistID).
			MaxResults(50).
			Context(ctx)
		if pageToken != "" {
			req = req.PageToken(pageToken)
		}
		resp, err := req.Do()
		if err != nil {
			return PeerChannelVideos{}, fmt.Errorf("playlistItems.list: %w", err)
		}
		for _, item := range resp.Items {
			if item.ContentDetails != nil {
				videoIDs = append(videoIDs, item.ContentDetails.VideoId)
			}
		}
		if resp.NextPageToken == "" || len(videoIDs) >= peerPlaylistMaxFetched {
			break
		}
		pageToken = resp.NextPageToken
	}
	if len(videoIDs) > peerPlaylistMaxFetched {
		videoIDs = videoIDs[:peerPlaylistMaxFetched]
	}

	// Bulk fetch video details in 50-id batches.
	var rows []PeerVideo
	for i := 0; i < len(videoIDs); i += 50 {
		end := i + 50
		if end > len(videoIDs) {
			end = len(videoIDs)
		}
		resp, err := svc.Videos.List([]string{"snippet", "statistics", "contentDetails", "liveStreamingDetails"}).
			Id(strings.Join(videoIDs[i:end], ",")).Context(ctx).Do()
		if err != nil {
			return PeerChannelVideos{}, fmt.Errorf("videos.list: %w", err)
		}
		for _, v := range resp.Items {
			if v.Snippet == nil || v.ContentDetails == nil {
				continue
			}
			pv := PeerVideo{
				ID:              v.Id,
				Title:           v.Snippet.Title,
				DurationSeconds: parseDuration(v.ContentDetails.Duration),
				VideoType:       categorizeVideo(v),
			}
			if t, err := time.Parse(time.RFC3339, v.Snippet.PublishedAt); err == nil {
				pv.PublishedAt = t
			}
			if v.Statistics != nil {
				pv.ViewCount = int64(v.Statistics.ViewCount)
				pv.LikeCount = int64(v.Statistics.LikeCount)
			}
			rows = append(rows, pv)
		}
	}

	return PeerChannelVideos{
		Handle:       ch.Handle,
		Angle:        ch.Angle,
		ChannelID:    channel.Id,
		ChannelTitle: channel.Snippet.Title,
		Videos:       selectTopRecent(rows, now, window, top),
	}, nil
}

func topView(videos []PeerVideo) int64 {
	if len(videos) == 0 {
		return 0
	}
	max := videos[0].ViewCount
	for _, v := range videos[1:] {
		if v.ViewCount > max {
			max = v.ViewCount
		}
	}
	return max
}
