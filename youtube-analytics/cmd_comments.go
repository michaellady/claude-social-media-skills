package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

const commentsJSONPath = "data/comments.json"

// CommentsFile is the on-disk shape of data/comments.json — keyed by video ID,
// with a fetched_at timestamp per video so we know how stale each entry is.
type CommentsFile struct {
	FetchedAt time.Time                 `json:"fetched_at"`
	Videos    map[string]VideoComments  `json:"videos"`
}

type VideoComments struct {
	VideoID   string    `json:"video_id"`
	FetchedAt time.Time `json:"fetched_at"`
	Comments  []Comment `json:"comments"`
}

// Comment is a top-level comment thread (replies are summarized via ReplyCount
// rather than fetched, since reply trees rarely add signal worth the quota).
type Comment struct {
	ID          string    `json:"id"`
	AuthorName  string    `json:"author_name,omitempty"`
	AuthorURL   string    `json:"author_url,omitempty"`
	Text        string    `json:"text"`
	LikeCount   int64     `json:"like_count,omitempty"`
	ReplyCount  int64     `json:"reply_count,omitempty"`
	PublishedAt time.Time `json:"published_at"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
}

func cmdFetchComments(args []string) error {
	fs := flag.NewFlagSet("fetch-comments", flag.ExitOnError)
	ff := addFilterFlags(fs)
	limit := fs.Int("limit", 20, "max top-level comments per video")
	order := fs.String("order", "relevance", "comment ordering: relevance|time")
	maxAge := fs.Duration("max-age", 7*24*time.Hour, "skip videos whose comments were fetched more recently than this (use 0 to refetch all)")
	force := fs.Bool("force", false, "ignore --max-age and refetch every filtered video")
	dryRun := fs.Bool("dry-run", false, "print which videos would be fetched (and the quota cost) without calling the API")
	if err := fs.Parse(args); err != nil {
		return err
	}
	filter, err := ff.build()
	if err != nil {
		return err
	}
	switch *order {
	case "relevance", "time":
	default:
		return fmt.Errorf("--order must be relevance|time, got %q", *order)
	}

	apiKey := os.Getenv("YOUTUBE_API_KEY")
	if apiKey == "" && !*dryRun {
		return fmt.Errorf("YOUTUBE_API_KEY must be set in .env")
	}
	data, err := loadData(videosJSONPath)
	if err != nil {
		return err
	}
	videos := filter.Apply(data.Videos)
	if !filter.IsZero() {
		fmt.Printf("Filter %s: %d of %d videos.\n", filter.Describe(), len(videos), len(data.Videos))
	}
	if len(videos) == 0 {
		return fmt.Errorf("no videos to fetch comments for")
	}

	existing, err := loadComments(commentsJSONPath)
	if err != nil {
		return err
	}

	// Staleness guard: skip videos with fresh comments unless --force.
	now := time.Now()
	var toFetch []Video
	staleSkipped := 0
	for _, v := range videos {
		if *force || *maxAge == 0 {
			toFetch = append(toFetch, v)
			continue
		}
		prev, ok := existing.Videos[v.ID]
		if !ok {
			toFetch = append(toFetch, v)
			continue
		}
		if now.Sub(prev.FetchedAt) > *maxAge {
			toFetch = append(toFetch, v)
			continue
		}
		staleSkipped++
	}

	// Preflight: commentThreads.list is 1 quota unit per call.
	fmt.Printf("Quota preflight: %d videos to fetch (~%d Data API units; %d skipped as fresh-within-%s)\n",
		len(toFetch), len(toFetch), staleSkipped, *maxAge)
	if *dryRun {
		fmt.Println("(dry-run: no API calls)")
		for _, v := range toFetch {
			fmt.Printf("  would fetch: %s  %s\n", v.ID, truncate(v.Title, 60))
		}
		return nil
	}
	if len(toFetch) == 0 {
		fmt.Println("Nothing to fetch.")
		return nil
	}

	ctx := context.Background()
	svc, err := youtube.NewService(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return fmt.Errorf("creating YouTube service: %w", err)
	}

	fetched := 0
	skipped := 0
	commentTotal := 0
	for i, v := range toFetch {
		// Comments are disabled or private on a non-trivial subset of videos —
		// the API returns 403 for those. Treat as a soft skip rather than abort.
		comments, err := fetchVideoComments(ctx, svc, v.ID, int64(*limit), *order)
		if err != nil {
			skipped++
			fmt.Printf("  [%d/%d] %s: skipped (%v)\n", i+1, len(toFetch), v.ID, err)
			continue
		}
		existing.Videos[v.ID] = VideoComments{
			VideoID:   v.ID,
			FetchedAt: now,
			Comments:  comments,
		}
		fetched++
		commentTotal += len(comments)
		if (i+1)%25 == 0 || i == len(toFetch)-1 {
			fmt.Printf("  [%d/%d] fetched=%d skipped=%d total_comments=%d\n",
				i+1, len(toFetch), fetched, skipped, commentTotal)
		}
	}

	existing.FetchedAt = now
	if err := saveComments(commentsJSONPath, existing); err != nil {
		return err
	}
	fmt.Printf("Wrote %s — %d videos with comments (%d skipped)\n", commentsJSONPath, fetched, skipped)
	return nil
}

func fetchVideoComments(ctx context.Context, svc *youtube.Service, videoID string, limit int64, order string) ([]Comment, error) {
	resp, err := svc.CommentThreads.List([]string{"snippet"}).
		VideoId(videoID).
		MaxResults(limit).
		Order(order).
		TextFormat("plainText").
		Context(ctx).
		Do()
	if err != nil {
		return nil, err
	}
	out := make([]Comment, 0, len(resp.Items))
	for _, item := range resp.Items {
		if item.Snippet == nil || item.Snippet.TopLevelComment == nil || item.Snippet.TopLevelComment.Snippet == nil {
			continue
		}
		s := item.Snippet.TopLevelComment.Snippet
		c := Comment{
			ID:         item.Snippet.TopLevelComment.Id,
			AuthorName: s.AuthorDisplayName,
			AuthorURL:  s.AuthorChannelUrl,
			Text:       s.TextDisplay,
			LikeCount:  s.LikeCount,
			ReplyCount: item.Snippet.TotalReplyCount,
		}
		if t, err := time.Parse(time.RFC3339, s.PublishedAt); err == nil {
			c.PublishedAt = t
		}
		if t, err := time.Parse(time.RFC3339, s.UpdatedAt); err == nil {
			c.UpdatedAt = t
		}
		out = append(out, c)
	}
	// Stable order: most-liked first, then most-recent. Helps `jq` slicing.
	sort.Slice(out, func(i, j int) bool {
		if out[i].LikeCount != out[j].LikeCount {
			return out[i].LikeCount > out[j].LikeCount
		}
		return out[i].PublishedAt.After(out[j].PublishedAt)
	})
	return out, nil
}

func loadComments(path string) (*CommentsFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &CommentsFile{Videos: map[string]VideoComments{}}, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	out := &CommentsFile{}
	if err := json.Unmarshal(raw, out); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if out.Videos == nil {
		out.Videos = map[string]VideoComments{}
	}
	return out, nil
}

func saveComments(path string, file *CommentsFile) error {
	if err := os.MkdirAll("data", 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0644)
}
