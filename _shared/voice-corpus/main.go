// voice-corpus — fetch the author's recent newsletters from beehiiv RSS, cache locally,
// and print as JSON for skills to inject into compose-phase prompts.
//
// Pure transport. No cognition. The judgment about which excerpts to use, how to weight them,
// or how to interpret the voice belongs in the caller skill's prompt — not here.
//
// Usage:
//
//	voice-corpus                  # fetch if cache stale, print cache JSON to stdout
//	voice-corpus --refresh        # force fetch, ignore cache age
//	voice-corpus --num 3          # override num_recent (use 0 for all)
//	voice-corpus --print-only     # print existing cache, do not fetch
//
// Output JSON shape:
//
//	{
//	  "fetched_at": "2026-04-27T...",
//	  "feed_url": "https://rss.beehiiv.com/feeds/9AbhG8CTgD.xml",
//	  "num_posts": 5,
//	  "posts": [
//	    {"title": "...", "url": "...", "published_at": "...", "body_text": "<plain text>"},
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
	BodyText    string `json:"body_text"`
}

type cache struct {
	FetchedAt time.Time `json:"fetched_at"`
	FeedURL   string    `json:"feed_url"`
	NumPosts  int       `json:"num_posts"`
	Posts     []post    `json:"posts"`
}

func main() {
	var (
		refresh   = flag.Bool("refresh", false, "force fetch, ignore cache age")
		numFlag   = flag.Int("num", -1, "override num_recent (-1 = use config; 0 = all in feed)")
		printOnly = flag.Bool("print-only", false, "print existing cache, do not fetch")
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

	cachePath := filepath.Join(exeDir, cfg.CachePath)

	if *printOnly {
		c, err := readCache(cachePath)
		if err != nil {
			fail(66, "read cache: "+err.Error())
		}
		writeJSON(os.Stdout, c)
		return
	}

	if !*refresh {
		if c, ok := readCacheIfFresh(cachePath, cfg.StaleDays); ok {
			writeJSON(os.Stdout, c)
			return
		}
	}

	posts, err := fetchAndParse(cfg.FeedURL, cfg.NumRecent, cfg.MaxCharsPerPost)
	if err != nil {
		fail(66, "fetch: "+err.Error())
	}

	c := cache{
		FetchedAt: time.Now().UTC(),
		FeedURL:   cfg.FeedURL,
		NumPosts:  len(posts),
		Posts:     posts,
	}
	if err := writeCache(cachePath, c); err != nil {
		fail(67, "write cache: "+err.Error())
	}
	writeJSON(os.Stdout, c)
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
		MaxCharsPerPost: 2000,
		StaleDays:       7,
		CachePath:       "cache.json",
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

func readCacheIfFresh(path string, staleDays int) (cache, bool) {
	c, err := readCache(path)
	if err != nil {
		return cache{}, false
	}
	age := time.Since(c.FetchedAt)
	if age > time.Duration(staleDays)*24*time.Hour {
		return cache{}, false
	}
	return c, true
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
