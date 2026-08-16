package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	stdhtml "html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	xhtml "golang.org/x/net/html"
)

const (
	schemaVersion = 2
	maxAssetSize  = 50 << 20
)

type prepareOptions struct {
	FeedURL      string
	Article      string
	RenderedPath string
	OutDir       string
}

type renderedSnapshot struct {
	URL                    string              `json:"url"`
	Title                  string              `json:"title"`
	Subtitle               string              `json:"subtitle"`
	PublishedAt            string              `json:"published_at"`
	CoverImageURL          string              `json:"cover_image_url"`
	ParagraphCount         *int                `json:"paragraph_count"`
	HeadingCount           *int                `json:"heading_count"`
	BlockquoteCount        *int                `json:"blockquote_count"`
	BodyImages             []renderedImage     `json:"body_images"`
	RenderedOnlyParagraphs []renderedParagraph `json:"rendered_only_paragraphs,omitempty"`
}

type renderedImage struct {
	Src     string `json:"src"`
	Alt     string `json:"alt"`
	Caption string `json:"caption"`
}

type renderedParagraph struct {
	Index int    `json:"index"`
	Text  string `json:"text"`
}

type rssDocument struct {
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string       `xml:"title"`
	Link        string       `xml:"link"`
	GUID        string       `xml:"guid"`
	Description string       `xml:"description"`
	Content     string       `xml:"encoded"`
	PubDate     string       `xml:"pubDate"`
	Enclosure   rssEnclosure `xml:"enclosure"`
}

type rssEnclosure struct {
	URL  string `xml:"url,attr"`
	Type string `xml:"type,attr"`
}

type article struct {
	FeedURL     string
	URL         string
	Title       string
	Subtitle    string
	PublishedAt string
	Cover       imageData
	Elements    []element
}

type element struct {
	Kind  string
	HTML  string
	Text  string
	Image *imageData
}

type imageData struct {
	SourceURL string
	Alt       string
	Caption   string
	LocalPath string
}

type countManifest struct {
	Paragraphs  int `json:"paragraphs"`
	Headings    int `json:"headings"`
	BodyImages  int `json:"body_images"`
	Blockquotes int `json:"populated_blockquotes"`
}

type expectedManifest struct {
	Paragraphs  *int `json:"paragraphs,omitempty"`
	Headings    *int `json:"headings,omitempty"`
	BodyImages  *int `json:"body_images,omitempty"`
	Blockquotes *int `json:"populated_blockquotes,omitempty"`
}

type sourceManifest struct {
	FeedURL     string `json:"feed_url"`
	ArticleURL  string `json:"article_url"`
	Title       string `json:"title"`
	Subtitle    string `json:"subtitle"`
	PublishedAt string `json:"published_at"`
}

type imageManifest struct {
	Index        int    `json:"index"`
	ElementIndex int    `json:"element_index"`
	SourceURL    string `json:"source_url"`
	LocalPath    string `json:"local_path"`
	Alt          string `json:"alt,omitempty"`
	Caption      string `json:"caption,omitempty"`
}

type coverManifest struct {
	SourceURL string `json:"source_url"`
	LocalPath string `json:"local_path"`
	Alt       string `json:"alt,omitempty"`
}

type artifactManifest struct {
	LinkedInSubstackHTML string `json:"linkedin_substack_html"`
	SubstackHTML         string `json:"substack_html"`
	MediumHTML           string `json:"medium_html"`
	PlainText            string `json:"plain_text"`
	Manifest             string `json:"manifest"`
}

type manifest struct {
	SchemaVersion int              `json:"schema_version"`
	RunID         string           `json:"run_id"`
	RunDirectory  string           `json:"run_directory"`
	Source        sourceManifest   `json:"source"`
	Counts        countManifest    `json:"counts"`
	Expected      expectedManifest `json:"rendered_expected"`
	Cover         coverManifest    `json:"cover"`
	Images        []imageManifest  `json:"images"`
	Artifacts     artifactManifest `json:"artifacts"`
}

