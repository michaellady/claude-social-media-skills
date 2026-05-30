package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	peerChannelsYAMLPath = "data/peer-channels.yaml"
	peerVideosJSONPath   = "data/peer-videos.json"
)

// PeerChannel is one row in data/peer-channels.yaml — a YouTube channel we
// track for title-grounding inspiration.
type PeerChannel struct {
	Handle string `yaml:"handle" json:"handle"`
	Angle  string `yaml:"angle"  json:"angle"` // one-line editorial-angle summary
}

// PeerVideo is one fetched video from a peer channel. Snapshot-only — no
// analytics scope on peer channels (and never will).
type PeerVideo struct {
	ID              string    `json:"id"`
	Title           string    `json:"title"`
	PublishedAt     time.Time `json:"published_at"`
	ViewCount       int64     `json:"view_count"`
	LikeCount       int64     `json:"like_count,omitempty"`
	DurationSeconds int       `json:"duration_seconds"`
	VideoType       string    `json:"video_type"` // short | long-form | live
}

// PeerChannelVideos groups a peer channel's resolved metadata + selected videos.
type PeerChannelVideos struct {
	Handle       string      `json:"handle"`
	Angle        string      `json:"angle,omitempty"`
	ChannelID    string      `json:"channel_id"`
	ChannelTitle string      `json:"channel_title"`
	Videos       []PeerVideo `json:"videos"`
}

// PeerVideosFile is the on-disk shape of data/peer-videos.json — keyed by
// channel handle so the model can group titles by source.
type PeerVideosFile struct {
	FetchedAt time.Time                    `json:"fetched_at"`
	Channels  map[string]PeerChannelVideos `json:"channels"`
}

// loadPeerChannels reads the configured peer list. Missing file returns
// empty (fresh checkout tolerated — fetch-peers will error with a hint).
func loadPeerChannels(path string) ([]PeerChannel, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading peer-channels yaml: %w", err)
	}
	var out []PeerChannel
	if err := yaml.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing peer-channels yaml: %w", err)
	}
	// Normalize handles so downstream lookups don't have to.
	for i := range out {
		out[i].Handle = normalizeHandle(out[i].Handle)
	}
	return out, nil
}

// loadPeerVideos reads the cached peer videos file. Missing file returns
// an initialized empty file so callers can append.
func loadPeerVideos(path string) (*PeerVideosFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &PeerVideosFile{Channels: map[string]PeerChannelVideos{}}, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	out := &PeerVideosFile{}
	if err := json.Unmarshal(raw, out); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if out.Channels == nil {
		out.Channels = map[string]PeerChannelVideos{}
	}
	return out, nil
}

// savePeerVideos pretty-prints the file for human-readable diffs (the file
// is gitignored but devs may inspect it).
func savePeerVideos(path string, file *PeerVideosFile) error {
	if err := os.MkdirAll("data", 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0644)
}

// selectTopRecent applies the deterministic selection rules from the plan:
// recency window, then sort desc by ViewCount, then take top N. Pure
// transport — no judgment about what's "good," just rank within window.
func selectTopRecent(videos []PeerVideo, now time.Time, window time.Duration, n int) []PeerVideo {
	cutoff := now.Add(-window)
	filtered := make([]PeerVideo, 0, len(videos))
	for _, v := range videos {
		if v.PublishedAt.Before(cutoff) {
			continue
		}
		filtered = append(filtered, v)
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].ViewCount > filtered[j].ViewCount
	})
	if len(filtered) > n {
		filtered = filtered[:n]
	}
	return filtered
}

// normalizeHandle strips URL noise + ensures a leading "@" so config and
// API responses key the same way.
func normalizeHandle(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Strip protocols + youtube.com prefixes.
	for _, p := range []string{"https://", "http://", "www.", "youtube.com/", "m.youtube.com/"} {
		s = strings.TrimPrefix(s, p)
	}
	if !strings.HasPrefix(s, "@") {
		s = "@" + s
	}
	return s
}
