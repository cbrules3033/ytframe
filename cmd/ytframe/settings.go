//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const appVersion = "1.3.2"

const layoutSlots = 5

type Layout struct {
	Used    bool  `json:"used"`
	X       int32 `json:"x"`
	Y       int32 `json:"y"`
	Width   int32 `json:"width"`
	Height  int32 `json:"height"`
	BarX    int32 `json:"bar_x"`
	BarY    int32 `json:"bar_y"`
	BarSet  bool  `json:"bar_set"`
	SavedAt int64 `json:"saved_at"`
}

type Settings struct {
	LastURL       string              `json:"last_url"`
	X             int32               `json:"x"`
	Y             int32               `json:"y"`
	Width         int32               `json:"width"`
	Height        int32               `json:"height"`
	HasRect       bool                `json:"has_rect"`
	Volume        int                 `json:"volume"`
	Ontop         bool                `json:"ontop"`
	Resume        bool                `json:"resume"`
	Quality       string              `json:"quality"`
	SponsorBlock  bool                `json:"sponsor_block"`
	Controls      bool                `json:"controls"`
	VideoMode     string              `json:"video_mode"`
	BarVisible    bool                `json:"bar_visible"`
	BarOntop      bool                `json:"bar_ontop"`
	BarX          int32               `json:"bar_x"`
	BarY          int32               `json:"bar_y"`
	BarHasPos     bool                `json:"bar_has_pos"`
	Layouts       [layoutSlots]Layout `json:"layouts"`
	DefaultLayout int                 `json:"default_layout"`
	BrowseX       int32               `json:"browse_x"`
	BrowseY       int32               `json:"browse_y"`
	BrowseWidth   int32               `json:"browse_width"`
	BrowseHeight  int32               `json:"browse_height"`
	BrowseHasRect bool                `json:"browse_has_rect"`
	ExtraArgs     string              `json:"extra_args,omitempty"`
}

func defaultSettings() Settings {
	return Settings{
		Width: 960, Height: 540, Volume: 100,
		Resume: true, Quality: "best", SponsorBlock: true, Controls: false,
		VideoMode: "auto", BarVisible: true, BarOntop: true,
		DefaultLayout: -1, BrowseWidth: 1200, BrowseHeight: 800,
	}
}

var (
	logMu       sync.Mutex
	logFile     *os.File
	dataDirPath string
)

func dataDir() string {
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exe)
		if f, err := os.CreateTemp(dir, ".ytframe-write-*"); err == nil {
			f.Close()
			os.Remove(f.Name())
			return dir
		}
	}
	ad := os.Getenv("LOCALAPPDATA")
	if ad == "" {
		ad = os.TempDir()
	}
	dir := filepath.Join(ad, "YTFrame")
	os.MkdirAll(dir, 0o755)
	return dir
}

func settingsPath() string { return filepath.Join(dataDirPath, "settings.json") }

func loadSettings() Settings {
	s := defaultSettings()
	b, err := os.ReadFile(settingsPath())
	if err == nil {
		json.Unmarshal(b, &s)
	}
	if s.Width < 100 || s.Height < 60 {
		s.Width, s.Height = 960, 540
	}
	if s.Volume < 0 || s.Volume > 130 {
		s.Volume = 100
	}
	switch s.Quality {
	case "best", "1080", "720", "480":
	default:
		s.Quality = "best"
	}
	switch s.VideoMode {
	case "auto", "visible", "hidden":
	default:
		s.VideoMode = "auto"
	}
	if s.DefaultLayout < -1 || s.DefaultLayout >= layoutSlots {
		s.DefaultLayout = -1
	}
	if s.BrowseWidth < 640 || s.BrowseHeight < 480 {
		s.BrowseWidth, s.BrowseHeight = 1200, 800
	}
	return s
}

func saveSettings(s Settings) {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	tmp := settingsPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return
	}
	os.Rename(tmp, settingsPath())
}

func initLog() {
	logFile = nil
	p := filepath.Join(dataDirPath, "ytframe.log")
	if st, err := os.Stat(p); err == nil && st.Size() > 2<<20 {
		os.Remove(p)
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err == nil {
		logFile = f
	}
	logf("YTFrame %s starting in %s", appVersion, dataDirPath)
}

func logf(format string, args ...interface{}) {
	line := fmt.Sprintf(format, args...)
	logMu.Lock()
	defer logMu.Unlock()
	if logFile != nil {
		fmt.Fprintf(logFile, "%s %s\n", time.Now().Format("2006-01-02 15:04:05"), line)
		// Let the OS buffer diagnostics; fsync on every UI event stalls playback.
	}
	fmt.Fprintf(os.Stderr, "ytframe: %s\n", line)
}

func closeLog() {
	logMu.Lock()
	defer logMu.Unlock()
	if logFile != nil {
		logFile.Close()
		logFile = nil
	}
}

// Program files stay beside the exe even when mutable data falls back to LocalAppData.
func bundledPath(name string) string {
	exe, err := os.Executable()
	if err != nil {
		return filepath.Join(dataDirPath, name)
	}
	return filepath.Join(filepath.Dir(exe), name)
}
