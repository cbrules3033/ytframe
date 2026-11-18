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

const appVersion = "1.0.0"

type Settings struct {
	LastURL   string `json:"last_url"`
	X         int32  `json:"x"`
	Y         int32  `json:"y"`
	Width     int32  `json:"width"`
	Height    int32  `json:"height"`
	HasRect   bool   `json:"has_rect"`
	Volume    int    `json:"volume"`
	Ontop     bool   `json:"ontop"`
	ExtraArgs string `json:"extra_args,omitempty"`
}

func defaultSettings() Settings {
	return Settings{Width: 960, Height: 540, Volume: 100}
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
		test := filepath.Join(dir, ".write-test")
		if f, err := os.Create(test); err == nil {
			f.Close()
			os.Remove(test)
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
		logFile.Sync()
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
