package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// tagRule adds tags when any keyword appears in the video title or description.
// If Keywords is empty, the rule applies unconditionally.
type tagRule struct {
	Keywords []string
	Tags     []string
}

// liveTagRules are the keyword-to-tag mappings for the "live" content type.
// Universal rule (empty Keywords) fires first.
var liveTagRules = []tagRule{
	{Keywords: nil, Tags: []string{
		"vibe coding",
		"vibecoding",
		"enterprise vibe code",
		"claude code",
		"ai agents",
		"live coding",
		"agentic coding",
		"ai coding",
		"software engineering",
	}},
	{Keywords: []string{"gas town", "gastown"}, Tags: []string{
		"gas town",
		"gastown",
		"steve yegge",
		"agent orchestration",
		"multi agent",
	}},
	{Keywords: []string{"$200", "max 20x"}, Tags: []string{
		"claude code max",
		"anthropic",
	}},
	{Keywords: []string{"opus 4.6", "opus agent", "opus teams"}, Tags: []string{
		"claude opus",
		"claude opus 4.6",
	}},
	{Keywords: []string{"agent teams", "opus teams", "opus 4.6 teams"}, Tags: []string{
		"agent teams",
		"claude agent teams",
	}},
	{Keywords: []string{"bmad", "gsd", "superpowers", "openspec"}, Tags: []string{
		"bmad",
		"gsd",
		"superpowers",
		"openspec",
		"agent frameworks",
	}},
	{Keywords: []string{"kiro"}, Tags: []string{
		"kiro",
		"kiro spec",
	}},
	{Keywords: []string{"lovable"}, Tags: []string{
		"lovable",
	}},
	{Keywords: []string{"aws"}, Tags: []string{
		"aws",
		"cloud deployment",
	}},
	{Keywords: []string{"github"}, Tags: []string{
		"github",
		"github automation",
	}},
	{Keywords: []string{"ig threads", "instagram"}, Tags: []string{
		"instagram",
		"ig threads",
		"social media automation",
	}},
	{Keywords: []string{"bluesky"}, Tags: []string{
		"bluesky",
		"social media automation",
	}},
	{Keywords: []string{"buffer"}, Tags: []string{
		"buffer",
		"social media automation",
		"content scheduling",
	}},
	{Keywords: []string{"twitter clone", " x "}, Tags: []string{
		"twitter",
		"social media",
	}},
	{Keywords: []string{"wasteland"}, Tags: []string{
		"the wasteland",
	}},
	{Keywords: []string{"mcp"}, Tags: []string{
		"mcp",
		"model context protocol",
		"mcp servers",
	}},
	{Keywords: []string{"debug"}, Tags: []string{
		"debugging",
		"live debugging",
	}},
	{Keywords: []string{"deploy", "prod"}, Tags: []string{
		"deployment",
		"production deployment",
	}},
	{Keywords: []string{"auth"}, Tags: []string{
		"authentication",
	}},
	{Keywords: []string{"newsletter"}, Tags: []string{
		"newsletter",
		"content creation",
	}},
	{Keywords: []string{"skill", "skills"}, Tags: []string{
		"claude skills",
		"custom skills",
	}},
	{Keywords: []string{"saas"}, Tags: []string{
		"saas",
	}},
	{Keywords: []string{"roxas"}, Tags: []string{
		"content as code",
	}},
	{Keywords: []string{"plan mode"}, Tags: []string{
		"claude code plan mode",
	}},
}

// maxTagsTotalChars is YouTube's limit on combined tag length.
// Multi-word tags count with surrounding quotes, plus commas between tags.
const maxTagsTotalChars = 500

func runSuggestTags(dataPath, outputPath string, filter VideoFilter) error {
	data, err := loadData(dataPath)
	if err != nil {
		return err
	}
	videos := filter.Apply(data.Videos)
	if len(videos) == 0 {
		return fmt.Errorf("no videos match the filter")
	}

	out := map[string][]string{}
	for _, v := range videos {
		// Only live streams have a rule set today; skip other types.
		if v.VideoType != "live" {
			continue
		}
		tags := tagsForVideo(v, liveTagRules)
		tags = trimToYouTubeLimit(tags, maxTagsTotalChars)
		out[v.ID] = tags
	}

	if err := os.MkdirAll(dirOf(outputPath), 0o755); err != nil {
		return err
	}
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return err
	}

	fmt.Printf("Wrote %d tag set(s) to %s\n", len(out), outputPath)
	fmt.Println()
	fmt.Println("Sample of recommendations (first 5 newest):")
	var ids []string
	for id := range out {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		var a, b Video
		for _, v := range videos {
			if v.ID == ids[i] {
				a = v
			}
			if v.ID == ids[j] {
				b = v
			}
		}
		return a.PublishedAt.After(b.PublishedAt)
	})
	limit := 5
	if len(ids) < limit {
		limit = len(ids)
	}
	for _, id := range ids[:limit] {
		var title string
		for _, v := range videos {
			if v.ID == id {
				title = v.Title
				break
			}
		}
		fmt.Printf("  [%s] %s\n", id, truncate(title, 60))
		fmt.Printf("    %s\n", strings.Join(out[id], ", "))
		fmt.Println()
	}
	return nil
}

func tagsForVideo(v Video, rules []tagRule) []string {
	// Match keyword rules on the TITLE only — descriptions contain shared boilerplate
	// (recurring PR links, newsletter CTAs) that cause false positives.
	hay := strings.ToLower(v.Title)
	seen := map[string]bool{}
	var tags []string
	for _, r := range rules {
		match := len(r.Keywords) == 0
		for _, kw := range r.Keywords {
			if strings.Contains(hay, strings.ToLower(kw)) {
				match = true
				break
			}
		}
		if !match {
			continue
		}
		for _, t := range r.Tags {
			key := strings.ToLower(t)
			if seen[key] {
				continue
			}
			seen[key] = true
			tags = append(tags, t)
		}
	}
	return tags
}

// youtubeTagsCharCount returns the length YouTube bills the tag list at.
// Multi-word tags are quoted; tags are comma-separated.
func youtubeTagsCharCount(tags []string) int {
	total := 0
	for i, t := range tags {
		if i > 0 {
			total++ // comma
		}
		if strings.Contains(t, " ") {
			total += len(t) + 2 // quotes
		} else {
			total += len(t)
		}
	}
	return total
}

func trimToYouTubeLimit(tags []string, limit int) []string {
	for youtubeTagsCharCount(tags) > limit && len(tags) > 0 {
		tags = tags[:len(tags)-1]
	}
	return tags
}

func dirOf(path string) string {
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		return path[:idx]
	}
	return "."
}
