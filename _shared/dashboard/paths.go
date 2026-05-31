// paths.go — env-overridable filesystem locations for every snapshot the
// dashboard reads. Mirrors the envOr/home pattern from
// _shared/content-attribution so the binary is relocatable and testable.
package main

import (
	"os"
	"path/filepath"
	"sort"
)

func home() string {
	h, _ := os.UserHomeDir()
	return h
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func repoRoot() string {
	return envOr("DASH_REPO_ROOT", filepath.Join(home(), "dev/claude-social-media-skills"))
}

// flywheelSnapshotPath returns the snapshot to read: an explicit override if set,
// otherwise the newest dated JSON in the snapshots dir. Empty string if none.
func flywheelSnapshotPath() string {
	if p := os.Getenv("DASH_FLYWHEEL_SNAPSHOT"); p != "" {
		return p
	}
	dir := envOr("DASH_FLYWHEEL_SNAPSHOT_DIR", filepath.Join(home(), "dev/flywheel-snapshots"))
	return newestMatch(dir, ".json")
}

func ytVideosPath() string {
	return envOr("DASH_YT_VIDEOS", filepath.Join(repoRoot(), "youtube-analytics/data/videos.json"))
}

func bufferCacheDir() string {
	return envOr("DASH_BUFFER_CACHE_DIR", filepath.Join(repoRoot(), "buffer-stats/cache"))
}

func linkedinCacheDir() string {
	return envOr("DASH_LINKEDIN_CACHE_DIR", filepath.Join(repoRoot(), "linkedin-stats/cache"))
}

func voiceCachePath() string {
	return envOr("DASH_VOICE_CACHE", filepath.Join(repoRoot(), "_shared/voice-corpus/cache.json"))
}

func crossSurfaceLedgerDir() string {
	return envOr("DASH_XSURFACE_LEDGER", filepath.Join(repoRoot(), "insights"))
}

func youtubeLedgerDir() string {
	return envOr("DASH_YT_LEDGER", filepath.Join(repoRoot(), "youtube-analytics/data/insights"))
}

// newestMatch returns the lexicographically-largest file in dir with the given
// suffix (ISO-8601-dated names sort chronologically). Empty string if none.
func newestMatch(dir, suffix string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if len(n) >= len(suffix) && n[len(n)-len(suffix):] == suffix {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	return filepath.Join(dir, names[len(names)-1])
}
