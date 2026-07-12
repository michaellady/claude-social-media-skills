package videoedit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const (
	defaultMaxFeedBytes  = int64(32 << 20)
	defaultMaxImageBytes = int64(64 << 20)
)

// ArticleFetchOptions configures RSS retrieval and optional image downloads.
// DownloadDir may be empty, in which case image URLs are retained without
// downloading their contents.
type ArticleFetchOptions struct {
	FeedURL       string
	CanonicalURL  string
	DownloadDir   string
	RawSourcePath string
	HTTPClient    *http.Client
	MaxFeedBytes  int64
	MaxImageBytes int64
	Now           func() time.Time
}

// FetchArticle retrieves a Beehiiv-compatible RSS feed, selects the item whose
// link matches canonicalURL, and returns an ordered article snapshot. When
// downloadDir is non-empty, referenced images are downloaded under stable
// URL-derived names.
func FetchArticle(ctx context.Context, feedURL, canonicalURL, downloadDir string) (Article, error) {
	return FetchArticleWithOptions(ctx, ArticleFetchOptions{
		FeedURL:      feedURL,
		CanonicalURL: canonicalURL,
		DownloadDir:  downloadDir,
	})
}

// FetchArticleWithOptions is FetchArticle with injectable transport, limits,
// and clock for callers that need deterministic tests or stricter limits.
func FetchArticleWithOptions(ctx context.Context, opts ArticleFetchOptions) (Article, error) {
	if strings.TrimSpace(opts.FeedURL) == "" {
		return Article{}, errors.New("feed URL is required")
	}
	if strings.TrimSpace(opts.CanonicalURL) == "" {
		return Article{}, errors.New("canonical newsletter URL is required")
	}
	if _, err := url.ParseRequestURI(opts.FeedURL); err != nil {
		return Article{}, fmt.Errorf("parse feed URL: %w", err)
	}
	canonical, err := normalizeURL(opts.CanonicalURL)
	if err != nil {
		return Article{}, fmt.Errorf("parse canonical newsletter URL: %w", err)
	}

	client := opts.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	maxFeedBytes := opts.MaxFeedBytes
	if maxFeedBytes <= 0 {
		maxFeedBytes = defaultMaxFeedBytes
	}
	maxImageBytes := opts.MaxImageBytes
	if maxImageBytes <= 0 {
		maxImageBytes = defaultMaxImageBytes
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}

	feedBytes, err := fetchBytes(ctx, client, opts.FeedURL, maxFeedBytes)
	if err != nil {
		return Article{}, fmt.Errorf("fetch RSS feed: %w", err)
	}
	if opts.RawSourcePath != "" {
		if err := writeBytesAtomic(opts.RawSourcePath, feedBytes, 0o644); err != nil {
			return Article{}, fmt.Errorf("write raw RSS snapshot: %w", err)
		}
	}
	var feed rssDocument
	if err := xml.Unmarshal(feedBytes, &feed); err != nil {
		return Article{}, fmt.Errorf("parse RSS feed: %w", err)
	}

	item, ok := selectRSSItem(feed.Channel.Items, canonical)
	if !ok {
		return Article{}, fmt.Errorf("newsletter %q was not found in RSS feed", canonical)
	}
	itemCanonical := canonical
	if normalized, normalizeErr := normalizeURL(item.Link); normalizeErr == nil {
		itemCanonical = normalized
	}

	blocks, images, warnings, err := parseArticleHTML(item.Content, itemCanonical)
	if err != nil {
		return Article{}, fmt.Errorf("parse newsletter HTML: %w", err)
	}
	if len(blocks) == 0 {
		warnings = append(warnings, "RSS item contained no supported content blocks")
	}

	if opts.DownloadDir != "" {
		if err := os.MkdirAll(opts.DownloadDir, 0o755); err != nil {
			return Article{}, fmt.Errorf("create image download directory: %w", err)
		}
		for i := range images {
			if strings.TrimSpace(images[i].URL) == "" {
				warnings = append(warnings, fmt.Sprintf("image %s has no downloadable URL", images[i].ID))
				continue
			}
			imagePath, downloadErr := downloadArticleImage(ctx, client, images[i].URL, opts.DownloadDir, maxImageBytes)
			if downloadErr != nil {
				if ctx.Err() != nil {
					return Article{}, ctx.Err()
				}
				warnings = append(warnings, fmt.Sprintf("image %s download failed: %v", images[i].ID, downloadErr))
				continue
			}
			images[i].Path = imagePath
		}
	}

	publishedAt := normalizePublishedAt(item.PubDate)
	article := Article{
		SchemaVersion: CurrentSchemaVersion,
		SourceURL:     opts.FeedURL,
		CanonicalURL:  itemCanonical,
		Title:         normalizeText(item.Title),
		PublishedAt:   publishedAt,
		FetchedAt:     now().UTC(),
		RawSourcePath: opts.RawSourcePath,
		Blocks:        blocks,
		Images:        images,
		Warnings:      warnings,
	}
	article.ContentHash = articleContentHash(article)
	return article, nil
}

