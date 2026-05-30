package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

func runUpdateTags(dataPath, tagsPath string, filter VideoFilter, apply bool) error {
	data, err := loadData(dataPath)
	if err != nil {
		return err
	}

	raw, err := os.ReadFile(tagsPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", tagsPath, err)
	}
	var tagMap map[string][]string
	if err := json.Unmarshal(raw, &tagMap); err != nil {
		return fmt.Errorf("parsing %s: %w", tagsPath, err)
	}

	// Apply the same filter flags to restrict which videos we touch.
	videos := filter.Apply(data.Videos)
	videoByID := map[string]Video{}
	for _, v := range videos {
		videoByID[v.ID] = v
	}

	// Build the change plan.
	type change struct {
		video   Video
		oldTags []string
		newTags []string
	}
	var changes []change
	for id, newTags := range tagMap {
		v, ok := videoByID[id]
		if !ok {
			// Video not in filtered set — skip silently.
			continue
		}
		if tagsEqual(v.Tags, newTags) {
			continue
		}
		changes = append(changes, change{video: v, oldTags: v.Tags, newTags: newTags})
	}

	mode := "DRY RUN"
	if apply {
		mode = "APPLY"
	}
	fmt.Println("═══════════════════════════════════════════════════════════")
	fmt.Printf("  Tag Update — %s\n", mode)
	if !filter.IsZero() {
		fmt.Printf("  Filter: %s\n", filter.Describe())
	}
	fmt.Printf("  Tags file: %s (%d entries)\n", tagsPath, len(tagMap))
	fmt.Printf("  Matched videos: %d\n", len(changes))
	fmt.Println("═══════════════════════════════════════════════════════════")
	if len(changes) == 0 {
		fmt.Println("  Nothing to update — current tags already match recommendations.")
		return nil
	}

	for i, c := range changes {
		fmt.Printf("\n  %d. [%s] %s\n", i+1, c.video.ID, truncate(c.video.Title, 55))
		if len(c.oldTags) == 0 {
			fmt.Println("     current: (none)")
		} else {
			fmt.Printf("     current: %s\n", strings.Join(c.oldTags, ", "))
		}
		fmt.Printf("     → new:  %s\n", strings.Join(c.newTags, ", "))
		fmt.Printf("     chars:  %d / %d\n", youtubeTagsCharCount(c.newTags), maxTagsTotalChars)
	}

	if !apply {
		fmt.Println()
		fmt.Println("  (dry run — no videos modified. Re-run with --apply to push changes.)")
		return nil
	}

	// Push changes via YouTube Data API v3 videos.update.
	ctx := context.Background()
	client, err := getWriteClient(ctx)
	if err != nil {
		return fmt.Errorf("OAuth (write scope) failed: %w", err)
	}
	svc, err := youtube.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return fmt.Errorf("creating YouTube service: %w", err)
	}

	fmt.Println()
	fmt.Printf("  Pushing %d updates...\n", len(changes))
	var failed int
	for i, c := range changes {
		if err := pushTagUpdate(svc, c.video.ID, c.newTags); err != nil {
			fmt.Fprintf(os.Stderr, "    %d. [%s] FAILED: %v\n", i+1, c.video.ID, err)
			failed++
			continue
		}
		fmt.Printf("    %d. [%s] OK\n", i+1, c.video.ID)
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d updates failed", failed, len(changes))
	}
	fmt.Println()
	fmt.Println("  All updates succeeded. Run `go run . fetch` to refresh the local cache.")
	return nil
}

// pushTagUpdate fetches the video's current snippet, replaces tags, and writes it back.
// videos.update requires the complete snippet (else fields reset to empty).
func pushTagUpdate(svc *youtube.Service, id string, tags []string) error {
	resp, err := svc.Videos.List([]string{"snippet"}).Id(id).Do()
	if err != nil {
		return fmt.Errorf("fetching snippet: %w", err)
	}
	if len(resp.Items) == 0 {
		return fmt.Errorf("video not found on YouTube")
	}
	item := resp.Items[0]
	item.Snippet.Tags = tags
	_, err = svc.Videos.Update([]string{"snippet"}, item).Do()
	if err != nil {
		return fmt.Errorf("update call: %w", err)
	}
	return nil
}

func tagsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
