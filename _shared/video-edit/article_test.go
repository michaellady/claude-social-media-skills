package videoedit

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestFetchArticleSelectsCanonicalItemAndBuildsStableBlocks(t *testing.T) {
	t.Parallel()

	imageBytes := []byte("fixture image")
	const origin = "https://feed.example.test"
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/feed":
			feed := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <item>
      <title>Wrong post</title>
      <link>%s/p/wrong</link>
      <content:encoded><![CDATA[<p>Wrong body.</p>]]></content:encoded>
    </item>
    <item>
      <title>Target Essay</title>
      <link>%s/p/target?utm_source=rss</link>
      <guid>%s/p/target</guid>
      <pubDate>Fri, 10 Jul 2026 12:30:00 -0700</pubDate>
      <content:encoded><![CDATA[
        <h2>Introduction</h2>
        <p>Before <strong>the image</strong>.</p>
        <figure>
          <img src="/assets/map.png" alt="System map">
          <figcaption>Architecture overview</figcaption>
        </figure>
        <blockquote><p>A quoted idea.</p><cite>Jane Doe</cite></blockquote>
        <blockquote><br></blockquote>
        <p>After the quote.</p>
      ]]></content:encoded>
    </item>
  </channel>
</rss>`, origin, origin, origin)
			return httpResponse(request, http.StatusOK, "application/rss+xml", []byte(feed)), nil
		case "/assets/map.png":
			return httpResponse(request, http.StatusOK, "image/png", imageBytes), nil
		default:
			return httpResponse(request, http.StatusNotFound, "text/plain", []byte("not found")), nil
		}
	})}

	downloadDir := t.TempDir()
	rawSourcePath := filepath.Join(t.TempDir(), "article-source.xml")
	fixedTime := time.Date(2026, time.July, 11, 20, 0, 0, 0, time.UTC)
	options := ArticleFetchOptions{
		FeedURL:       origin + "/feed",
		CanonicalURL:  origin + "/p/target#section",
		DownloadDir:   downloadDir,
		RawSourcePath: rawSourcePath,
		HTTPClient:    client,
		Now:           func() time.Time { return fixedTime },
	}
	article, err := FetchArticleWithOptions(context.Background(), options)
	if err != nil {
		t.Fatalf("FetchArticleWithOptions() error = %v", err)
	}

	if article.Title != "Target Essay" {
		t.Fatalf("Title = %q, want Target Essay", article.Title)
	}
	if article.CanonicalURL != origin+"/p/target" {
		t.Fatalf("CanonicalURL = %q", article.CanonicalURL)
	}
	if article.PublishedAt != "2026-07-10T19:30:00Z" {
		t.Fatalf("PublishedAt = %q", article.PublishedAt)
	}
	if !article.FetchedAt.Equal(fixedTime) {
		t.Fatalf("FetchedAt = %s, want %s", article.FetchedAt, fixedTime)
	}
	if len(article.ContentHash) != 64 {
		t.Fatalf("ContentHash length = %d, want 64", len(article.ContentHash))
	}

	var kinds []string
	for _, block := range article.Blocks {
		kinds = append(kinds, block.Kind)
	}
	wantKinds := []string{"heading", "paragraph", "image", "blockquote", "blockquote", "paragraph"}
	if !reflect.DeepEqual(kinds, wantKinds) {
		t.Fatalf("block kinds = %#v, want %#v", kinds, wantKinds)
	}
	if article.Blocks[0].Level != 2 || article.Blocks[0].Text != "Introduction" {
		t.Fatalf("heading = %#v", article.Blocks[0])
	}
	if article.Blocks[3].Text != "A quoted idea." || article.Blocks[3].QuoteAttribution != "Jane Doe" {
		t.Fatalf("quote = %#v", article.Blocks[3])
	}
	if article.Blocks[4].Text != "" {
		t.Fatalf("empty blockquote text = %q", article.Blocks[4].Text)
	}
	if len(article.Warnings) != 1 || !strings.Contains(article.Warnings[0], "empty RSS blockquote") {
		t.Fatalf("Warnings = %#v", article.Warnings)
	}

	if len(article.Images) != 1 {
		t.Fatalf("Images length = %d, want 1", len(article.Images))
	}
	image := article.Images[0]
	if image.URL != origin+"/assets/map.png" || image.Alt != "System map" || image.Caption != "Architecture overview" {
		t.Fatalf("image metadata = %#v", image)
	}
	if image.BlockID != article.Blocks[2].ID || !reflect.DeepEqual(article.Blocks[2].ImageIDs, []string{image.ID}) {
		t.Fatalf("image/block links are inconsistent: image=%#v block=%#v", image, article.Blocks[2])
	}
	wantAdjacent := []string{article.Blocks[1].ID, article.Blocks[3].ID}
	if !reflect.DeepEqual(image.AdjacentTextBlockIDs, wantAdjacent) {
		t.Fatalf("AdjacentTextBlockIDs = %#v, want %#v", image.AdjacentTextBlockIDs, wantAdjacent)
	}
	downloaded, err := os.ReadFile(image.Path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", image.Path, err)
	}
	if string(downloaded) != string(imageBytes) {
		t.Fatalf("downloaded image = %q", downloaded)
	}
	if filepath.Dir(image.Path) != downloadDir {
		t.Fatalf("image path %q is outside download dir %q", image.Path, downloadDir)
	}
	rawSource, err := os.ReadFile(rawSourcePath)
	if err != nil {
		t.Fatalf("raw RSS snapshot was not written: %v", err)
	}
	if !strings.Contains(string(rawSource), "<title>Target Essay</title>") || article.RawSourcePath != rawSourcePath {
		t.Fatalf("raw RSS snapshot metadata/content is wrong: path=%q", article.RawSourcePath)
	}

	second, err := FetchArticleWithOptions(context.Background(), options)
	if err != nil {
		t.Fatalf("second FetchArticleWithOptions() error = %v", err)
	}
	if second.ContentHash != article.ContentHash {
		t.Fatalf("content hashes differ: %q != %q", second.ContentHash, article.ContentHash)
	}
	for index := range article.Blocks {
		if second.Blocks[index].ID != article.Blocks[index].ID {
			t.Fatalf("block %d ID changed: %q != %q", index, second.Blocks[index].ID, article.Blocks[index].ID)
		}
	}
	if second.Images[0].ID != article.Images[0].ID || second.Images[0].Path != article.Images[0].Path {
		t.Fatalf("image identity changed: %#v != %#v", second.Images[0], article.Images[0])
	}
}

func TestFetchArticleKeepsSnapshotWhenImageDownloadFails(t *testing.T) {
	t.Parallel()

	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/feed" {
			feed := `<?xml version="1.0"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
<channel><item><title>Post</title><link>https://feed.example.test/p/post</link>
<content:encoded><![CDATA[<p>Before.</p><img src="/missing.png"><p>After.</p>]]></content:encoded>
</item></channel></rss>`
			return httpResponse(request, http.StatusOK, "application/rss+xml", []byte(feed)), nil
		}
		return httpResponse(request, http.StatusServiceUnavailable, "text/plain", []byte("unavailable")), nil
	})}
	article, err := FetchArticleWithOptions(context.Background(), ArticleFetchOptions{
		FeedURL:      "https://feed.example.test/feed",
		CanonicalURL: "https://feed.example.test/p/post",
		DownloadDir:  t.TempDir(),
		HTTPClient:   client,
	})
	if err != nil {
		t.Fatalf("FetchArticleWithOptions() error = %v", err)
	}
	if len(article.Images) != 1 || article.Images[0].URL != "https://feed.example.test/missing.png" || article.Images[0].Path != "" {
		t.Fatalf("image was not preserved after download failure: %#v", article.Images)
	}
	if len(article.Warnings) != 1 || !strings.Contains(article.Warnings[0], "download failed") {
		t.Fatalf("warnings = %#v", article.Warnings)
	}
}

func TestFetchArticleReportsMissingCanonicalItem(t *testing.T) {
	t.Parallel()

	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return httpResponse(request, http.StatusOK, "application/rss+xml", []byte(`<?xml version="1.0"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
<channel><item><title>Other</title><link>https://example.test/p/other</link>
<content:encoded><![CDATA[<p>Other body.</p>]]></content:encoded></item></channel></rss>`)), nil
	})}

	_, err := FetchArticleWithOptions(context.Background(), ArticleFetchOptions{
		FeedURL:      "https://feed.example.test/rss",
		CanonicalURL: "https://example.test/p/missing",
		HTTPClient:   client,
	})
	if err == nil || !strings.Contains(err.Error(), "was not found") {
		t.Fatalf("error = %v, want not-found error", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func httpResponse(request *http.Request, status int, contentType string, body []byte) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(strings.NewReader(string(body))),
		Request:    request,
	}
}

func TestParseArticleHTMLStableIDsDistinguishDuplicateBlocks(t *testing.T) {
	t.Parallel()

	blocks, _, _, err := parseArticleHTML("<p>Repeat.</p><p>Repeat.</p>", "https://example.test/p/post")
	if err != nil {
		t.Fatalf("parseArticleHTML() error = %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("blocks length = %d, want 2", len(blocks))
	}
	if blocks[0].ID == blocks[1].ID {
		t.Fatalf("duplicate blocks received the same ID %q", blocks[0].ID)
	}

	again, _, _, err := parseArticleHTML("<p>Repeat.</p><p>Repeat.</p>", "https://example.test/p/post")
	if err != nil {
		t.Fatalf("second parseArticleHTML() error = %v", err)
	}
	if blocks[0].ID != again[0].ID || blocks[1].ID != again[1].ID {
		t.Fatalf("duplicate block IDs are not stable: %#v != %#v", blocks, again)
	}
}

func TestNormalizeURLPreservesEscapedPathWithoutDoubleEncoding(t *testing.T) {
	t.Parallel()

	got, err := normalizeURL("HTTPS://Example.Test/p/a%20b/?utm_source=rss#section")
	if err != nil {
		t.Fatalf("normalizeURL() error = %v", err)
	}
	if got != "https://example.test/p/a%20b" {
		t.Fatalf("normalizeURL() = %q", got)
	}
}
