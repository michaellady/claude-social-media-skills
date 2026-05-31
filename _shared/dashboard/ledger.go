// ledger.go — read the two hypothesis ledgers (repo-level cross-surface
// insights/ + youtube-analytics/data/insights/) for the dashboard Learning
// panel. Parses the same YAML-frontmatter shape the youtube-analytics insights
// binary writes; reading it in-process keeps the dashboard self-contained (no
// per-request shell-out to a maybe-unbuilt binary). Pure transport.
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type ledgerHypothesis struct {
	Ledger        string `json:"ledger"` // "cross-surface" | "youtube"
	FileDate      string `json:"date"`
	ID            string `json:"id"`
	Surface       string `json:"surface,omitempty"`
	Cohort        string `json:"cohort,omitempty"`
	Prediction    string `json:"prediction"`
	Metric        string `json:"metric,omitempty"`
	Direction     string `json:"direction,omitempty"`
	EvaluateAfter string `json:"evaluate_after,omitempty"`
	Outcome       string `json:"outcome,omitempty"`
	Verdict       string `json:"verdict,omitempty"`
}

// frontmatter matches the ledger file shape (only fields we render).
type ledgerFrontmatter struct {
	Date       string `yaml:"date"`
	Hypotheses []struct {
		ID            string `yaml:"id"`
		Surface       string `yaml:"surface"`
		Cohort        string `yaml:"cohort"`
		Prediction    string `yaml:"prediction"`
		Metric        string `yaml:"metric"`
		Direction     string `yaml:"direction"`
		EvaluateAfter string `yaml:"evaluate_after"`
		Outcome       string `yaml:"outcome"`
		Verdict       string `yaml:"verdict"`
	} `yaml:"hypotheses"`
}

// loadAllLedgers returns hypotheses from both ledgers, newest file first.
func loadAllLedgers() []ledgerHypothesis {
	out := []ledgerHypothesis{}
	out = append(out, loadLedger(crossSurfaceLedgerDir(), "cross-surface")...)
	out = append(out, loadLedger(youtubeLedgerDir(), "youtube")...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].FileDate > out[j].FileDate })
	return out
}

func loadLedger(dir, label string) []ledgerHypothesis {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []ledgerHypothesis
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		front, ok := frontmatterBytes(raw)
		if !ok {
			continue
		}
		var fm ledgerFrontmatter
		if err := yaml.Unmarshal(front, &fm); err != nil {
			continue
		}
		date := fm.Date
		if date == "" {
			date = strings.TrimSuffix(e.Name(), ".md")
		}
		for _, h := range fm.Hypotheses {
			out = append(out, ledgerHypothesis{
				Ledger: label, FileDate: date, ID: h.ID, Surface: h.Surface,
				Cohort: h.Cohort, Prediction: h.Prediction, Metric: h.Metric,
				Direction: h.Direction, EvaluateAfter: h.EvaluateAfter,
				Outcome: h.Outcome, Verdict: h.Verdict,
			})
		}
	}
	return out
}

// frontmatterBytes peels a leading ---\n...\n--- block. ok=false if absent.
func frontmatterBytes(raw []byte) ([]byte, bool) {
	if !bytes.HasPrefix(raw, []byte("---\n")) {
		return nil, false
	}
	rest := raw[4:]
	idx := bytes.Index(rest, []byte("\n---"))
	if idx < 0 {
		return nil, false
	}
	return rest[:idx], true
}
