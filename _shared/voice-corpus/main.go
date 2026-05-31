// voice-corpus — fetch the author's recent newsletters from beehiiv RSS, cache locally,
// and print as JSON for skills to inject into compose-phase prompts.
//
// Pure transport. No cognition. The judgment about which excerpts to use, how to weight them,
// or how to interpret the voice belongs in the caller skill's prompt — not here.
//
// The corpus optionally also ingests recent YouTube *livestream* transcripts
// (video_type=="live" only — long-form videos are just the author reading the
// newsletter, and Shorts are clip captions). Newsletter and livestream slices
// refresh on independent TTLs and are merged into one flat posts[] array,
// each post tagged with source_type so consumer skills can weight them.
//
// Usage:
//
//	voice-corpus                  # fetch if cache stale, print cache JSON to stdout
//	voice-corpus --refresh        # force fetch, ignore cache age (both sources)
//	voice-corpus --num 3          # override num_recent (use 0 for all) — newsletters
//	voice-corpus --print-only     # print existing cache, do not fetch
//	voice-corpus --youtube        # force-enable livestream transcript ingestion
//	voice-corpus --no-youtube     # force-disable livestream transcript ingestion
//
// Output JSON shape:
//
//	{
//	  "fetched_at": "2026-04-27T...",            # newsletter slice fetch time
//	  "youtube_fetched_at": "2026-04-27T...",    # livestream slice fetch time (if ingested)
//	  "feed_url": "https://rss.beehiiv.com/feeds/9AbhG8CTgD.xml",
//	  "num_posts": 5,
//	  "posts": [
//	    {"title": "...", "url": "...", "published_at": "...", "source_type": "newsletter", "body_text": "<plain text>"},
//	    {"title": "...", "url": "...", "published_at": "...", "source_type": "youtube_live", "body_text": "<transcript>"},
//	    ...
//	  ]
//	}
//
// Exit codes: 0=ok, 64=usage, 65=config parse error, 66=fetch error, 67=cache write error.
package main

import (
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/net/html"
)

type config struct {
	FeedURL         string `json:"feed_url"`
	NumRecent       int    `json:"num_recent"`
	MaxCharsPerPost int    `json:"max_chars_per_post"`
	StaleDays       int    `json:"stale_days"`
	CachePath       string `json:"cache_path"`

	// YouTube livestream ingestion (Pillar 2). All optional; ingestion is off
	// unless IngestYouTube is true (or forced via --youtube).
	IngestYouTube           bool   `json:"ingest_youtube"`
	YouTubeVideosPath       string `json:"youtube_videos_path"`        // relative to the binary dir, or absolute
	YouTubeTranscriptScript string `json:"youtube_transcript_script"`  // relative to the binary dir, or absolute
	YouTubeNumRecent        int    `json:"youtube_num_recent"`         // most-recent N livestreams to transcribe
	YouTubeMaxCharsPerPost  int    `json:"youtube_max_chars_per_post"` // per-transcript truncation cap
	YouTubeStaleDays        int    `json:"youtube_stale_days"`         // separate TTL (archives don't change)
}

type rssFeed struct {
	XMLName xml.Name `xml:"rss"`
	Channel struct {
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
}

type rssItem struct {
	Title          string `xml:"title"`
	Link           string `xml:"link"`
	PubDate        string `xml:"pubDate"`
	Description    string `xml:"description"`
	ContentEncoded string `xml:"http://purl.org/rss/1.0/modules/content/ encoded"`
}

type post struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	PublishedAt string `json:"published_at"`
	SourceType  string `json:"source_type"` // "newsletter" | "youtube_live"
	BodyText    string `json:"body_text"`
}

type cache struct {
	FetchedAt        time.Time `json:"fetched_at"`                  // newsletter slice
	YouTubeFetchedAt time.Time `json:"youtube_fetched_at,omitempty"` // livestream slice
	FeedURL          string    `json:"feed_url"`
	NumPosts         int       `json:"num_posts"`
	Posts            []post    `json:"posts"`
}