type prepareResult struct {
	RunDirectory string `json:"run_directory"`
	Manifest     string `json:"manifest"`
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "crosspost-prep:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) == 0 || args[0] != "prepare" {
		return errors.New("usage: crosspost-prep prepare --feed-url <url> --article latest|<url> --rendered <snapshot.json> [--out <parent-dir>]")
	}

	fs := flag.NewFlagSet("prepare", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	opts := prepareOptions{}
	fs.StringVar(&opts.FeedURL, "feed-url", "", "Beehiiv RSS URL")
	fs.StringVar(&opts.Article, "article", "", "latest or an article URL")
	fs.StringVar(&opts.RenderedPath, "rendered", "", "rendered article snapshot JSON")
	fs.StringVar(&opts.OutDir, "out", "", "parent directory for the unique run directory")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	client := &http.Client{Timeout: 45 * time.Second}
	m, err := prepare(ctx, client, opts)
	if err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(prepareResult{RunDirectory: m.RunDirectory, Manifest: m.Artifacts.Manifest})
}

func prepare(ctx context.Context, client *http.Client, opts prepareOptions) (*manifest, error) {
	if err := validateOptions(opts); err != nil {
		return nil, err
	}

	rendered, err := readRenderedSnapshot(opts.RenderedPath)
	if err != nil {
		return nil, err
	}
	feed, err := fetchFeed(ctx, client, opts.FeedURL)
	if err != nil {
		return nil, err
	}
	item, err := selectItem(feed.Channel.Items, opts.Article)
	if err != nil {
		return nil, err
	}
	articleURL := itemURL(item)
	if opts.Article != "latest" {
		articleURL = opts.Article
	}
	if rendered.URL != "" {
		if !sameArticle(articleURL, rendered.URL) {
			return nil, fmt.Errorf("rendered snapshot URL %q does not match selected article %q", rendered.URL, articleURL)
		}
		articleURL = rendered.URL
	}
	baseURL, err := url.Parse(articleURL)
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" {
		return nil, fmt.Errorf("invalid selected article URL %q", articleURL)
	}

	rawHTML := item.Content
	if strings.TrimSpace(rawHTML) == "" {
		rawHTML = item.Description
	}
	if strings.TrimSpace(rawHTML) == "" {
		return nil, errors.New("selected RSS item has no article HTML")
	}
	elements, err := parseArticleHTML(rawHTML, baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse article HTML: %w", err)
	}

	a := article{
		FeedURL:     opts.FeedURL,
		URL:         articleURL,
		Title:       cleanText(item.Title),
		PublishedAt: normalizeDate(item.PubDate),
		Cover:       imageData{SourceURL: cleanURL(item.Enclosure.URL, baseURL)},
		Elements:    elements,
	}
	if err := mergeRendered(&a, rendered); err != nil {
		return nil, err
	}
	counts := countElements(a.Elements)
	expected := expectedFromRendered(rendered)
	if err := validateCounts(counts, expected); err != nil {
		return nil, err
	}
	if a.Title == "" {
		return nil, errors.New("source title is empty")
	}
	if a.Cover.SourceURL == "" {
		return nil, errors.New("rendered snapshot and RSS enclosure do not provide a cover image")
	}

	parent := opts.OutDir
	if parent == "" {
		parent = os.TempDir()
	}
	parent, err = filepath.Abs(parent)
	if err != nil {
		return nil, fmt.Errorf("resolve output parent: %w", err)
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, fmt.Errorf("create output parent: %w", err)
	}
	runDir, err := os.MkdirTemp(parent, "crosspost-"+slugForPath(a.URL)+"-")
	if err != nil {
		return nil, fmt.Errorf("create unique run directory: %w", err)
	}

	if err := downloadAssets(ctx, client, runDir, &a); err != nil {
		return nil, err
	}

	linkedinPath := filepath.Join(runDir, "article-linkedin-substack.html")
	substackPath := filepath.Join(runDir, "article-substack.html")
	mediumPath := filepath.Join(runDir, "article-medium.html")
	plainPath := filepath.Join(runDir, "article.txt")
	manifestPath := filepath.Join(runDir, "manifest.json")
	if err := writeAtomic(linkedinPath, []byte(renderLinkedInSubstack(a.Elements))); err != nil {
		return nil, err
	}
	if err := writeAtomic(substackPath, []byte(renderSubstack(a.Elements))); err != nil {
		return nil, err
	}
	if err := writeAtomic(mediumPath, []byte(renderMedium(a.Cover, a.Elements))); err != nil {
		return nil, err
	}
	if err := writeAtomic(plainPath, []byte(renderPlain(a))); err != nil {
		return nil, err
	}

	m := buildManifest(a, counts, expected, runDir, linkedinPath, substackPath, mediumPath, plainPath, manifestPath)
	manifestBytes, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode manifest: %w", err)
	}
	manifestBytes = append(manifestBytes, '\n')
	if err := writeAtomic(manifestPath, manifestBytes); err != nil {
		return nil, err
	}
	return m, nil
}

