//go:build windows

package main

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// HistoryEntry is the resume position and title for one video. It doubles as
// the storage for a future "recent links" menu.
type HistoryEntry struct {
	URL     string  `json:"url"`
	Title   string  `json:"title,omitempty"`
	Pos     float64 `json:"pos"`
	Dur     float64 `json:"dur"`
	Updated int64   `json:"updated"`
}

type History struct {
	Version int                     `json:"version"`
	Entries map[string]HistoryEntry `json:"entries"`
}

const historyVersion = 1

func historyPath() string { return filepath.Join(dataDirPath, "history.json") }

func loadHistory() *History {
	h := &History{Version: historyVersion, Entries: map[string]HistoryEntry{}}
	if b, err := os.ReadFile(historyPath()); err == nil {
		json.Unmarshal(b, h)
	}
	if h.Entries == nil {
		h.Entries = map[string]HistoryEntry{}
	}
	return h
}

func (h *History) get(key string) (HistoryEntry, bool) {
	if h == nil || key == "" {
		return HistoryEntry{}, false
	}
	e, ok := h.Entries[key]
	return e, ok
}

func (h *History) set(key string, e HistoryEntry) {
	if h == nil || key == "" {
		return
	}
	if h.Entries == nil {
		h.Entries = map[string]HistoryEntry{}
	}
	h.Entries[key] = e
}

func (h *History) remove(key string) {
	if h == nil || key == "" {
		return
	}
	delete(h.Entries, key)
}

func (h *History) save() {
	if h == nil {
		return
	}
	// Keep the store small: prune the oldest entries beyond 100.
	if len(h.Entries) > 100 {
		type pair struct {
			k string
			t int64
		}
		all := make([]pair, 0, len(h.Entries))
		for k, e := range h.Entries {
			all = append(all, pair{k, e.Updated})
		}
		sort.Slice(all, func(i, j int) bool { return all[i].t > all[j].t })
		for _, p := range all[100:] {
			delete(h.Entries, p.k)
		}
	}
	h.Version = historyVersion
	b, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return
	}
	tmp := historyPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		logf("history write failed: %v", err)
		return
	}
	if err := os.Rename(tmp, historyPath()); err != nil {
		logf("history rename failed: %v", err)
	}
}

// videoKey returns a stable key for resume/history. YouTube links collapse to
// the 11-character video ID, local paths to a lower-cased path, and other URLs
// to a version without timestamp parameters.
func videoKey(raw string) string {
	if id := youtubeID(raw); id != "" {
		return "yt:" + id
	}
	if isDrivePath(raw) {
		return "file:" + strings.ToLower(filepath.Clean(raw))
	}
	low := strings.ToLower(raw)
	if strings.HasPrefix(low, "file://") {
		return "file:" + strings.ToLower(raw)
	}
	if u, err := url.Parse(raw); err == nil {
		q := u.Query()
		q.Del("t")
		q.Del("start")
		u.RawQuery = q.Encode()
		u.Fragment = ""
		return "url:" + u.String()
	}
	return "raw:" + raw
}

func youtubeID(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	host = strings.TrimPrefix(host, "www.")
	switch host {
	case "youtu.be":
		id := strings.Trim(strings.TrimPrefix(u.Path, "/"), "/")
		if validYouTubeID(id) {
			return id
		}
		return ""
	case "youtube.com", "m.youtube.com", "music.youtube.com", "youtube-nocookie.com":
		if v := u.Query().Get("v"); validYouTubeID(v) {
			return v
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) >= 2 {
			switch parts[0] {
			case "shorts", "live", "embed", "v":
				if validYouTubeID(parts[1]) {
					return parts[1]
				}
			}
		}
	}
	return ""
}

func validYouTubeID(s string) bool {
	if len(s) != 11 {
		return false
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func hasPlaylistParam(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return u.Query().Get("list") != ""
}

func isLiveURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	low := strings.ToLower(u.Path)
	return strings.Contains(low, "/live/") || u.Query().Get("live") == "1"
}

// resumeStart returns a saved VOD position, or 0. A timestamp in the link is
// handled by the caller and takes precedence.
func resumeStart(raw string, h *History) int {
	if h == nil || raw == "" {
		return 0
	}
	if hasPlaylistParam(raw) || isLiveURL(raw) {
		return 0
	}
	e, ok := h.get(videoKey(raw))
	if !ok {
		return 0
	}
	if e.Dur < 60 || e.Pos < 10 || e.Pos > e.Dur-20 {
		return 0
	}
	pos := int(e.Pos)
	if pos > 86400 {
		pos = 86400
	}
	if pos < 0 {
		return 0
	}
	return pos
}

func nowUnix() int64 { return time.Now().Unix() }

func recentEntries(h *History, limit int) []HistoryEntry {
	if h == nil || limit <= 0 {
		return nil
	}
	entries := make([]HistoryEntry, 0, len(h.Entries))
	for _, e := range h.Entries {
		if e.URL != "" {
			entries = append(entries, e)
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Updated == entries[j].Updated {
			return entries[i].URL < entries[j].URL
		}
		return entries[i].Updated > entries[j].Updated
	})
	if len(entries) > limit {
		entries = entries[:limit]
	}
	return entries
}
