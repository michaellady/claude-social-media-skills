package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// templatedHashtagRe matches descriptions that end in a known repeated hashtag pattern,
// which is a signal of auto-generated content.
var templatedHashtagRe = regexp.MustCompile(`(?i)#(vibecoding|aiagents|agenticcoding|devops|claudecode|enterprisevibecode)\b`)

type qualityIssues struct {
	missingTags      bool
	shortDescription bool
	templatedDesc    bool
	missingThumb     bool
}

func (q qualityIssues) any() bool {
	return q.missingTags || q.shortDescription || q.templatedDesc || q.missingThumb
}

func (q qualityIssues) labels() []string {
	var out []string
	if q.missingTags {
		out = append(out, "no-tags")
	}
	if q.shortDescription {
		out = append(out, "short-desc")
	}
	if q.templatedDesc {
		out = append(out, "templated")
	}
	if q.missingThumb {
		out = append(out, "no-thumb")
	}
	return out
}

func classifyQuality(v Video) qualityIssues {
	q := qualityIssues{}
	if len(v.Tags) == 0 {
		q.missingTags = true
	}
	if len(v.Description) < 50 {
		q.shortDescription = true
	}
	if countHashtagsInTail(v.Description) >= 3 {
		q.templatedDesc = true
	}
	if v.ThumbnailURL == "" {
		q.missingThumb = true
	}
	return q
}

// countHashtagsInTail counts distinct templated hashtags in the last 200 chars of the description.
func countHashtagsInTail(desc string) int {
	if len(desc) == 0 {
		return 0
	}
	start := 0
	if len(desc) > 200 {
		start = len(desc) - 200
	}
	tail := desc[start:]
	matches := templatedHashtagRe.FindAllString(tail, -1)
	seen := map[string]bool{}
	for _, m := range matches {
		seen[strings.ToLower(m)] = true
	}
	return len(seen)
}

func runDataQuality(dataPath string, filter VideoFilter) error {
	data, err := loadData(dataPath)
	if err != nil {
		return err
	}
	videos := filter.Apply(data.Videos)

	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Println("  Data Quality Report")
	if !filter.IsZero() {
		fmt.Printf("  Filter: %s\n", filter.Describe())
	}
	fmt.Printf("  Videos: %d\n", len(videos))
	fmt.Println("═══════════════════════════════════════════════════════════")
	if len(videos) == 0 {
		return nil
	}

	var missingTags, shortDesc, templated, missingThumb int
	byMonth := map[string]map[string]int{} // month → issue → count
	monthTotals := map[string]int{}
	type flagged struct {
		v      Video
		issues qualityIssues
	}
	var issues []flagged
	for _, v := range videos {
		q := classifyQuality(v)
		month := v.PublishedAt.Format("2006-01")
		monthTotals[month]++
		if _, ok := byMonth[month]; !ok {
			byMonth[month] = map[string]int{}
		}
		if q.missingTags {
			missingTags++
			byMonth[month]["no-tags"]++
		}
		if q.shortDescription {
			shortDesc++
			byMonth[month]["short-desc"]++
		}
		if q.templatedDesc {
			templated++
			byMonth[month]["templated"]++
		}
		if q.missingThumb {
			missingThumb++
			byMonth[month]["no-thumb"]++
		}
		if q.any() {
			issues = append(issues, flagged{v: v, issues: q})
		}
	}

	fmt.Println("── Issue Counts ──────────────────────────────────────────")
	total := float64(len(videos))
	fmt.Printf("  Missing tags:           %4d (%.1f%%)\n", missingTags, float64(missingTags)/total*100)
	fmt.Printf("  Short description <50:  %4d (%.1f%%)\n", shortDesc, float64(shortDesc)/total*100)
	fmt.Printf("  Templated hashtag tail: %4d (%.1f%%)\n", templated, float64(templated)/total*100)
	fmt.Printf("  Missing thumbnail:      %4d (%.1f%%)\n", missingThumb, float64(missingThumb)/total*100)
	fmt.Println()

	fmt.Println("── Issues by Month ───────────────────────────────────────")
	var months []string
	for m := range byMonth {
		months = append(months, m)
	}
	sort.Strings(months)
	fmt.Printf("  %-8s  %5s  %7s  %10s  %9s  %7s\n", "Month", "Vids", "No Tags", "Short Desc", "Templated", "NoThumb")
	for _, m := range months {
		c := byMonth[m]
		fmt.Printf("  %-8s  %5d  %7d  %10d  %9d  %7d\n",
			m, monthTotals[m], c["no-tags"], c["short-desc"], c["templated"], c["no-thumb"])
	}
	fmt.Println()

	fmt.Println("── Top 20 Flagged Videos (most recent) ───────────────────")
	sort.Slice(issues, func(i, j int) bool {
		return issues[i].v.PublishedAt.After(issues[j].v.PublishedAt)
	})
	limit := 20
	if len(issues) < limit {
		limit = len(issues)
	}
	for i := 0; i < limit; i++ {
		f := issues[i]
		fmt.Printf("  %s  [%s]  %s\n",
			f.v.PublishedAt.Format("2006-01-02"),
			f.v.ID,
			truncate(f.v.Title, 55),
		)
		fmt.Printf("    issues: %s\n", strings.Join(f.issues.labels(), ", "))
	}
	return nil
}