func validateOptions(opts prepareOptions) error {
	if strings.TrimSpace(opts.FeedURL) == "" {
		return errors.New("--feed-url is required")
	}
	u, err := url.Parse(opts.FeedURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("invalid --feed-url %q", opts.FeedURL)
	}
	if opts.Article == "" {
		return errors.New("--article is required")
	}
	if opts.Article != "latest" {
		u, err := url.Parse(opts.Article)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("invalid --article %q", opts.Article)
		}
	}
	if strings.TrimSpace(opts.RenderedPath) == "" {
		return errors.New("--rendered is required")
	}
	return nil
}

func readRenderedSnapshot(path string) (renderedSnapshot, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return renderedSnapshot{}, fmt.Errorf("read rendered snapshot: %w", err)
	}
	var snapshot renderedSnapshot
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&snapshot); err != nil {
		return renderedSnapshot{}, fmt.Errorf("decode rendered snapshot: %w", err)
	}
	for i, image := range snapshot.BodyImages {
		if strings.TrimSpace(image.Src) == "" {
			return renderedSnapshot{}, fmt.Errorf("rendered body_images[%d].src is empty", i)
		}
	}
	return snapshot, nil
}

func fetchFeed(ctx context.Context, client *http.Client, feedURL string) (rssDocument, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return rssDocument{}, fmt.Errorf("build feed request: %w", err)
	}
	req.Header.Set("User-Agent", "crosspost-prep/1")
	resp, err := client.Do(req)
	if err != nil {
		return rssDocument{}, fmt.Errorf("fetch feed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return rssDocument{}, fmt.Errorf("fetch feed: HTTP %d", resp.StatusCode)
	}
	limited := io.LimitReader(resp.Body, 20<<20)
	var feed rssDocument
	dec := xml.NewDecoder(limited)
	if err := dec.Decode(&feed); err != nil {
		return rssDocument{}, fmt.Errorf("decode RSS: %w", err)
	}
	if len(feed.Channel.Items) == 0 {
		return rssDocument{}, errors.New("RSS contains no items")
	}
	return feed, nil
}

func selectItem(items []rssItem, requested string) (rssItem, error) {
	if requested != "latest" {
		for _, item := range items {
			if sameArticle(itemURL(item), requested) {
				return item, nil
			}
		}
		return rssItem{}, fmt.Errorf("article %q not found in RSS", requested)
	}
	latest := items[0]
	latestTime, latestOK := parseFeedTime(latest.PubDate)
	for _, item := range items[1:] {
		t, ok := parseFeedTime(item.PubDate)
		if ok && (!latestOK || t.After(latestTime)) {
			latest, latestTime, latestOK = item, t, true
		}
	}
	return latest, nil
}

func itemURL(item rssItem) string {
	if strings.TrimSpace(item.Link) != "" {
		return strings.TrimSpace(item.Link)
	}
	return strings.TrimSpace(item.GUID)
}

