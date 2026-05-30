package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	cohortsYAMLPath       = "data/cohorts.yaml"
	cohortAssignmentsPath = "data/cohort_assignments.json"
	videosJSONPath        = "data/videos.json"
)

// Cohort is a named group of videos with optional auto-assignment rules.
// Rules are OR'd together: a video matches if any rule matches.
type Cohort struct {
	ID          string       `yaml:"id" json:"id"`
	Name        string       `yaml:"name" json:"name"`
	Description string       `yaml:"description,omitempty" json:"description,omitempty"`
	Rules       []CohortRule `yaml:"rules,omitempty" json:"rules,omitempty"`
	CreatedAt   time.Time    `yaml:"created_at,omitempty" json:"created_at,omitempty"`
}

// CohortRule predicates within a single rule are AND'd together. An empty/zero
// field means "don't constrain on this dimension".
type CohortRule struct {
	TitleRegex       string    `yaml:"title_regex,omitempty" json:"title_regex,omitempty"`
	DescRegex        string    `yaml:"description_regex,omitempty" json:"description_regex,omitempty"`
	VideoType        string    `yaml:"video_type,omitempty" json:"video_type,omitempty"`
	DurationMin      int       `yaml:"duration_min,omitempty" json:"duration_min,omitempty"`
	DurationMax      int       `yaml:"duration_max,omitempty" json:"duration_max,omitempty"`
	PublishedAfter   time.Time `yaml:"published_after,omitempty" json:"published_after,omitempty"`
	PublishedBefore  time.Time `yaml:"published_before,omitempty" json:"published_before,omitempty"`
}

// loadCohorts reads cohorts from YAML. A missing file returns nil, nil — this
// makes the system tolerate a fresh install with no cohorts defined. Regex
// rules are compiled here so typos in cohorts.yaml fail fast at load time
// instead of silently never matching during autoAssign.
func loadCohorts(path string) ([]Cohort, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading cohorts file: %w", err)
	}
	var cohorts []Cohort
	if err := yaml.Unmarshal(data, &cohorts); err != nil {
		return nil, fmt.Errorf("parsing cohorts yaml: %w", err)
	}
	for _, c := range cohorts {
		for i, r := range c.Rules {
			if r.TitleRegex != "" {
				if _, err := regexp.Compile(r.TitleRegex); err != nil {
					return nil, fmt.Errorf("cohort %q rule %d: invalid title_regex %q: %w", c.ID, i, r.TitleRegex, err)
				}
			}
			if r.DescRegex != "" {
				if _, err := regexp.Compile(r.DescRegex); err != nil {
					return nil, fmt.Errorf("cohort %q rule %d: invalid description_regex %q: %w", c.ID, i, r.DescRegex, err)
				}
			}
		}
	}
	return cohorts, nil
}

// loadAssignments reads the video→cohorts map. Missing file returns empty map.
func loadAssignments(path string) (map[string][]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string][]string{}, nil
		}
		return nil, fmt.Errorf("reading assignments: %w", err)
	}
	out := map[string][]string{}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing assignments json: %w", err)
	}
	return out, nil
}

// saveAssignments writes the map sorted (stable diffs) and pretty-printed.
func saveAssignments(path string, m map[string][]string) error {
	// Stable output: sort the cohort lists per video.
	clone := make(map[string][]string, len(m))
	for k, v := range m {
		cp := append([]string(nil), v...)
		sort.Strings(cp)
		clone[k] = cp
	}
	data, err := json.MarshalIndent(clone, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
		return fmt.Errorf("writing assignments: %w", err)
	}
	return nil
}

// matchRule returns true if the video matches every constraint in the rule.
// An empty rule (zero value) matches nothing — that prevents a stray empty
// rule from sweeping everything into a cohort.
func matchRule(r CohortRule, v Video) bool {
	if isZeroRule(r) {
		return false
	}
	if r.TitleRegex != "" {
		re, err := regexp.Compile(r.TitleRegex)
		if err != nil || !re.MatchString(v.Title) {
			return false
		}
	}
	if r.DescRegex != "" {
		re, err := regexp.Compile(r.DescRegex)
		if err != nil || !re.MatchString(v.Description) {
			return false
		}
	}
	if r.VideoType != "" && v.VideoType != r.VideoType {
		return false
	}
	if r.DurationMin > 0 && v.DurationSeconds < r.DurationMin {
		return false
	}
	if r.DurationMax > 0 && v.DurationSeconds > r.DurationMax {
		return false
	}
	if !r.PublishedAfter.IsZero() && v.PublishedAt.Before(r.PublishedAfter) {
		return false
	}
	if !r.PublishedBefore.IsZero() && v.PublishedAt.After(r.PublishedBefore) {
		return false
	}
	return true
}

func isZeroRule(r CohortRule) bool {
	return r.TitleRegex == "" && r.DescRegex == "" && r.VideoType == "" &&
		r.DurationMin == 0 && r.DurationMax == 0 &&
		r.PublishedAfter.IsZero() && r.PublishedBefore.IsZero()
}

// matchCohort returns true if any rule on the cohort matches the video.
// A cohort with no rules matches nothing automatically (manual-only).
func matchCohort(c Cohort, v Video) bool {
	for _, r := range c.Rules {
		if matchRule(r, v) {
			return true
		}
	}
	return false
}

// autoAssignAll re-derives the full assignment map from rules + manual additions.
//
// Algorithm: for every (video, cohort) pair, decide membership = ruleMatch OR
// manualKept. A "manual" assignment is one that exists in `existing` but is
// NOT produced by any rule for that (video, cohort) — meaning a human or a
// previous run added it intentionally. Rule-driven assignments that no
// longer match are dropped (stale removal).
func autoAssignAll(cohorts []Cohort, videos []Video, existing map[string][]string) map[string][]string {
	out := map[string][]string{}
	cohortByID := make(map[string]Cohort, len(cohorts))
	for _, c := range cohorts {
		cohortByID[c.ID] = c
	}
	for _, v := range videos {
		seen := map[string]bool{}
		for _, c := range cohorts {
			if matchCohort(c, v) {
				if !seen[c.ID] {
					out[v.ID] = append(out[v.ID], c.ID)
					seen[c.ID] = true
				}
			}
		}
		// Preserve manual assignments: a cohort with NO rules is manual-only,
		// so any existing assignment to it is preserved. A cohort WITH rules
		// is auto-managed — if the rule no longer matches, the assignment is
		// stale and gets dropped (handled by the loop above not adding it).
		for _, cohortID := range existing[v.ID] {
			c, known := cohortByID[cohortID]
			if !known {
				continue // orphan: cohort definition removed
			}
			if len(c.Rules) > 0 {
				continue // auto-managed: rule above is the source of truth
			}
			if !seen[cohortID] {
				out[v.ID] = append(out[v.ID], cohortID)
				seen[cohortID] = true
			}
		}
	}
	// Drop entries with no assignments to keep the file sparse.
	for k, v := range out {
		if len(v) == 0 {
			delete(out, k)
		}
	}
	return out
}
