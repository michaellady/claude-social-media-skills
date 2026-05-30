package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const insightsDir = "data/insights"

// Hypothesis is a single testable prediction tracked across weeks.
//
// Why YAML dates are wrapped: stdlib time.Time UnmarshalYAML expects RFC3339,
// but humans write `2026-04-26`. The custom unmarshaler accepts the date-only
// form too.
type Hypothesis struct {
	ID               string   `yaml:"id"`
	Cohort           string   `yaml:"cohort"`
	Prediction       string   `yaml:"prediction"`
	EvidenceVideoIDs []string `yaml:"evidence_video_ids,omitempty"`
	Metric           string   `yaml:"metric,omitempty"`    // e.g., "retention", "subs_per_1k"
	Direction        string   `yaml:"direction,omitempty"` // "up" | "down"
	EvaluateAfter    time.Time `yaml:"evaluate_after"`
	Outcome          string    `yaml:"outcome,omitempty"`
	Verdict          string    `yaml:"verdict,omitempty"` // "confirm" | "refute" | "inconclusive"
}

// InsightFile is the parsed shape of one data/insights/<date>.md file.
type InsightFile struct {
	Path       string       `yaml:"-"`
	Date       time.Time    `yaml:"date"`
	Hypotheses []Hypothesis `yaml:"hypotheses,omitempty"`
	Body       string       `yaml:"-"` // markdown body after the frontmatter
}

// frontmatter is the YAML shape used for serialization. We use a separate
// struct so that yyyy-mm-dd dates can use a tagged !!str without confusing
// time.Time's default RFC3339 parsing.
type frontmatter struct {
	Date       dateOnly        `yaml:"date"`
	Hypotheses []hypothesisYAML `yaml:"hypotheses,omitempty"`
}

type hypothesisYAML struct {
	ID               string   `yaml:"id"`
	Cohort           string   `yaml:"cohort"`
	Prediction       string   `yaml:"prediction"`
	EvidenceVideoIDs []string `yaml:"evidence_video_ids,omitempty"`
	Metric           string   `yaml:"metric,omitempty"`
	Direction        string   `yaml:"direction,omitempty"`
	EvaluateAfter    dateOnly `yaml:"evaluate_after"`
	Outcome          string   `yaml:"outcome,omitempty"`
	Verdict          string   `yaml:"verdict,omitempty"`
}

// dateOnly is a YAML-serializable date that round-trips as YYYY-MM-DD.
type dateOnly struct{ time.Time }

func (d dateOnly) MarshalYAML() (any, error) {
	if d.IsZero() {
		return nil, nil
	}
	return d.Format("2006-01-02"), nil
}

func (d *dateOnly) UnmarshalYAML(value *yaml.Node) error {
	s := strings.TrimSpace(value.Value)
	if s == "" {
		return nil
	}
	// Try date-only first, fall back to RFC3339 (forgiving).
	if t, err := time.Parse("2006-01-02", s); err == nil {
		d.Time = t
		return nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		d.Time = t
		return nil
	}
	return fmt.Errorf("unrecognized date format: %q", s)
}

