package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type fixture struct {
	server       *httptest.Server
	feedURL      string
	articleURL   string
	renderedPath string
	outDir       string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	var serverURL string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/feed.xml":
			w.Header().Set("Content-Type", "application/rss+xml")
			fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>Fixture</title>
    <item>
      <title>Older article</title>
      <link>https://origin.example/p/older</link>
      <pubDate>Sun, 14 Jun 2026 09:00:00 +0000</pubDate>
      <content:encoded><![CDATA[<p>Old body</p>]]></content:encoded>
      <enclosure url="%s/cover.jpg" type="image/jpeg" />
    </item>
    <item>
      <title>Latest article</title>
      <link>https://origin.example/p/latest-article</link>
      <pubDate>Mon, 15 Jun 2026 09:00:00 +0000</pubDate>
      <content:encoded><![CDATA[
        <div class="newsletter-content">
          <p>First paragraph with a <a href="https://destination.example/read?utm_source=feed&amp;keep=1&amp;fbclid=nope">clean link</a>.</p>
          <h2>Source heading</h2>
          <figure><img src="%s/body.png?utm_medium=rss" alt="RSS alt"><figcaption>RSS caption</figcaption></figure>
          <p>Second paragraph.</p>
          <blockquote>Source-grounded quote.</blockquote>
          <p>Manage preferences or unsubscribe here.</p>
        </div>
      ]]></content:encoded>
      <enclosure url="%s/cover.jpg?utm_campaign=cover" type="image/jpeg" />
    </item>
  </channel>
</rss>`, serverURL, serverURL, serverURL)
		case "/cover.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("fixture-cover"))
		case "/body.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("fixture-body"))
		default:
			http.NotFound(w, r)
		}
	})
	server := httptest.NewServer(handler)
	serverURL = server.URL

	tmp := t.TempDir()
	articleURL := "https://newsletter.example/p/latest-article"
	snapshot := renderedSnapshot{
		URL:             articleURL,
		Title:           "Latest article",
		Subtitle:        "Source subtitle",
		PublishedAt:     "2026-06-15",
		CoverImageURL:   server.URL + "/cover.jpg?utm_source=rendered",
		ParagraphCount:  intPointer(2),
		HeadingCount:    intPointer(1),
		BlockquoteCount: intPointer(1),
		BodyImages: []renderedImage{{
			Src:     server.URL + "/body.png?utm_source=rendered&width=1200",
			Alt:     "Rendered alt",
			Caption: "Rendered caption",
		}},
	}
	renderedPath := filepath.Join(tmp, "rendered.json")
	b, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(renderedPath, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return fixture{
		server:       server,
		feedURL:      server.URL + "/feed.xml",
		articleURL:   articleURL,
		renderedPath: renderedPath,
		outDir:       filepath.Join(tmp, "runs"),
	}
}

func intPointer(value int) *int {
	return &value
}

func TestPreparePreservesOrderSanitizesAndWritesManifest(t *testing.T) {
	fx := newFixture(t)
	defer fx.server.Close()

	m, err := prepare(context.Background(), fx.server.Client(), prepareOptions{
		FeedURL:      fx.feedURL,
		Article:      "latest",
		RenderedPath: fx.renderedPath,
		OutDir:       fx.outDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	if m.Source.ArticleURL != fx.articleURL {
		t.Fatalf("article URL = %q, want %q", m.Source.ArticleURL, fx.articleURL)
	}
	if m.Source.Title != "Latest article" || m.Source.Subtitle != "Source subtitle" {
		t.Fatalf("unexpected source metadata: %+v", m.Source)
	}
	if got, want := m.Counts, (countManifest{Paragraphs: 2, Headings: 1, BodyImages: 1, Blockquotes: 1}); got != want {
		t.Fatalf("counts = %+v, want %+v", got, want)
	}
	if m.RunID != filepath.Base(m.RunDirectory) || !strings.HasPrefix(m.RunDirectory, fx.outDir+string(os.PathSeparator)) {
		t.Fatalf("unexpected run directory: %q", m.RunDirectory)
	}
	for _, path := range []string{m.Cover.LocalPath, m.Images[0].LocalPath, m.Artifacts.LinkedInSubstackHTML, m.Artifacts.SubstackHTML, m.Artifacts.MediumHTML, m.Artifacts.PlainText, m.Artifacts.Manifest} {
		if info, err := os.Stat(path); err != nil || info.Size() == 0 {
			t.Fatalf("artifact %q missing or empty: %v", path, err)
		}
	}
	if m.Images[0].Index != 1 || m.Images[0].ElementIndex != 2 {
		t.Fatalf("image position = %+v", m.Images[0])
	}
	if strings.Contains(m.Images[0].SourceURL, "utm_") || !strings.Contains(m.Images[0].SourceURL, "width=1200") {
		t.Fatalf("body image URL not sanitized correctly: %q", m.Images[0].SourceURL)
	}

	linkedin := mustRead(t, m.Artifacts.LinkedInSubstackHTML)
	assertInOrder(t, linkedin,
		"First paragraph",
		"Source heading",
		"CROSSPOST_IMAGE_01",
		"Second paragraph",
		"Source-grounded quote",
	)
	if strings.Contains(linkedin, "unsubscribe") || strings.Contains(linkedin, "utm_") || strings.Contains(linkedin, "fbclid") {
		t.Fatalf("LinkedIn/Substack HTML contains boilerplate or tracking: %s", linkedin)
	}
	if !strings.Contains(linkedin, "keep=1") || strings.Count(linkedin, "CROSSPOST_IMAGE_01") != 1 {
		t.Fatalf("LinkedIn/Substack HTML lost stable query or image anchor: %s", linkedin)
	}

	substack := mustRead(t, m.Artifacts.SubstackHTML)
	assertInOrder(t, substack,
		"First paragraph",
		"Source heading",
		serverImage(fx.server.URL, "/body.png"),
		"Rendered caption",
		"Second paragraph",
	)
	if strings.Contains(substack, "CROSSPOST_IMAGE") || strings.Contains(substack, "utm_") {
		t.Fatalf("Substack HTML contains an image anchor or tracking parameter: %s", substack)
	}
	if !strings.Contains(substack, `<figure><img src="`) || !strings.Contains(substack, `alt="Rendered alt"`) {
		t.Fatalf("Substack HTML is not paste-ready rich HTML: %s", substack)
	}

	medium := mustRead(t, m.Artifacts.MediumHTML)
	assertInOrder(t, medium,
		serverImage(fx.server.URL, "/cover.jpg"),
		"First paragraph",
		"Source heading",
		serverImage(fx.server.URL, "/body.png"),
		"Second paragraph",
	)
	if !strings.Contains(medium, "Rendered caption") {
		t.Fatalf("Medium HTML missing rendered caption: %s", medium)
	}

	var disk manifest
	if err := json.Unmarshal([]byte(mustRead(t, m.Artifacts.Manifest)), &disk); err != nil {
		t.Fatal(err)
	}
	if disk.SchemaVersion != schemaVersion || disk.RunDirectory != m.RunDirectory || len(disk.Images) != 1 || disk.Artifacts.SubstackHTML != m.Artifacts.SubstackHTML {
		t.Fatalf("unexpected manifest on disk: %+v", disk)
	}
}