func main() {
	var (
		refresh    = flag.Bool("refresh", false, "force fetch, ignore cache age (both sources)")
		numFlag    = flag.Int("num", -1, "override num_recent (-1 = use config; 0 = all in feed)")
		printOnly  = flag.Bool("print-only", false, "print existing cache, do not fetch")
		youtubeOn  = flag.Bool("youtube", false, "force-enable livestream transcript ingestion")
		youtubeOff = flag.Bool("no-youtube", false, "force-disable livestream transcript ingestion")
	)
	flag.Parse()

	exeDir, err := exeDir()
	if err != nil {
		fail(64, "find executable dir: "+err.Error())
	}

	cfg, err := loadConfig(filepath.Join(exeDir, "config.json"), filepath.Join(exeDir, "config.local.json"))
	if err != nil {
		fail(65, "load config: "+err.Error())
	}
	if *numFlag >= 0 {
		cfg.NumRecent = *numFlag
	}

	// Resolve livestream ingestion: config default, overridable by flags (off wins).
	ingestYT := cfg.IngestYouTube
	if *youtubeOn {
		ingestYT = true
	}
	if *youtubeOff {
		ingestYT = false
	}

	cachePath := filepath.Join(exeDir, cfg.CachePath)

	if *printOnly {
		c, err := readCache(cachePath)
		if err != nil {
			fail(66, "read cache: "+err.Error())
		}
		writeJSON(os.Stdout, c)
		return
	}

	// Load any existing cache so each source can reuse a still-fresh slice.
	existing, _ := readCache(cachePath) // zero value if missing/unreadable
	now := time.Now().UTC()

	// ---- Newsletter slice (own TTL) ----
	newsletterPosts := filterBySource(existing.Posts, "newsletter")
	newsletterFetchedAt := existing.FetchedAt
	if *refresh || isStale(existing.FetchedAt, cfg.StaleDays) || len(newsletterPosts) == 0 {
		np, err := fetchAndParse(cfg.FeedURL, cfg.NumRecent, cfg.MaxCharsPerPost)
		if err != nil {
			// Never lose newsletters: degrade to the cached slice if we have one.
			if len(newsletterPosts) == 0 {
				fail(66, "fetch: "+err.Error())
			}
			fmt.Fprintln(os.Stderr, "voice-corpus: newsletter fetch failed; using cached newsletters: "+err.Error())
		} else {
			newsletterPosts = np
			newsletterFetchedAt = now
		}
	}
	for i := range newsletterPosts {
		newsletterPosts[i].SourceType = "newsletter" // stamp legacy/reused posts
	}

	// ---- Livestream slice (own TTL; never fatal — newsletters must always survive) ----
	youtubePosts := filterBySource(existing.Posts, "youtube_live")
	youtubeFetchedAt := existing.YouTubeFetchedAt
	if ingestYT {
		if *refresh || isStale(existing.YouTubeFetchedAt, cfg.YouTubeStaleDays) || len(youtubePosts) == 0 {
			yp, n, err := fetchYouTubeTranscripts(exeDir, cfg)
			if err != nil {
				fmt.Fprintln(os.Stderr, "voice-corpus: livestream ingestion skipped: "+err.Error())
			} else {
				youtubePosts = yp
				youtubeFetchedAt = now
				fmt.Fprintf(os.Stderr, "voice-corpus: ingested %d/%d livestream transcripts\n", len(yp), n)
			}
		}
	} else {
		youtubePosts = nil // disabled → drop any cached livestream slice from output
	}

	merged := make([]post, 0, len(newsletterPosts)+len(youtubePosts))
	merged = append(merged, newsletterPosts...)
	merged = append(merged, youtubePosts...)

	c := cache{
		FetchedAt:        newsletterFetchedAt,
		YouTubeFetchedAt: youtubeFetchedAt,
		FeedURL:          cfg.FeedURL,
		NumPosts:         len(merged),
		Posts:            merged,
	}
	if err := writeCache(cachePath, c); err != nil {
		fail(67, "write cache: "+err.Error())
	}
	writeJSON(os.Stdout, c)
}

// isStale reports whether t is older than days (a zero time is always stale).
func isStale(t time.Time, days int) bool {
	if t.IsZero() {
		return true
	}
	return time.Since(t) > time.Duration(days)*24*time.Hour
}

// effectiveSource treats a missing source_type as "newsletter" so a pre-Pillar-2
// cache (written before the field existed) still classifies correctly on read.
func effectiveSource(p post) string {
	if p.SourceType == "" {
		return "newsletter"
	}
	return p.SourceType
}