type rssDocument struct {
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	Title   string `xml:"title"`
	Link    string `xml:"link"`
	GUID    string `xml:"guid"`
	PubDate string `xml:"pubDate"`
	Content string `xml:"http://purl.org/rss/1.0/modules/content/ encoded"`
}

func selectRSSItem(items []rssItem, canonical string) (rssItem, bool) {
	wanted := comparableURL(canonical)
	for _, item := range items {
		for _, candidate := range []string{item.Link, item.GUID} {
			normalized, err := normalizeURL(candidate)
			if err == nil && comparableURL(normalized) == wanted {
				return item, true
			}
		}
	}
	return rssItem{}, false
}

func normalizeURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("URL must include scheme and host")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	parsed.RawPath = ""
	return parsed.String(), nil
}

func comparableURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	parsed.Host = strings.TrimPrefix(strings.ToLower(parsed.Host), "www.")
	return parsed.String()
}

type rawArticleBlock struct {
	kind        string
	text        string
	level       int
	attribution string
	image       *rawArticleImage
}

type rawArticleImage struct {
	url     string
	alt     string
	caption string
	credit  string
}

func parseArticleHTML(source, baseURL string) ([]ArticleBlock, []ArticleImage, []string, error) {
	doc, err := html.Parse(strings.NewReader(source))
	if err != nil {
		return nil, nil, nil, err
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, nil, nil, err
	}

	var rawBlocks []rawArticleBlock
	var warnings []string
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode {
			tag := strings.ToLower(node.Data)
			switch {
			case isHeadingTag(tag):
				text := normalizeText(nodeText(node, nil))
				if text != "" {
					level, _ := strconv.Atoi(strings.TrimPrefix(tag, "h"))
					rawBlocks = append(rawBlocks, rawArticleBlock{kind: "heading", text: text, level: level})
				}
				return
			case tag == "p" || tag == "li" || tag == "figcaption" || tag == "pre":
				text := normalizeText(nodeText(node, nil))
				if text != "" {
					rawBlocks = append(rawBlocks, rawArticleBlock{kind: "paragraph", text: text})
				}
				forEachDescendantImage(node, func(imageNode *html.Node) {
					rawBlocks = append(rawBlocks, rawArticleBlock{kind: "image", image: parseImageNode(imageNode, "", base)})
				})
				return
			case tag == "blockquote":
				attributionNodes := descendantElements(node, "cite", "footer")
				skip := make(map[*html.Node]bool, len(attributionNodes))
				var attributionParts []string
				for _, attributionNode := range attributionNodes {
					skip[attributionNode] = true
					if text := normalizeText(nodeText(attributionNode, nil)); text != "" {
						attributionParts = append(attributionParts, text)
					}
				}
				text := normalizeText(nodeText(node, skip))
				rawBlocks = append(rawBlocks, rawArticleBlock{
					kind:        "blockquote",
					text:        text,
					attribution: strings.Join(attributionParts, " "),
				})
				if text == "" {
					warnings = append(warnings, fmt.Sprintf("empty RSS blockquote at content block %d", len(rawBlocks)-1))
				}
				forEachDescendantImage(node, func(imageNode *html.Node) {
					rawBlocks = append(rawBlocks, rawArticleBlock{kind: "image", image: parseImageNode(imageNode, "", base)})
				})
				return
			case tag == "figure":
				caption := ""
				if captions := descendantElements(node, "figcaption"); len(captions) > 0 {
					caption = normalizeText(nodeText(captions[0], nil))
				}
				forEachDescendantImage(node, func(imageNode *html.Node) {
					rawBlocks = append(rawBlocks, rawArticleBlock{kind: "image", image: parseImageNode(imageNode, caption, base)})
				})
				return
			case tag == "img":
				rawBlocks = append(rawBlocks, rawArticleBlock{kind: "image", image: parseImageNode(node, "", base)})
				return
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)

	blocks := make([]ArticleBlock, 0, len(rawBlocks))
	images := make([]ArticleImage, 0)
	blockOccurrences := make(map[string]int)
	imageOccurrences := make(map[string]int)
	for _, rawBlock := range rawBlocks {
		blockKey := rawBlock.kind + "\x00" + rawBlock.text + "\x00" + strconv.Itoa(rawBlock.level) + "\x00" + rawBlock.attribution
		if rawBlock.image != nil {
			blockKey += "\x00" + rawBlock.image.url + "\x00" + rawBlock.image.alt + "\x00" + rawBlock.image.caption
		}
		occurrence := blockOccurrences[blockKey]
		blockOccurrences[blockKey]++
		blockID := stableID("block", blockKey, occurrence)
		block := ArticleBlock{
			ID:               blockID,
			Index:            len(blocks),
			Kind:             rawBlock.kind,
			Text:             rawBlock.text,
			Level:            rawBlock.level,
			QuoteAttribution: rawBlock.attribution,
		}
		if rawBlock.image != nil {
			imageKey := rawBlock.image.url + "\x00" + rawBlock.image.alt + "\x00" + rawBlock.image.caption + "\x00" + rawBlock.image.credit
			imageOccurrence := imageOccurrences[imageKey]
			imageOccurrences[imageKey]++
			imageID := stableID("image", imageKey, imageOccurrence)
			block.ImageIDs = []string{imageID}
			images = append(images, ArticleImage{
				ID:      imageID,
				Index:   len(images),
				URL:     rawBlock.image.url,
				Alt:     rawBlock.image.alt,
				Caption: rawBlock.image.caption,
				Credit:  rawBlock.image.credit,
				BlockID: blockID,
			})
		}
		blocks = append(blocks, block)
	}
	setImageAdjacency(blocks, images)
	return blocks, images, warnings, nil
}

func isHeadingTag(tag string) bool {
	return len(tag) == 2 && tag[0] == 'h' && tag[1] >= '1' && tag[1] <= '6'
}

func nodeText(node *html.Node, skip map[*html.Node]bool) string {
	var builder strings.Builder
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if skip[current] {
			return
		}
		if current.Type == html.TextNode {
			builder.WriteString(current.Data)
			builder.WriteByte(' ')
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return builder.String()
}

func descendantElements(node *html.Node, tags ...string) []*html.Node {
	wanted := make(map[string]bool, len(tags))
	for _, tag := range tags {
		wanted[tag] = true
	}
	var matches []*html.Node
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current != node && current.Type == html.ElementNode && wanted[strings.ToLower(current.Data)] {
			matches = append(matches, current)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return matches
}

func forEachDescendantImage(node *html.Node, visit func(*html.Node)) {
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current != node && current.Type == html.ElementNode && strings.EqualFold(current.Data, "img") {
			visit(current)
			return
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
}

func parseImageNode(node *html.Node, caption string, base *url.URL) *rawArticleImage {
	src := firstAttribute(node, "src", "data-src", "data-lazy-src")
	resolved := src
	if parsed, err := url.Parse(src); err == nil && src != "" {
		resolved = base.ResolveReference(parsed).String()
	}
	return &rawArticleImage{
		url:     resolved,
		alt:     normalizeText(firstAttribute(node, "alt")),
		caption: caption,
		credit:  normalizeText(firstAttribute(node, "data-credit")),
	}
}

func firstAttribute(node *html.Node, names ...string) string {
	for _, name := range names {
		for _, attribute := range node.Attr {
			if strings.EqualFold(attribute.Key, name) && strings.TrimSpace(attribute.Val) != "" {
				return strings.TrimSpace(attribute.Val)
			}
		}
	}
	return ""
}

func normalizeText(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func stableID(prefix, key string, occurrence int) string {
	digest := sha256.Sum256([]byte(key + "\x00" + strconv.Itoa(occurrence)))
	return prefix + "-" + hex.EncodeToString(digest[:8])
}

func setImageAdjacency(blocks []ArticleBlock, images []ArticleImage) {
	blockPosition := make(map[string]int, len(blocks))
	for i, block := range blocks {
		blockPosition[block.ID] = i
	}
	for i := range images {
		position, ok := blockPosition[images[i].BlockID]
		if !ok {
			continue
		}
		for before := position - 1; before >= 0; before-- {
			if blocks[before].Kind != "image" && blocks[before].Text != "" {
				images[i].AdjacentTextBlockIDs = append(images[i].AdjacentTextBlockIDs, blocks[before].ID)
				break
			}
		}
		for after := position + 1; after < len(blocks); after++ {
			if blocks[after].Kind != "image" && blocks[after].Text != "" {
				images[i].AdjacentTextBlockIDs = append(images[i].AdjacentTextBlockIDs, blocks[after].ID)
				break
			}
		}
	}
}

func articleContentHash(article Article) string {
	canonical := struct {
		CanonicalURL string         `json:"canonical_url"`
		Title        string         `json:"title"`
		PublishedAt  string         `json:"published_at"`
		Blocks       []ArticleBlock `json:"blocks"`
		Images       []struct {
			ID                   string   `json:"id"`
			URL                  string   `json:"url"`
			Alt                  string   `json:"alt"`
			Caption              string   `json:"caption"`
			Credit               string   `json:"credit"`
			BlockID              string   `json:"block_id"`
			AdjacentTextBlockIDs []string `json:"adjacent_text_block_ids"`
		} `json:"images"`
	}{
		CanonicalURL: article.CanonicalURL,
		Title:        article.Title,
		PublishedAt:  article.PublishedAt,
		Blocks:       article.Blocks,
	}
	for _, image := range article.Images {
		canonical.Images = append(canonical.Images, struct {
			ID                   string   `json:"id"`
			URL                  string   `json:"url"`
			Alt                  string   `json:"alt"`
			Caption              string   `json:"caption"`
			Credit               string   `json:"credit"`
			BlockID              string   `json:"block_id"`
			AdjacentTextBlockIDs []string `json:"adjacent_text_block_ids"`
		}{
			ID:                   image.ID,
			URL:                  image.URL,
			Alt:                  image.Alt,
			Caption:              image.Caption,
			Credit:               image.Credit,
			BlockID:              image.BlockID,
			AdjacentTextBlockIDs: image.AdjacentTextBlockIDs,
		})
	}
	encoded, _ := json.Marshal(canonical)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func normalizePublishedAt(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	formats := []string{time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822, time.RFC3339}
	for _, format := range formats {
		if parsed, err := time.Parse(format, raw); err == nil {
			return parsed.UTC().Format(time.RFC3339)
		}
	}
	return raw
}

func fetchBytes(ctx context.Context, client *http.Client, sourceURL string, maxBytes int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/rss+xml, application/xml;q=0.9, text/xml;q=0.8")
	request.Header.Set("User-Agent", "video-edit/1")
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected HTTP status %s", response.Status)
	}
	return readLimited(response.Body, maxBytes)
}

func readLimited(reader io.Reader, maxBytes int64) ([]byte, error) {
	limited := io.LimitReader(reader, maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("response exceeds %d bytes", maxBytes)
	}
	return data, nil
}

func downloadArticleImage(ctx context.Context, client *http.Client, imageURL, directory string, maxBytes int64) (string, error) {
	parsed, err := url.Parse(imageURL)
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("unsupported image URL scheme %q", parsed.Scheme)
	}
	digest := sha256.Sum256([]byte(imageURL))
	baseName := "image-" + hex.EncodeToString(digest[:10])
	extension := normalizedImageExtension(filepath.Ext(parsed.Path))
	if extension != "" {
		path := filepath.Join(directory, baseName+extension)
		if fileIsNonEmpty(path) {
			return path, nil
		}
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Accept", "image/*")
	request.Header.Set("User-Agent", "video-edit/1")
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("unexpected HTTP status %s", response.Status)
	}
	if extension == "" {
		extension = extensionForMediaType(response.Header.Get("Content-Type"))
	}
	if extension == "" {
		extension = ".img"
	}
	destination := filepath.Join(directory, baseName+extension)
	if fileIsNonEmpty(destination) {
		return destination, nil
	}

	contents, err := readLimited(response.Body, maxBytes)
	if err != nil {
		return "", err
	}
	temporary, err := os.CreateTemp(directory, ".image-download-*")
	if err != nil {
		return "", err
	}
	temporaryPath := temporary.Name()
	keepTemporary := true
	defer func() {
		if keepTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if _, err := temporary.Write(contents); err != nil {
		_ = temporary.Close()
		return "", err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return "", err
	}
	if err := temporary.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return "", err
	}
	keepTemporary = false
	return destination, nil
}

func normalizedImageExtension(extension string) string {
	extension = strings.ToLower(extension)
	switch extension {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".avif", ".svg":
		return extension
	default:
		return ""
	}
}

func extensionForMediaType(contentType string) string {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return ""
	}
	switch mediaType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/avif":
		return ".avif"
	case "image/svg+xml":
		return ".svg"
	default:
		return ""
	}
}

func fileIsNonEmpty(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}

func writeBytesAtomic(path string, contents []byte, permissions os.FileMode) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".snapshot-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	keepTemporary := true
	defer func() {
		if keepTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if _, err := temporary.Write(contents); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Chmod(permissions); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	keepTemporary = false
	return nil
}
