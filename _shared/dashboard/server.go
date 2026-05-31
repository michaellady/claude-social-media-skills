// server.go — the localhost API + embedded single-page app. Every request reads
// the NEWEST snapshot fresh, so a new /flywheel run shows up on browser refresh
// without restarting the server. Bound to 127.0.0.1 only (never exposed).
package main

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"
	"time"
)

//go:embed static
var staticFS embed.FS

const staleThresholdDays = 14

type server struct {
	mux *http.ServeMux
}

func newServer() *server {
	s := &server{mux: http.NewServeMux()}

	sub, _ := fs.Sub(staticFS, "static")
	s.mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(sub))))
	s.mux.HandleFunc("/", s.index)
	s.mux.HandleFunc("/api/overview", s.overview)
	s.mux.HandleFunc("/api/learning", s.learning)
	s.mux.HandleFunc("/api/youtube", s.youtube)
	s.mux.HandleFunc("/api/linkedin", s.linkedin)
	s.mux.HandleFunc("/api/source/", s.source)
	return s
}

func (s *server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	b, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "index missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(b)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func (s *server) overview(w http.ResponseWriter, r *http.Request) {
	path := flywheelSnapshotPath()
	if path == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":   false,
			"meta": map[string]any{"has_snapshot": false},
			"hint": "No flywheel snapshot found in ~/dev/flywheel-snapshots/. Run /flywheel first.",
		})
		return
	}
	snap, err := loadSnapshot(path)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":   false,
			"meta": map[string]any{"has_snapshot": false, "error": err.Error(), "snapshot_path": path},
			"hint": "Snapshot present but unreadable — re-run /flywheel.",
		})
		return
	}

	// Voice freshness: prefer the snapshot's own (frozen contract), else read the
	// voice-corpus cache directly so an old-shape snapshot still shows it.
	var voice any
	if snap.has("voice_corpus_freshness") {
		voice = snap.raw("voice_corpus_freshness")
	} else {
		voice = loadVoiceFreshness()
	}

	age := snapshotAgeDays(snap.Date, time.Now())
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"meta": map[string]any{
			"has_snapshot":   true,
			"snapshot_path":  snap.Path,
			"snapshot_date":  snap.Date,
			"age_days":       age,
			"stale":          age < 0 || age > staleThresholdDays,
			"schema_version": snap.raw("schema_version"),
		},
		"snapshot": snap.Top, // passthrough every key present (tolerant of drift)
		"voice":    voice,
		"learning": ledgerSummary(),
	})
}

func (s *server) learning(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "hypotheses": loadAllLedgers()})
}

func (s *server) youtube(w http.ResponseWriter, r *http.Request) {
	vids, ok := loadYouTubeVideos(40)
	writeJSON(w, http.StatusOK, map[string]any{"ok": ok, "videos": vids})
}

func (s *server) linkedin(w http.ResponseWriter, r *http.Request) {
	posts, ok := loadLinkedInPosts()
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "available": false,
			"reason": "no linkedin-stats snapshot with profile.recent_posts[]"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "available": true, "recent_posts": posts})
}

func (s *server) source(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/source/")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "missing source id"})
		return
	}
	path := flywheelSnapshotPath()
	if path == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "no snapshot"})
		return
	}
	snap, err := loadSnapshot(path)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	rec, ok := snap.findSource(id)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "source not found in content_attribution[]", "id": id})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "record": rec})
}

// ledgerSummary is a compact roll-up of both ledgers for the overview card.
func ledgerSummary() map[string]any {
	hs := loadAllLedgers()
	pending, confirmed, refuted, inconclusive := 0, 0, 0, 0
	for _, h := range hs {
		switch h.Verdict {
		case "confirm":
			confirmed++
		case "refute":
			refuted++
		case "inconclusive":
			inconclusive++
		default:
			pending++
		}
	}
	return map[string]any{
		"total": len(hs), "pending": pending, "confirmed": confirmed,
		"refuted": refuted, "inconclusive": inconclusive,
	}
}
