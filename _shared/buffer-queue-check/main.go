// buffer-queue-check — filter Buffer posts (queued or recently sent) by distinctive phrases.
//
// Pure transport. No cognition. Reads list_posts JSON from stdin (the skill calls
// mcp__buffer__list_posts inside the Claude harness and pipes the result here),
// returns matching posts as JSON.
//
// This avoids introducing a separate Buffer API token auth path — the skill keeps
// using the Buffer MCP for auth; this binary just does the deterministic substring
// match + per-channel grouping.
//
// Usage:
//
//	cat list_posts_response.json | buffer-queue-check \
//	  --keywords "phrase1,phrase2,phrase3" \
//	  [--channel-id <id>] \
//	  [--max-text-length 5000]
//
// Output: JSON dict { matches_per_keyword: { "phrase1": [post, post, ...], ... },
//                     total_posts_scanned: N, total_matches: M }
//
// Exit codes: 0=ok, 64=usage error, 65=input parse error.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

// Subset of Buffer's list_posts response we actually use.
type listPostsResponse struct {
	Edges []struct {
		Node struct {
			ID             string   `json:"id"`
			Status         string   `json:"status"`
			Text           string   `json:"text"`
			DueAt          string   `json:"dueAt"`
			SentAt         string   `json:"sentAt"`
			CreatedAt      string   `json:"createdAt"`
			ChannelID      string   `json:"channelId"`
			ChannelService string   `json:"channelService"`
			Tags           []any    `json:"tags"`
		} `json:"node"`
	} `json:"edges"`
}

type matchedPost struct {
	ID             string `json:"id"`
	Status         string `json:"status"`
	DueAt          string `json:"dueAt,omitempty"`
	SentAt         string `json:"sentAt,omitempty"`
	ChannelID      string `json:"channelId"`
	ChannelService string `json:"channelService"`
	TextSnippet    string `json:"textSnippet"`
}

type output struct {
	MatchesPerKeyword map[string][]matchedPost `json:"matches_per_keyword"`
	TotalPostsScanned int                      `json:"total_posts_scanned"`
	TotalMatches      int                      `json:"total_matches"`
}

func main() {
	var (
		keywordsFlag = flag.String("keywords", "", "comma-separated distinctive phrases to match (case-insensitive substring)")
		channelID    = flag.String("channel-id", "", "optional: filter to one channel ID")
		maxLen       = flag.Int("max-text-length", 200, "max chars of post.text to include in output snippet")
	)
	flag.Parse()

	if strings.TrimSpace(*keywordsFlag) == "" {
		fail(64, "missing --keywords (comma-separated)")
	}
	keywords := strings.Split(*keywordsFlag, ",")
	for i := range keywords {
		keywords[i] = strings.TrimSpace(keywords[i])
	}

	rawIn, err := io.ReadAll(os.Stdin)
	if err != nil {
		fail(65, "read stdin: "+err.Error())
	}
	if len(rawIn) == 0 {
		fail(64, "empty stdin (expected mcp__buffer__list_posts response JSON)")
	}

	var resp listPostsResponse
	if err := json.Unmarshal(rawIn, &resp); err != nil {
		fail(65, "parse stdin JSON: "+err.Error())
	}

	out := output{
		MatchesPerKeyword: make(map[string][]matchedPost, len(keywords)),
	}
	for _, kw := range keywords {
		out.MatchesPerKeyword[kw] = []matchedPost{}
	}

	for _, e := range resp.Edges {
		n := e.Node
		out.TotalPostsScanned++
		if *channelID != "" && n.ChannelID != *channelID {
			continue
		}
		textLower := strings.ToLower(n.Text)
		for _, kw := range keywords {
			if strings.Contains(textLower, strings.ToLower(kw)) {
				snippet := n.Text
				if len(snippet) > *maxLen {
					snippet = snippet[:*maxLen]
				}
				out.MatchesPerKeyword[kw] = append(out.MatchesPerKeyword[kw], matchedPost{
					ID:             n.ID,
					Status:         n.Status,
					DueAt:          n.DueAt,
					SentAt:         n.SentAt,
					ChannelID:      n.ChannelID,
					ChannelService: n.ChannelService,
					TextSnippet:    snippet,
				})
				out.TotalMatches++
			}
		}
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fail(70, "json encode: "+err.Error())
	}
}

func fail(code int, msg string) {
	fmt.Fprintln(os.Stderr, "buffer-queue-check: "+msg)
	os.Exit(code)
}