func TestRenderSubstackKeepsTextAdjacentToImages(t *testing.T) {
	elements := []element{
		{Kind: "paragraph", HTML: "<p>Before image</p>", Text: "Before image"},
		{Kind: "image", Image: &imageData{SourceURL: "https://media.example/one.png", Alt: "One", Caption: "Caption one"}},
		{Kind: "heading", HTML: "<h2>After-image heading</h2>", Text: "After-image heading"},
		{Kind: "paragraph", HTML: "<p>After image paragraph</p>", Text: "After image paragraph"},
	}

	got := renderSubstack(elements)
	assertInOrder(t, got, "Before image", "https://media.example/one.png", "Caption one", "After-image heading", "After image paragraph")
	if strings.Contains(got, "CROSSPOST_IMAGE") {
		t.Fatalf("paste-ready Substack HTML contains an upload anchor: %s", got)
	}
}

func TestPrepareUsesUniqueRunDirectories(t *testing.T) {
	fx := newFixture(t)
	defer fx.server.Close()
	opts := prepareOptions{FeedURL: fx.feedURL, Article: fx.articleURL, RenderedPath: fx.renderedPath, OutDir: fx.outDir}

	first, err := prepare(context.Background(), fx.server.Client(), opts)
	if err != nil {
		t.Fatal(err)
	}
	second, err := prepare(context.Background(), fx.server.Client(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if first.RunDirectory == second.RunDirectory {
		t.Fatalf("run directories collided: %q", first.RunDirectory)
	}
}

func TestMalformedInputs(t *testing.T) {
	fx := newFixture(t)
	defer fx.server.Close()

	tests := []struct {
		name string
		opts prepareOptions
		want string
	}{
		{"missing feed", prepareOptions{Article: "latest", RenderedPath: fx.renderedPath}, "--feed-url is required"},
		{"bad article", prepareOptions{FeedURL: fx.feedURL, Article: "not-a-url", RenderedPath: fx.renderedPath}, "invalid --article"},
		{"missing snapshot", prepareOptions{FeedURL: fx.feedURL, Article: "latest", RenderedPath: filepath.Join(t.TempDir(), "missing.json")}, "read rendered snapshot"},
		{"missing RSS item", prepareOptions{FeedURL: fx.feedURL, Article: "https://elsewhere.example/p/missing", RenderedPath: fx.renderedPath}, "not found in RSS"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := prepare(context.Background(), fx.server.Client(), tt.opts)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}

	b := []byte(mustRead(t, fx.renderedPath))
	var mismatch map[string]any
	if err := json.Unmarshal(b, &mismatch); err != nil {
		t.Fatal(err)
	}
	mismatch["paragraph_count"] = 999
	mismatchPath := filepath.Join(t.TempDir(), "mismatch.json")
	mismatchBytes, _ := json.Marshal(mismatch)
	if err := os.WriteFile(mismatchPath, mismatchBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := prepare(context.Background(), fx.server.Client(), prepareOptions{FeedURL: fx.feedURL, Article: "latest", RenderedPath: mismatchPath})
	if err == nil || !strings.Contains(err.Error(), "paragraphs: RSS=2 rendered=999") {
		t.Fatalf("count mismatch error = %v", err)
	}
}

func TestSelectItemUsesLatestPublicationDate(t *testing.T) {
	items := []rssItem{
		{Title: "First in feed", PubDate: "Sun, 14 Jun 2026 09:00:00 +0000"},
		{Title: "Actually latest", PubDate: "Mon, 15 Jun 2026 09:00:00 +0000"},
	}
	got, err := selectItem(items, "latest")
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Actually latest" {
		t.Fatalf("selected %q", got.Title)
	}
}

func TestMergeRenderedOnlyParagraphsPreservesRenderedIndexes(t *testing.T) {
	a := article{Elements: []element{
		{Kind: "paragraph", HTML: "<p>Zero</p>", Text: "Zero"},
		{Kind: "heading", HTML: "<h2>Heading</h2>", Text: "Heading"},
		{Kind: "paragraph", HTML: "<p>Original one</p>", Text: "Original one"},
	}}
	additions := []renderedParagraph{
		{Index: 2, Text: "Rendered two"},
		{Index: 1, Text: "Rendered one"},
	}
	if err := mergeRenderedParagraphs(&a, additions); err != nil {
		t.Fatal(err)
	}
	if got := countElements(a.Elements).Paragraphs; got != 4 {
		t.Fatalf("paragraph count = %d, want 4", got)
	}
	var text []string
	for _, el := range a.Elements {
		if el.Kind == "paragraph" {
			text = append(text, el.Text)
		}
	}
	if got, want := strings.Join(text, "|"), "Zero|Rendered one|Rendered two|Original one"; got != want {
		t.Fatalf("paragraph order = %q, want %q", got, want)
	}
	if !strings.Contains(a.Elements[2].HTML, "Rendered one") || a.Elements[1].Kind != "heading" {
		t.Fatalf("non-paragraph source order changed: %+v", a.Elements)
	}
}

func TestSkillReferencesAndPrimaryRules(t *testing.T) {
	skillRoot := filepath.Clean(filepath.Join("..", ".."))
	skillPath := filepath.Join(skillRoot, "SKILL.md")
	skill := mustRead(t, skillPath)
	if lines := strings.Count(skill, "\n") + 1; lines >= 500 {
		t.Fatalf("SKILL.md has %d lines; want fewer than 500", lines)
	}

	referencePattern := regexp.MustCompile(`\]\((references/[^)]+\.md)\)`)
	matches := referencePattern.FindAllStringSubmatch(skill, -1)
	seen := map[string]bool{}
	for _, match := range matches {
		if seen[match[1]] {
			continue
		}
		seen[match[1]] = true
		if _, err := os.Stat(filepath.Join(skillRoot, match[1])); err != nil {
			t.Fatalf("linked reference %q does not exist: %v", match[1], err)
		}
	}
	if len(seen) != 6 {
		t.Fatalf("found %d unique reference links, want 6", len(seen))
	}

	for _, forbidden := range []string{
		"HN link submissions do NOT support a text body",
		"if URL is filled, the text field must be empty",
		"Reddit blocks Claude in Chrome",
		"Default: all four non-Substack",
		"SKIP Substack",
		"gstack",
	} {
		if strings.Contains(skill, forbidden) {
			t.Fatalf("primary workflow contains conflicting/superseded rule %q", forbidden)
		}
	}
	for _, required := range []string{
		"A URL submission may also include text",
		"Never click a final Publish, Send, Submit, or Post control without a fresh approval",
		"Re-snapshot after navigation, post-type changes, modals, flair changes, or uploads",
		"Before the final receipt, recheck HN once more",
		"article-substack.html",
		"crosspost_verify",
	} {
		contents := skill
		switch required {
		case "A URL submission may also include text":
			contents = mustRead(t, filepath.Join(skillRoot, "references", "hacker-news.md"))
		case "Re-snapshot after navigation, post-type changes, modals, flair changes, or uploads":
			contents = mustRead(t, filepath.Join(skillRoot, "references", "troubleshooting.md"))
		case "article-substack.html":
			contents = mustRead(t, filepath.Join(skillRoot, "references", "substack.md"))
		case "crosspost_verify":
			contents = mustRead(t, filepath.Join(skillRoot, "references", "troubleshooting.md"))
		}
		if !strings.Contains(contents, required) {
			t.Fatalf("missing required canonical rule %q", required)
		}
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func assertInOrder(t *testing.T, value string, parts ...string) {
	t.Helper()
	position := -1
	for _, part := range parts {
		next := strings.Index(value, part)
		if next < 0 || next <= position {
			t.Fatalf("%q missing or out of order in %q", part, value)
		}
		position = next
	}
}

func serverImage(serverURL, path string) string {
	return serverURL + path
}
