// model.go — tolerant loader for the frozen flywheel snapshot contract
// (flywheel/SKILL.md Phase 7, schema_version "1"). Decodes the top level into a
// raw key→value map so a partial / older-shape snapshot renders whatever keys it
// has rather than crashing the reader. Section-level decoding is lazy and only
// for the keys the server needs server-side (content_attribution drill-down).
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Snapshot is one flywheel snapshot file, kept as raw top-level keys.
type Snapshot struct {
	Path string
	Date string // YYYY-MM-DD from the filename
	Top  map[string]json.RawMessage
}

func loadSnapshot(path string) (*Snapshot, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	top := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, fmt.Errorf("parse snapshot %s: %w", path, err)
	}
	date := strings.TrimSuffix(filepath.Base(path), ".json")
	return &Snapshot{Path: path, Date: date, Top: top}, nil
}

// has reports whether a non-null top-level key is present.
func (s *Snapshot) has(key string) bool {
	v, ok := s.Top[key]
	return ok && len(v) > 0 && string(v) != "null"
}

// raw returns a top-level key's bytes (or JSON null if absent).
func (s *Snapshot) raw(key string) json.RawMessage {
	if v, ok := s.Top[key]; ok {
		return v
	}
	return json.RawMessage("null")
}

// contentAttribution decodes the content_attribution array into raw per-record
// messages (each a full source+derivatives record). Empty on absence/parse error.
func (s *Snapshot) contentAttribution() []json.RawMessage {
	var recs []json.RawMessage
	if v, ok := s.Top["content_attribution"]; ok {
		_ = json.Unmarshal(v, &recs) // tolerate: empty on failure
	}
	return recs
}

// findSource returns the content_attribution record whose source.id matches id.
func (s *Snapshot) findSource(id string) (json.RawMessage, bool) {
	for _, rec := range s.contentAttribution() {
		var probe struct {
			Source struct {
				ID string `json:"id"`
			} `json:"source"`
		}
		if err := json.Unmarshal(rec, &probe); err != nil {
			continue
		}
		if probe.Source.ID == id {
			return rec, true
		}
	}
	return nil, false
}
