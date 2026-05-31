// youtube.go — ingest the author's recent YouTube *livestream* transcripts into
// the voice corpus. Livestreams (video_type=="live") capture the author's
// spontaneous spoken register; long-form videos are excluded (they're just the
// author reading the newsletter, which the beehiiv slice already covers) and so
// are Shorts (clip captions, not authored voice).
//
// Pure transport: read the video list, shell out to the proven transcript
// fetcher, truncate, tag source_type. No cleaning or summarization happens here
// (that's cognition for the consumer skill). Transcript failures are per-video
// and never fatal — the newsletter slice must always survive.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"
)

type ytVideo struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	PublishedAt string `json:"published_at"`
	VideoType   string `json:"video_type"`
}

type ytVideosFile struct {
	Videos []ytVideo `json:"videos"`
}

// fetchYouTubeTranscripts returns the transcribed livestream posts plus the
// number of livestreams it attempted (so the caller can report "n/m fetched").
// An error is returned only for setup problems that prevent ingestion entirely
// (missing video list, missing transcript script) — per-video failures are
// logged to stderr and skipped.
func fetchYouTubeTranscripts(exeDir string, cfg config) ([]post, int, error) {
	videosPath := resolveRelative(exeDir, cfg.YouTubeVideosPath)
	scriptPath := resolveRelative(exeDir, cfg.YouTubeTranscriptScript)

	if _, err := os.Stat(scriptPath); err != nil {
		return nil, 0, fmt.Errorf("transcript script not found at %s: %w", scriptPath, err)
	}

	videos, err := loadLiveVideos(videosPath)
	if err != nil {
		return nil, 0, err
	}

	videos = sortAndLimitVideos(videos, cfg.YouTubeNumRecent)

	posts := make([]post, 0, len(videos))
	for _, v := range videos {
		text, err := runTranscript(scriptPath, v.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "voice-corpus: skip livestream %s (%s): %v\n", v.ID, v.Title, err)
			continue
		}
		text = collapseWhitespace(text)
		if text == "" {
			fmt.Fprintf(os.Stderr, "voice-corpus: skip livestream %s (%s): empty transcript\n", v.ID, v.Title)
			continue
		}
		if cfg.YouTubeMaxCharsPerPost > 0 && len(text) > cfg.YouTubeMaxCharsPerPost {
			text = text[:cfg.YouTubeMaxCharsPerPost]
		}
		posts = append(posts, post{
			Title:       v.Title,
			URL:         "https://www.youtube.com/watch?v=" + v.ID,
			PublishedAt: shortDate(v.PublishedAt),
			SourceType:  "youtube_live",
			BodyText:    text,
		})
	}
	return posts, len(videos), nil
}

// sortAndLimitVideos orders videos most-recent first (published_at is RFC3339, so
// lexical sort == chronological) and, when numRecent > 0, keeps only the first
// numRecent. numRecent <= 0 keeps all. Sort is stable so ties preserve input order.
func sortAndLimitVideos(videos []ytVideo, numRecent int) []ytVideo {
	sort.SliceStable(videos, func(i, j int) bool {
		return videos[i].PublishedAt > videos[j].PublishedAt
	})
	if numRecent > 0 && len(videos) > numRecent {
		videos = videos[:numRecent]
	}
	return videos
}

// loadLiveVideos reads videos.json and returns only video_type=="live" entries.
// Tolerates both the {videos:[...]} wrapper and a bare [...] array.
func loadLiveVideos(path string) ([]ytVideo, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read video list %s: %w", path, err)
	}
	var file ytVideosFile
	if err := json.Unmarshal(raw, &file); err != nil || len(file.Videos) == 0 {
		// Fall back to a bare array.
		var bare []ytVideo
		if err2 := json.Unmarshal(raw, &bare); err2 == nil && len(bare) > 0 {
			file.Videos = bare
		} else if err != nil {
			return nil, fmt.Errorf("parse video list %s: %w", path, err)
		}
	}
	live := make([]ytVideo, 0, len(file.Videos))
	for _, v := range file.Videos {
		if v.VideoType == "live" && v.ID != "" {
			live = append(live, v)
		}
	}
	return live, nil
}

// runTranscript shells out to generate-transcript.sh (which wraps
// youtube_transcript_api). stdout is the transcript; a non-zero exit means
// captions are unavailable / private / region-locked / the package is missing.
func runTranscript(scriptPath, videoID string) (string, error) {
	cmd := exec.Command("bash", scriptPath, videoID)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return "", fmt.Errorf("%s", firstLine(string(ee.Stderr)))
		}
		return "", err
	}
	return string(out), nil
}

// resolveRelative joins p onto base unless p is already absolute.
func resolveRelative(base, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(base, p)
}

// shortDate trims an RFC3339 timestamp to YYYY-MM-DD; passes through anything else.
func shortDate(s string) string {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.Format("2006-01-02")
	}
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

func firstLine(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return s[:i]
		}
	}
	return s
}