func parseFeedTime(value string) (time.Time, bool) {
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822, time.RFC3339} {
		if t, err := time.Parse(layout, strings.TrimSpace(value)); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func normalizeDate(value string) string {
	if t, ok := parseFeedTime(value); ok {
		return t.Format("2006-01-02")
	}
	value = strings.TrimSpace(value)
	if len(value) >= 10 {
		if _, err := time.Parse("2006-01-02", value[:10]); err == nil {
			return value[:10]
		}
	}
	return value
}

func sameArticle(a, b string) bool {
	ua, ea := url.Parse(strings.TrimSpace(a))
	ub, eb := url.Parse(strings.TrimSpace(b))
	if ea != nil || eb != nil {
		return strings.TrimRight(a, "/") == strings.TrimRight(b, "/")
	}
	pa := strings.TrimRight(strings.ToLower(ua.EscapedPath()), "/")
	pb := strings.TrimRight(strings.ToLower(ub.EscapedPath()), "/")
	return pa != "" && pa == pb
}

func parseArticleHTML(raw string, baseURL *url.URL) ([]element, error) {
	nodes, err := xhtml.ParseFragment(strings.NewReader(raw), nil)
	if err != nil {
		return nil, err
	}
	var elements []element
	for _, node := range nodes {
		collectElements(node, baseURL, &elements)
	}
	if len(elements) == 0 {
		return nil, errors.New("no semantic article elements found")
	}
	return elements, nil
}

func collectElements(node *xhtml.Node, baseURL *url.URL, out *[]element) {
	if node.Type != xhtml.ElementNode {
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			collectElements(child, baseURL, out)
		}
		return
	}

	tag := strings.ToLower(node.Data)
	switch tag {
	case "figure":
		if imgNode := firstDescendant(node, "img"); imgNode != nil {
			img := imageFromNode(imgNode, baseURL)
			if capNode := firstDescendant(node, "figcaption"); capNode != nil {
				img.Caption = cleanText(textContent(capNode))
			}
			if img.SourceURL != "" {
				*out = append(*out, element{Kind: "image", Image: &img})
			}
			return
		}
	case "img":
		img := imageFromNode(node, baseURL)
		if img.SourceURL != "" {
			*out = append(*out, element{Kind: "image", Image: &img})
		}
		return
	case "p":
		if imgs := descendants(node, "img"); len(imgs) > 0 {
			text := cleanText(textContent(node))
			if text != "" && !isBoilerplate(text) {
				*out = append(*out, element{Kind: "paragraph", HTML: renderBlock("p", node, baseURL), Text: text})
			}
			for _, imgNode := range imgs {
				img := imageFromNode(imgNode, baseURL)
				if img.SourceURL != "" {
					*out = append(*out, element{Kind: "image", Image: &img})
				}
			}
			return
		}
		text := cleanText(textContent(node))
		if text != "" && !isBoilerplate(text) {
			*out = append(*out, element{Kind: "paragraph", HTML: renderBlock("p", node, baseURL), Text: text})
		}
		return
	case "h1", "h2", "h3", "h4", "h5", "h6":
		text := cleanText(textContent(node))
		if text != "" && !isBoilerplate(text) {
			*out = append(*out, element{Kind: "heading", HTML: renderBlock("h2", node, baseURL), Text: text})
		}
		return
	case "blockquote":
		text := cleanText(textContent(node))
		if text != "" && !isBoilerplate(text) {
			*out = append(*out, element{Kind: "blockquote", HTML: renderBlock("blockquote", node, baseURL), Text: text})
		}
		return
	case "ul", "ol":
		text := cleanText(textContent(node))
		if text != "" && !isBoilerplate(text) {
			*out = append(*out, element{Kind: "list", HTML: renderList(tag, node, baseURL), Text: text})
		}
		return
	case "pre":
		text := strings.TrimSpace(textContent(node))
		if text != "" {
			*out = append(*out, element{Kind: "pre", HTML: "<pre><code>" + stdhtml.EscapeString(text) + "</code></pre>", Text: text})
		}
		return
	case "hr":
		*out = append(*out, element{Kind: "separator", HTML: "<hr>"})
		return
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		collectElements(child, baseURL, out)
	}
}

func renderBlock(tag string, node *xhtml.Node, baseURL *url.URL) string {
	var b strings.Builder
	b.WriteByte('<')
	b.WriteString(tag)
	b.WriteByte('>')
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		renderInline(&b, child, baseURL)
	}
	b.WriteString("</")
	b.WriteString(tag)
	b.WriteByte('>')
	return b.String()
}

func renderList(tag string, node *xhtml.Node, baseURL *url.URL) string {
	var b strings.Builder
	b.WriteByte('<')
	b.WriteString(tag)
	b.WriteByte('>')
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == xhtml.ElementNode && strings.EqualFold(child.Data, "li") {
				b.WriteString("<li>")
				for nested := child.FirstChild; nested != nil; nested = nested.NextSibling {
					renderInline(&b, nested, baseURL)
				}
				b.WriteString("</li>")
			} else {
				walk(child)
			}
		}
	}
	walk(node)
	b.WriteString("</")
	b.WriteString(tag)
	b.WriteByte('>')
	return b.String()
}