// readInsightFile parses a single insight markdown file with YAML frontmatter.
// Files without frontmatter return an InsightFile whose Body holds the whole
// file (lenient — useful for human-only notes).
func readInsightFile(path string) (*InsightFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	front, body, err := splitFrontmatter(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	out := &InsightFile{Path: path, Body: body}
	if len(front) == 0 {
		return out, nil
	}
	var fm frontmatter
	if err := yaml.Unmarshal(front, &fm); err != nil {
		return nil, fmt.Errorf("%s: parse frontmatter: %w", path, err)
	}
	out.Date = fm.Date.Time
	for _, h := range fm.Hypotheses {
		out.Hypotheses = append(out.Hypotheses, Hypothesis{
			ID:               h.ID,
			Cohort:           h.Cohort,
			Prediction:       h.Prediction,
			EvidenceVideoIDs: h.EvidenceVideoIDs,
			Metric:           h.Metric,
			Direction:        h.Direction,
			EvaluateAfter:    h.EvaluateAfter.Time,
			Outcome:          h.Outcome,
			Verdict:          h.Verdict,
		})
	}
	return out, nil
}

// writeInsightFile serializes an InsightFile to disk with YAML frontmatter.
// Body is preserved verbatim. If Body is empty, a minimal heading is generated.
func writeInsightFile(path string, f *InsightFile) error {
	fm := frontmatter{Date: dateOnly{f.Date}}
	for _, h := range f.Hypotheses {
		fm.Hypotheses = append(fm.Hypotheses, hypothesisYAML{
			ID:               h.ID,
			Cohort:           h.Cohort,
			Prediction:       h.Prediction,
			EvidenceVideoIDs: h.EvidenceVideoIDs,
			Metric:           h.Metric,
			Direction:        h.Direction,
			EvaluateAfter:    dateOnly{h.EvaluateAfter},
			Outcome:          h.Outcome,
			Verdict:          h.Verdict,
		})
	}
	yamlBytes, err := yaml.Marshal(&fm)
	if err != nil {
		return err
	}
	body := f.Body
	if body == "" {
		body = fmt.Sprintf("# Insights — %s\n", f.Date.Format("2006-01-02"))
	}
	out := &bytes.Buffer{}
	out.WriteString("---\n")
	out.Write(yamlBytes)
	out.WriteString("---\n\n")
	out.WriteString(body)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, out.Bytes(), 0644)
}

// splitFrontmatter peels off a leading `---\n...\n---\n` block. Returns
// (frontmatterYAML, body, error). When no frontmatter is present, returns
// (nil, fullContent, nil).
func splitFrontmatter(raw []byte) ([]byte, string, error) {
	if !bytes.HasPrefix(raw, []byte("---\n")) {
		return nil, string(raw), nil
	}
	rest := raw[4:]
	idx := bytes.Index(rest, []byte("\n---"))
	if idx < 0 {
		return nil, "", fmt.Errorf("unterminated frontmatter")
	}
	front := rest[:idx]
	// Skip past `\n---` and any trailing newline.
	body := rest[idx+4:]
	body = bytes.TrimLeft(body, "\n")
	return front, string(body), nil
}

// listInsightFiles enumerates *.md files in dir, sorted by name (which is
// the date prefix, so it sorts chronologically).
func listInsightFiles(dir string) ([]*InsightFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var paths []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		paths = append(paths, filepath.Join(dir, e.Name()))
	}
	sort.Strings(paths)
	var out []*InsightFile
	for _, p := range paths {
		f, err := readInsightFile(p)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

// pendingHypotheses returns hypotheses with EvaluateAfter <= now and no
// verdict yet. Used by `insights pending` to surface what the user/model
// must grade this cycle.
func pendingHypotheses(dir string, now time.Time) ([]Hypothesis, error) {
	files, err := listInsightFiles(dir)
	if err != nil {
		return nil, err
	}
	var out []Hypothesis
	for _, f := range files {
		for _, h := range f.Hypotheses {
			if h.Verdict != "" {
				continue
			}
			if h.EvaluateAfter.IsZero() || h.EvaluateAfter.After(now) {
				continue
			}
			out = append(out, h)
		}
	}
	return out, nil
}

// gradeHypothesis finds a hypothesis by ID across all insight files in dir
// and writes back the verdict + outcome. Validates verdict is in the closed
// set and outcome is non-empty so callers can't slip judgment-free grades
// into the ledger (defense-in-depth — runInsightsGrade also validates).
func gradeHypothesis(dir, id, verdict, outcome string) error {
	switch verdict {
	case "confirm", "refute", "inconclusive":
	default:
		return fmt.Errorf("verdict must be confirm|refute|inconclusive, got %q", verdict)
	}
	if strings.TrimSpace(outcome) == "" {
		return fmt.Errorf("outcome must describe what actually happened (non-empty)")
	}
	files, err := listInsightFiles(dir)
	if err != nil {
		return err
	}
	for _, f := range files {
		for i := range f.Hypotheses {
			if f.Hypotheses[i].ID == id {
				f.Hypotheses[i].Verdict = verdict
				f.Hypotheses[i].Outcome = outcome
				return writeInsightFile(f.Path, f)
			}
		}
	}
	return fmt.Errorf("hypothesis %q not found in any %s/*.md", id, dir)
}
