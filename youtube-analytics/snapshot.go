package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const (
	snapshotDir    = "data/snapshots"
	snapshotKeep   = 30
	snapshotPrefix = "videos-"
	snapshotSuffix = ".json"
)

// snapshotVideosJSON copies data/videos.json into data/snapshots/videos-<timestamp>.json.
// Failures are returned to the caller but should be treated as non-fatal by callers.
func snapshotVideosJSON(sourcePath string) (string, error) {
	if err := os.MkdirAll(snapshotDir, 0755); err != nil {
		return "", fmt.Errorf("creating snapshot dir: %w", err)
	}
	ts := time.Now().UTC().Format("20060102T150405Z")
	dest := filepath.Join(snapshotDir, snapshotPrefix+ts+snapshotSuffix)

	src, err := os.Open(sourcePath)
	if err != nil {
		return "", fmt.Errorf("opening source: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(dest)
	if err != nil {
		return "", fmt.Errorf("creating snapshot: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return "", fmt.Errorf("writing snapshot: %w", err)
	}
	return dest, nil
}

// pruneSnapshots keeps the N newest snapshots and deletes the rest.
func pruneSnapshots(keep int) error {
	entries, err := os.ReadDir(snapshotDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	type item struct {
		path    string
		modTime time.Time
	}
	var snaps []item
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if len(name) < len(snapshotPrefix)+len(snapshotSuffix) ||
			name[:len(snapshotPrefix)] != snapshotPrefix {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		snaps = append(snaps, item{
			path:    filepath.Join(snapshotDir, name),
			modTime: info.ModTime(),
		})
	}
	if len(snaps) <= keep {
		return nil
	}
	sort.Slice(snaps, func(i, j int) bool { return snaps[i].modTime.After(snaps[j].modTime) })
	for _, s := range snaps[keep:] {
		_ = os.Remove(s.path)
	}
	return nil
}