func filterBySource(posts []post, source string) []post {
	out := make([]post, 0, len(posts))
	for _, p := range posts {
		if effectiveSource(p) == source {
			out = append(out, p)
		}
	}
	return out
}

func exeDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return "", err
	}
	return filepath.Dir(resolved), nil
}

func loadConfig(defaultPath, localPath string) (config, error) {
	cfg := config{
		FeedURL:         "https://rss.beehiiv.com/feeds/9AbhG8CTgD.xml",
		NumRecent:       0, // 0 = all items in the feed (~50 for beehiiv)
		MaxCharsPerPost: 50000,
		StaleDays:       7,
		CachePath:       "cache.json",

		// Livestream defaults: OFF unless config.json / config.local.json opts in.
		IngestYouTube:           false,
		YouTubeVideosPath:       "../../youtube-analytics/data/videos.json",
		YouTubeTranscriptScript: "../../youtube-analytics/scripts/generate-transcript.sh",
		YouTubeNumRecent:        10,
		YouTubeMaxCharsPerPost:  50000,
		YouTubeStaleDays:        30,
	}
	for _, p := range []string{defaultPath, localPath} {
		raw, err := os.ReadFile(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return cfg, fmt.Errorf("%s: %w", p, err)
		}
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return cfg, fmt.Errorf("%s: %w", p, err)
		}
	}
	return cfg, nil
}

func readCache(path string) (cache, error) {
	var c cache
	raw, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return c, err
	}
	return c, nil
}

func writeCache(path string, c cache) error {
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0644)
}

func writeJSON(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func fetchAndParse(feedURL string, n, maxChars int) ([]post, error) {
	req, err := http.NewRequest("GET", feedURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "voice-corpus/1.0 (+https://github.com/michaellady/claude-social-media-skills)")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, feedURL)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var feed rssFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("parse RSS: %w", err)
	}

	items := feed.Channel.Items
	// n == 0 means "all items in the feed"; otherwise cap at n.
	if n > 0 && len(items) > n {
		items = items[:n]
	}

	posts := make([]post, 0, len(items))
	for _, it := range items {
		raw := it.ContentEncoded
		if strings.TrimSpace(raw) == "" {
			raw = it.Description
		}
		text := htmlToText(raw)
		text = collapseWhitespace(text)
		if len(text) > maxChars {
			text = text[:maxChars]
		}
		posts = append(posts, post{
			Title:       strings.TrimSpace(it.Title),
			URL:         strings.TrimSpace(it.Link),
			PublishedAt: normalizeDate(it.PubDate),
			SourceType:  "newsletter",
			BodyText:    text,
		})
	}
	return posts, nil
}

// htmlToText strips HTML tags using golang.org/x/net/html, dropping <script>/<style> contents.
func htmlToText(raw string) string {
	doc, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return raw
	}
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style":
				return
			}
		}
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
		}
		// Add a separator after block-level elements so paragraphs don't run together.
		if n.Type == html.ElementNode {
			switch n.Data {
			case "p", "br", "div", "h1", "h2", "h3", "h4", "h5", "h6", "li", "blockquote", "tr":
				defer sb.WriteString("\n")
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return sb.String()
}

func collapseWhitespace(s string) string {
	// Normalize CRLF, then collapse runs of spaces and limit consecutive newlines to 2.
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	var sb strings.Builder
	prevSpace := false
	prevNewlines := 0
	for _, r := range s {
		if r == '\n' {
			prevNewlines++
			if prevNewlines <= 2 {
				sb.WriteRune('\n')
			}
			prevSpace = false
			continue
		}
		prevNewlines = 0
		if r == ' ' || r == '\t' {
			if !prevSpace {
				sb.WriteRune(' ')
				prevSpace = true
			}
			continue
		}
		prevSpace = false
		sb.WriteRune(r)
	}
	return strings.TrimSpace(sb.String())
}

func normalizeDate(pubDate string) string {
	// RSS pubDate is RFC1123Z; convert to YYYY-MM-DD. On parse failure, return raw.
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822} {
		if t, err := time.Parse(layout, pubDate); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return strings.TrimSpace(pubDate)
}

func fail(code int, msg string) {
	fmt.Fprintln(os.Stderr, "voice-corpus: "+msg)
	os.Exit(code)
}