func renderInline(b *strings.Builder, node *xhtml.Node, baseURL *url.URL) {
	switch node.Type {
	case xhtml.TextNode:
		b.WriteString(stdhtml.EscapeString(node.Data))
	case xhtml.ElementNode:
		tag := strings.ToLower(node.Data)
		switch tag {
		case "strong", "b":
			b.WriteString("<strong>")
			renderInlineChildren(b, node, baseURL)
			b.WriteString("</strong>")
		case "em", "i":
			b.WriteString("<em>")
			renderInlineChildren(b, node, baseURL)
			b.WriteString("</em>")
		case "code":
			b.WriteString("<code>")
			renderInlineChildren(b, node, baseURL)
			b.WriteString("</code>")
		case "a":
			href := cleanURL(attr(node, "href"), baseURL)
			if href == "" {
				renderInlineChildren(b, node, baseURL)
				return
			}
			b.WriteString(`<a href="`)
			b.WriteString(stdhtml.EscapeString(href))
			b.WriteString(`">`)
			renderInlineChildren(b, node, baseURL)
			b.WriteString("</a>")
		case "br":
			b.WriteString("<br>")
		case "span", "small", "mark", "u", "sup", "sub":
			renderInlineChildren(b, node, baseURL)
		default:
			renderInlineChildren(b, node, baseURL)
		}
	}
}

func renderInlineChildren(b *strings.Builder, node *xhtml.Node, baseURL *url.URL) {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		renderInline(b, child, baseURL)
	}
}

func imageFromNode(node *xhtml.Node, baseURL *url.URL) imageData {
	src := attr(node, "src")
	if src == "" {
		src = attr(node, "data-src")
	}
	return imageData{
		SourceURL: cleanURL(src, baseURL),
		Alt:       cleanText(attr(node, "alt")),
	}
}

func firstDescendant(node *xhtml.Node, tag string) *xhtml.Node {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == xhtml.ElementNode && strings.EqualFold(child.Data, tag) {
			return child
		}
		if found := firstDescendant(child, tag); found != nil {
			return found
		}
	}
	return nil
}

func descendants(node *xhtml.Node, tag string) []*xhtml.Node {
	var found []*xhtml.Node
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == xhtml.ElementNode && strings.EqualFold(child.Data, tag) {
				found = append(found, child)
			}
			walk(child)
		}
	}
	walk(node)
	return found
}

func attr(node *xhtml.Node, key string) string {
	for _, a := range node.Attr {
		if strings.EqualFold(a.Key, key) {
			return strings.TrimSpace(a.Val)
		}
	}
	return ""
}

