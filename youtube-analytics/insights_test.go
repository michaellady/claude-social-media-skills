package main

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"
)

func TestInsightFile_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "2026-04-26.md")

	original := InsightFile{
		Date: mustDate(t, "2026-04-26"),
		Hypotheses: []Hypothesis{
			{
				ID:               "h-2026-04-26-1",
				Cohort:           "gastown-series",
				Prediction:       "Title variation lifts retention",
				EvidenceVideoIDs: []string{"abc", "def"},
				Metric:           "retention",
				Direction:        "up",
				EvaluateAfter:    mustDate(t, "2026-05-10"),
			},
			{
				ID:            "h-2026-04-26-2",
				Cohort:        "claude-code",
				Prediction:    "Adding install steps boosts conversion",
				EvaluateAfter: mustDate(t, "2026-05-03"),
				Outcome:       "Conversion went from 1.2 to 2.1 subs/1K",
				Verdict:       "confirm",
			},
		},
		Body: "# Insights — 2026-04-26\n\nFreeform notes here.\n",
	}
	if err := writeInsightFile(path, &original); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := readInsightFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !got.Date.Equal(original.Date) {
		t.Errorf("date mismatch: got %v, want %v", got.Date, original.Date)
	}
	if len(got.Hypotheses) != 2 {
		t.Fatalf("hypothesis count: got %d, want 2", len(got.Hypotheses))
	}
	if !reflect.DeepEqual(sortVideoIDs(got.Hypotheses[0].EvidenceVideoIDs), []string{"abc", "def"}) {
		t.Errorf("evidence_video_ids: got %v", got.Hypotheses[0].EvidenceVideoIDs)
	}
	if got.Hypotheses[1].Verdict != "confirm" {
		t.Errorf("verdict: got %q", got.Hypotheses[1].Verdict)
	}
	// Body roundtrip is not strictly required but should not be lost
	if got.Body == "" {
		t.Errorf("body was dropped on roundtrip")
	}
}

func TestListInsights_FindsAllFiles(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"2026-04-19", "2026-04-26", "2026-05-03"} {
		if err := writeInsightFile(filepath.Join(dir, d+".md"), &InsightFile{
			Date: mustDate(t, d),
			Hypotheses: []Hypothesis{{
				ID: "h-" + d, Cohort: "gastown-series",
				Prediction: "x", EvaluateAfter: mustDate(t, "2026-06-01"),
			}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	files, err := listInsightFiles(dir)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(files) != 3 {
		t.Errorf("expected 3 files, got %d", len(files))
	}
}

func TestPendingHypotheses_FiltersByEvalDate(t *testing.T) {
	dir := t.TempDir()
	now := mustDate(t, "2026-04-27")

	// One past-due, one future, one already graded.
	files := []InsightFile{
		{Date: mustDate(t, "2026-04-19"), Hypotheses: []Hypothesis{{
			ID: "past-due", Cohort: "x", Prediction: "p",
			EvaluateAfter: mustDate(t, "2026-04-25"), // past
		}}},
		{Date: mustDate(t, "2026-04-19"), Hypotheses: []Hypothesis{{
			ID: "future", Cohort: "x", Prediction: "p",
			EvaluateAfter: mustDate(t, "2026-05-15"), // future
		}}},
		{Date: mustDate(t, "2026-04-12"), Hypotheses: []Hypothesis{{
			ID: "graded", Cohort: "x", Prediction: "p",
			EvaluateAfter: mustDate(t, "2026-04-20"),
			Verdict:       "confirm", // already graded
		}}},
	}
	for i, f := range files {
		// Must use distinct paths since two share a date.
		path := filepath.Join(dir, f.Date.Format("2006-01-02")+"-"+string(rune('a'+i))+".md")
		if err := writeInsightFile(path, &f); err != nil {
			t.Fatal(err)
		}
	}

	pending, err := pendingHypotheses(dir, now)
	if err != nil {
		t.Fatalf("pending: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending, got %d: %+v", len(pending), pending)
	}
	if pending[0].ID != "past-due" {
		t.Errorf("expected past-due, got %s", pending[0].ID)
	}
}

func TestGradeHypothesis_WritesBack(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "2026-04-19.md")
	if err := writeInsightFile(path, &InsightFile{
		Date: mustDate(t, "2026-04-19"),
		Hypotheses: []Hypothesis{{
			ID: "h-1", Cohort: "x", Prediction: "p",
			EvaluateAfter: mustDate(t, "2026-04-25"),
		}},
	}); err != nil {
		t.Fatal(err)
	}

	if err := gradeHypothesis(dir, "h-1", "refute", "It went down 5pp instead"); err != nil {
		t.Fatalf("grade: %v", err)
	}

	got, err := readInsightFile(path)
	if err != nil {
		t.Fatal(err)
	}
	h := got.Hypotheses[0]
	if h.Verdict != "refute" || h.Outcome != "It went down 5pp instead" {
		t.Errorf("not persisted: verdict=%q outcome=%q", h.Verdict, h.Outcome)
	}
}

func TestGradeHypothesis_UnknownID(t *testing.T) {
	dir := t.TempDir()
	if err := gradeHypothesis(dir, "missing", "confirm", "real outcome text"); err == nil {
		t.Errorf("expected error for unknown hypothesis id")
	}
}

func TestGradeHypothesis_RejectsBadVerdict(t *testing.T) {
	dir := t.TempDir()
	if err := gradeHypothesis(dir, "irrelevant", "yes", "outcome"); err == nil {
		t.Errorf("expected error for verdict outside the closed set")
	}
}

func TestGradeHypothesis_RejectsEmptyOutcome(t *testing.T) {
	dir := t.TempDir()
	if err := gradeHypothesis(dir, "irrelevant", "confirm", "   "); err == nil {
		t.Errorf("expected error for whitespace-only outcome")
	}
}

func mustDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func sortVideoIDs(ids []string) []string {
	cp := append([]string(nil), ids...)
	sort.Strings(cp)
	return cp
}

// Sanity check that the temp dir is properly isolated.
func TestInsight_TempDirIsolation(t *testing.T) {
	d1 := t.TempDir()
	d2 := t.TempDir()
	if d1 == d2 {
		t.Fatal("expected distinct temp dirs")
	}
	if _, err := os.Stat(d1); err != nil {
		t.Fatal(err)
	}
}