func textContent(node *xhtml.Node) string {
	var b strings.Builder
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.TextNode {
			b.WriteString(n.Data)
			b.WriteByte(' ')
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return b.String()
}

func cleanText(value string) string {
	value = stdhtml.UnescapeString(value)
	return strings.Join(strings.Fields(value), " ")
}

func isBoilerplate(value string) bool {
	value = strings.ToLower(cleanText(value))
	if len(value) > 300 {
		return false
	}
	markers := []string{
		"view in browser",
		"manage preferences",
		"manage your preferences",
		"unsubscribe",
		"powered by beehiiv",
		"download the beehiiv app",
		"you're receiving this email",
		"you are receiving this email",
		"no longer want to receive",
	}
	for _, marker := range markers {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func cleanURL(value string, baseURL *url.URL) string {
	value = strings.TrimSpace(stdhtml.UnescapeString(value))
	if value == "" {
		return ""
	}
	u, err := url.Parse(value)
	if err != nil {
		return ""
	}
	if baseURL != nil {
		u = baseURL.ResolveReference(u)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	q := u.Query()
	for key := range q {
		lower := strings.ToLower(key)
		if strings.HasPrefix(lower, "utm_") || trackingQueryKey(lower) {
			q.Del(key)
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func trackingQueryKey(key string) bool {
	switch key {
	case "fbclid", "gclid", "dclid", "msclkid", "mc_cid", "mc_eid", "ref", "source", "subscriber_id", "publication_id", "post_id":
		return true
	default:
		return false
	}
}

func mergeRendered(a *article, rendered renderedSnapshot) error {
	if rendered.Title != "" {
		if a.Title != "" && cleanText(a.Title) != cleanText(rendered.Title) {
			return fmt.Errorf("rendered title %q does not match RSS title %q", rendered.Title, a.Title)
		}
		a.Title = cleanText(rendered.Title)
	}
	if rendered.Subtitle != "" {
		a.Subtitle = cleanText(rendered.Subtitle)
	}
	if rendered.PublishedAt != "" {
		date := normalizeDate(rendered.PublishedAt)
		if a.PublishedAt != "" && date != "" && a.PublishedAt != date {
			return fmt.Errorf("rendered publication date %q does not match RSS date %q", date, a.PublishedAt)
		}
		a.PublishedAt = date
	}
	baseURL, _ := url.Parse(a.URL)
	if rendered.CoverImageURL != "" {
		a.Cover.SourceURL = cleanURL(rendered.CoverImageURL, baseURL)
	}
	if err := mergeRenderedParagraphs(a, rendered.RenderedOnlyParagraphs); err != nil {
		return err
	}
	images := articleImages(a.Elements)
	if len(rendered.BodyImages) > 0 {
		if len(images) != len(rendered.BodyImages) {
			return fmt.Errorf("rendered body image count %d does not match RSS body image count %d", len(rendered.BodyImages), len(images))
		}
		for i, renderedImage := range rendered.BodyImages {
			images[i].SourceURL = cleanURL(renderedImage.Src, baseURL)
			if renderedImage.Alt != "" {
				images[i].Alt = cleanText(renderedImage.Alt)
			}
			if renderedImage.Caption != "" {
				images[i].Caption = cleanText(renderedImage.Caption)
			}
		}
	}
	return nil
}

func mergeRenderedParagraphs(a *article, additions []renderedParagraph) error {
	if len(additions) == 0 {
		return nil
	}
	additions = append([]renderedParagraph(nil), additions...)
	sort.Slice(additions, func(i, j int) bool { return additions[i].Index < additions[j].Index })
	for i := range additions {
		additions[i].Text = cleanText(additions[i].Text)
		if additions[i].Index < 0 {
			return fmt.Errorf("rendered_only_paragraphs[%d].index must be non-negative", i)
		}
		if additions[i].Text == "" {
			return fmt.Errorf("rendered_only_paragraphs[%d].text is empty", i)
		}
		if i > 0 && additions[i].Index == additions[i-1].Index {
			return fmt.Errorf("rendered-only paragraph index %d is duplicated", additions[i].Index)
		}
	}

	merged := make([]element, 0, len(a.Elements)+len(additions))
	paragraphIndex := 0
	additionIndex := 0
	appendAdditions := func() {
		for additionIndex < len(additions) && additions[additionIndex].Index == paragraphIndex {
			text := additions[additionIndex].Text
			merged = append(merged, element{Kind: "paragraph", HTML: "<p>" + stdhtml.EscapeString(text) + "</p>", Text: text})
			paragraphIndex++
			additionIndex++
		}
	}
	for _, el := range a.Elements {
		if el.Kind == "paragraph" {
			appendAdditions()
		}
		merged = append(merged, el)
		if el.Kind == "paragraph" {
			paragraphIndex++
		}
	}
	appendAdditions()
	if additionIndex != len(additions) {
		return fmt.Errorf("rendered-only paragraph index %d is beyond the merged paragraph count %d", additions[additionIndex].Index, paragraphIndex)
	}
	a.Elements = merged
	return nil
}

func articleImages(elements []element) []*imageData {
	images := make([]*imageData, 0)
	for i := range elements {
		if elements[i].Kind == "image" && elements[i].Image != nil {
			images = append(images, elements[i].Image)
		}
	}
	return images
}

func countElements(elements []element) countManifest {
	var counts countManifest
	for _, el := range elements {
		switch el.Kind {
		case "paragraph":
			counts.Paragraphs++
		case "heading":
			counts.Headings++
		case "image":
			counts.BodyImages++
		case "blockquote":
			counts.Blockquotes++
		}
	}
	return counts
}

func expectedFromRendered(rendered renderedSnapshot) expectedManifest {
	var imageCount *int
	if rendered.BodyImages != nil {
		value := len(rendered.BodyImages)
		imageCount = &value
	}
	return expectedManifest{
		Paragraphs:  rendered.ParagraphCount,
		Headings:    rendered.HeadingCount,
		BodyImages:  imageCount,
		Blockquotes: rendered.BlockquoteCount,
	}
}

func validateCounts(actual countManifest, expected expectedManifest) error {
	type pair struct {
		name     string
		actual   int
		expected *int
	}
	pairs := []pair{
		{"paragraphs", actual.Paragraphs, expected.Paragraphs},
		{"headings", actual.Headings, expected.Headings},
		{"body images", actual.BodyImages, expected.BodyImages},
		{"populated blockquotes", actual.Blockquotes, expected.Blockquotes},
	}
	var mismatches []string
	for _, pair := range pairs {
		if pair.expected != nil && pair.actual != *pair.expected {
			mismatches = append(mismatches, fmt.Sprintf("%s: RSS=%d rendered=%d", pair.name, pair.actual, *pair.expected))
		}
	}
	if len(mismatches) > 0 {
		return fmt.Errorf("source validation failed (%s)", strings.Join(mismatches, "; "))
	}
	return nil
}

func downloadAssets(ctx context.Context, client *http.Client, runDir string, a *article) error {
	imageDir := filepath.Join(runDir, "images")
	if err := os.Mkdir(imageDir, 0o755); err != nil {
		return fmt.Errorf("create image directory: %w", err)
	}
	path, err := downloadAsset(ctx, client, a.Cover.SourceURL, imageDir, "cover")
	if err != nil {
		return fmt.Errorf("download cover: %w", err)
	}
	a.Cover.LocalPath = path
	for i, image := range articleImages(a.Elements) {
		path, err := downloadAsset(ctx, client, image.SourceURL, imageDir, fmt.Sprintf("body-%02d", i+1))
		if err != nil {
			return fmt.Errorf("download body image %d: %w", i+1, err)
		}
		image.LocalPath = path
	}
	return nil
}

func downloadAsset(ctx context.Context, client *http.Client, sourceURL, dir, stem string) (string, error) {
	if sourceURL == "" {
		return "", errors.New("source URL is empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "crosspost-prep/1")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d for %s", resp.StatusCode, sourceURL)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAssetSize+1))
	if err != nil {
		return "", err
	}
	if len(body) == 0 {
		return "", errors.New("empty response")
	}
	if len(body) > maxAssetSize {
		return "", fmt.Errorf("asset exceeds %d bytes", maxAssetSize)
	}
	ext := extensionFor(resp.Header.Get("Content-Type"), sourceURL)
	path := filepath.Join(dir, stem+ext)
	if err := writeAtomic(path, body); err != nil {
		return "", err
	}
	return path, nil
}

func extensionFor(contentType, sourceURL string) string {
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch contentType {
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
	}
	u, _ := url.Parse(sourceURL)
	ext := strings.ToLower(filepath.Ext(u.Path))
	if matched, _ := regexp.MatchString(`^\.(jpg|jpeg|png|gif|webp|avif)$`, ext); matched {
		if ext == ".jpeg" {
			return ".jpg"
		}
		return ext
	}
	return ".img"
}

func renderLinkedInSubstack(elements []element) string {
	var b strings.Builder
	imageIndex := 0
	for _, el := range elements {
		if el.Kind == "image" {
			imageIndex++
			fmt.Fprintf(&b, `<p data-crosspost-image="%02d"><!-- CROSSPOST_IMAGE_%02d --></p>`, imageIndex, imageIndex)
			b.WriteByte('\n')
			continue
		}
		b.WriteString(el.HTML)
		b.WriteByte('\n')
	}
	return b.String()
}

func renderSubstack(elements []element) string {
	var b strings.Builder
	for _, el := range elements {
		if el.Kind != "image" {
			b.WriteString(el.HTML)
			b.WriteByte('\n')
			continue
		}
		fmt.Fprintf(&b, `<figure><img src="%s" alt="%s">`, stdhtml.EscapeString(el.Image.SourceURL), stdhtml.EscapeString(el.Image.Alt))
		if el.Image.Caption != "" {
			fmt.Fprintf(&b, "<figcaption>%s</figcaption>", stdhtml.EscapeString(el.Image.Caption))
		}
		b.WriteString("</figure>\n")
	}
	return b.String()
}

func renderMedium(cover imageData, elements []element) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<img src="%s" alt="%s">`, stdhtml.EscapeString(cover.SourceURL), stdhtml.EscapeString(cover.Alt))
	b.WriteByte('\n')
	for _, el := range elements {
		if el.Kind != "image" {
			b.WriteString(el.HTML)
			b.WriteByte('\n')
			continue
		}
		fmt.Fprintf(&b, `<img src="%s" alt="%s">`, stdhtml.EscapeString(el.Image.SourceURL), stdhtml.EscapeString(el.Image.Alt))
		b.WriteByte('\n')
		if el.Image.Caption != "" {
			fmt.Fprintf(&b, "<p><em>%s</em></p>\n", stdhtml.EscapeString(el.Image.Caption))
		}
	}
	return b.String()
}

func renderPlain(a article) string {
	var b strings.Builder
	b.WriteString(a.Title)
	b.WriteByte('\n')
	if a.Subtitle != "" {
		b.WriteString(a.Subtitle)
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	for _, el := range a.Elements {
		if el.Kind == "image" {
			continue
		}
		if el.Text != "" {
			b.WriteString(el.Text)
			b.WriteString("\n\n")
		}
	}
	return b.String()
}

func buildManifest(a article, counts countManifest, expected expectedManifest, runDir, linkedinPath, substackPath, mediumPath, plainPath, manifestPath string) *manifest {
	images := make([]imageManifest, 0, counts.BodyImages)
	imageIndex := 0
	for elementIndex, el := range a.Elements {
		if el.Kind != "image" || el.Image == nil {
			continue
		}
		imageIndex++
		images = append(images, imageManifest{
			Index:        imageIndex,
			ElementIndex: elementIndex,
			SourceURL:    el.Image.SourceURL,
			LocalPath:    el.Image.LocalPath,
			Alt:          el.Image.Alt,
			Caption:      el.Image.Caption,
		})
	}
	return &manifest{
		SchemaVersion: schemaVersion,
		RunID:         filepath.Base(runDir),
		RunDirectory:  runDir,
		Source: sourceManifest{
			FeedURL:     a.FeedURL,
			ArticleURL:  a.URL,
			Title:       a.Title,
			Subtitle:    a.Subtitle,
			PublishedAt: a.PublishedAt,
		},
		Counts:   counts,
		Expected: expected,
		Cover: coverManifest{
			SourceURL: a.Cover.SourceURL,
			LocalPath: a.Cover.LocalPath,
			Alt:       a.Cover.Alt,
		},
		Images: images,
		Artifacts: artifactManifest{
			LinkedInSubstackHTML: linkedinPath,
			SubstackHTML:         substackPath,
			MediumHTML:           mediumPath,
			PlainText:            plainPath,
			Manifest:             manifestPath,
		},
	}
}

func writeAtomic(path string, body []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".crosspost-write-")
	if err != nil {
		return fmt.Errorf("create temporary file for %s: %w", path, err)
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		_ = tmp.Close()
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(body); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("commit %s: %w", path, err)
	}
	ok = true
	return nil
}

func slugForPath(articleURL string) string {
	u, _ := url.Parse(articleURL)
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	value := "article"
	if len(parts) > 0 && parts[len(parts)-1] != "" {
		value = parts[len(parts)-1]
	}
	value = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' {
			return unicode.ToLower(r)
		}
		return '-'
	}, value)
	value = strings.Trim(strings.Join(strings.FieldsFunc(value, func(r rune) bool { return r == '-' }), "-"), "-")
	if value == "" {
		return "article"
	}
	if len(value) > 48 {
		value = strings.TrimRight(value[:48], "-")
	}
	return value
}

func sortedTrackingKeys(rawURL string) []string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil
	}
	var keys []string
	for key := range u.Query() {
		lower := strings.ToLower(key)
		if strings.HasPrefix(lower, "utm_") || trackingQueryKey(lower) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}
